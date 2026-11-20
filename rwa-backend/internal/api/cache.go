package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/model"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/store"
)

// refreshInterval is how often the cache checks builds.is_current.
const refreshInterval = 15 * time.Second

// maxArchived is how many non-current builds (?build=<id>) are kept in memory (raw and gzip bytes only).
const maxArchived = 8

// blob is a build as served by /export: the exact bytes, their gzip form, the ETag and the metadata. Old builds are
// kept only as blobs: they are never parsed, so an export of an older schema can always be served.
type blob struct {
	meta store.BuildMeta
	raw  []byte
	gz   []byte
	etag string // quoted sha256
}

func newBlob(b *store.Build) blob {
	return blob{meta: b.BuildMeta, raw: b.Export, gz: gzipBytes(b.Export), etag: `"` + b.ExportSHA256 + `"`}
}

// flight is one in-progress load of an archived build, shared by concurrent requests.
type flight struct {
	done chan struct{}
	b    *blob
	err  error
}

// oppRow is one (token, opportunity) pair of the opportunities table; rel is nil for an unconfirmed candidate.
type oppRow struct {
	tokenID      int64
	underlyingID string
	issuerID     string
	opp          *model.Opportunity
	rel          *model.Relation
}

// snapshot is one build held in memory: the exact export bytes, their gzip form and the indexes of the parsed export.
type snapshot struct {
	blob
	export *model.Export

	underlyingByID     map[string]*model.Underlying
	underlyingByTicker map[string]*model.Underlying // upper-case ticker
	tokenByID          map[int64]*model.Token
	issuerByID         map[string]*model.Issuer
	oppByKey           map[string]*model.Opportunity
	rows               []oppRow // sorted by (opportunity key, token id)
}

func newSnapshot(b *store.Build) (*snapshot, error) {
	var exp model.Export
	if err := json.Unmarshal(b.Export, &exp); err != nil {
		return nil, fmt.Errorf("parse export of build %d: %w", b.ID, err)
	}
	exp.Normalize()
	s := &snapshot{
		blob:               newBlob(b),
		export:             &exp,
		underlyingByID:     make(map[string]*model.Underlying, len(exp.Underlyings)),
		underlyingByTicker: make(map[string]*model.Underlying, len(exp.Underlyings)),
		tokenByID:          make(map[int64]*model.Token, len(exp.Tokens)),
		issuerByID:         make(map[string]*model.Issuer, len(exp.Issuers)),
		oppByKey:           make(map[string]*model.Opportunity, len(exp.Opportunities)),
	}
	for i := range exp.Underlyings {
		u := &exp.Underlyings[i]
		s.underlyingByID[u.ID] = u
		if _, dup := s.underlyingByTicker[strings.ToUpper(u.Ticker)]; !dup {
			s.underlyingByTicker[strings.ToUpper(u.Ticker)] = u
		}
	}
	for i := range exp.Issuers {
		s.issuerByID[exp.Issuers[i].ID] = &exp.Issuers[i]
	}
	for i := range exp.Opportunities {
		s.oppByKey[exp.Opportunities[i].Key] = &exp.Opportunities[i]
	}
	for i := range exp.Tokens {
		t := &exp.Tokens[i]
		s.tokenByID[t.ID] = t
		issuer := ""
		if t.IssuerID != nil {
			issuer = *t.IssuerID
		}
		for j := range t.Relations {
			if o := s.oppByKey[t.Relations[j].Key]; o != nil {
				s.rows = append(s.rows, oppRow{t.ID, t.UnderlyingID, issuer, o, &t.Relations[j]})
			}
		}
		for _, k := range t.UnconfirmedKeys {
			if o := s.oppByKey[k]; o != nil {
				s.rows = append(s.rows, oppRow{t.ID, t.UnderlyingID, issuer, o, nil})
			}
		}
	}
	sort.SliceStable(s.rows, func(i, j int) bool { return rowLess(s.rows[i], s.rows[j].opp.Key, s.rows[j].tokenID) })
	return s, nil
}

// rowLess reports whether row a sorts before (key, tokenID).
func rowLess(a oppRow, key string, tokenID int64) bool {
	if a.opp.Key != key {
		return a.opp.Key < key
	}
	return a.tokenID < tokenID
}

// rowAfter reports whether row a sorts strictly after (key, tokenID).
func rowAfter(a oppRow, key string, tokenID int64) bool {
	if a.opp.Key != key {
		return a.opp.Key > key
	}
	return a.tokenID > tokenID
}

// underlying resolves an id or a ticker (case-insensitive).
func (s *snapshot) underlying(idOrTicker string) *model.Underlying {
	if u := s.underlyingByID[idOrTicker]; u != nil {
		return u
	}
	return s.underlyingByTicker[strings.ToUpper(idOrTicker)]
}

// cache holds the current build and a few older ones.
type cache struct {
	st  Store
	log *slog.Logger
	cur atomic.Pointer[snapshot]

	mu       sync.Mutex
	archived map[int64]*blob
	order    []int64 // least recently used first
	loading  map[int64]*flight
	loads    atomic.Int64 // database loads of archived builds (for tests)
}

func newCache(st Store, log *slog.Logger) *cache {
	return &cache{st: st, log: log, archived: map[int64]*blob{}, loading: map[int64]*flight{}}
}

func (c *cache) current() *snapshot { return c.cur.Load() }

// refresh loads the current build when it differs from the cached one. It is a no-op without a build.
func (c *cache) refresh(ctx context.Context) error {
	meta, err := c.st.CurrentBuildMeta(ctx)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil
		}
		return fmt.Errorf("current build meta: %w", err)
	}
	if cur := c.cur.Load(); cur != nil && cur.meta.ID == meta.ID && cur.meta.ExportSHA256 == meta.ExportSHA256 {
		return nil
	}
	b, err := c.st.CurrentBuild(ctx)
	if err != nil {
		return fmt.Errorf("load current build: %w", err)
	}
	snap, err := newSnapshot(b)
	if err != nil {
		return err
	}
	prev := c.cur.Swap(snap)
	c.log.Info("current build loaded", "build_id", snap.meta.ID, "run_id", snap.meta.RunID, "bytes", len(snap.raw))
	if prev != nil && prev.meta.ID != snap.meta.ID {
		c.archive(&prev.blob)
	}
	return nil
}

// run refreshes every refreshInterval until ctx ends. Errors are logged, the previous build keeps serving.
func (c *cache) run(ctx context.Context) {
	if err := c.refresh(ctx); err != nil && ctx.Err() == nil {
		c.log.Error("initial build load failed", "err", err)
	}
	t := time.NewTicker(refreshInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := c.refresh(ctx); err != nil && ctx.Err() == nil {
				c.log.Error("build refresh failed", "err", err)
			}
		}
	}
}

// archive stores b as the most recently used entry, evicting the least recently used one beyond maxArchived.
// c.mu must not be held.
func (c *cache) archive(b *blob) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.putLocked(b)
}

func (c *cache) putLocked(b *blob) {
	if _, ok := c.archived[b.meta.ID]; ok {
		c.touchLocked(b.meta.ID)
		return
	}
	c.archived[b.meta.ID] = b
	c.order = append(c.order, b.meta.ID)
	for len(c.order) > maxArchived {
		delete(c.archived, c.order[0])
		c.order = c.order[1:]
	}
}

func (c *cache) touchLocked(id int64) {
	for i, v := range c.order {
		if v == id {
			c.order = append(append(c.order[:i:i], c.order[i+1:]...), id)
			return
		}
	}
}

// byID returns a build by id: the current one, an archived one (LRU hit), or one loaded from the database. Concurrent
// misses for the same id share a single load.
func (c *cache) byID(ctx context.Context, id int64) (*blob, error) {
	if cur := c.cur.Load(); cur != nil && cur.meta.ID == id {
		return &cur.blob, nil
	}
	c.mu.Lock()
	if b := c.archived[id]; b != nil {
		c.touchLocked(id)
		c.mu.Unlock()
		return b, nil
	}
	if f := c.loading[id]; f != nil {
		c.mu.Unlock()
		select {
		case <-f.done:
			return f.b, f.err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	f := &flight{done: make(chan struct{})}
	c.loading[id] = f
	c.mu.Unlock()

	// The load must not die with the first requester's context: others may be waiting on it.
	lctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	c.loads.Add(1)
	b, err := c.st.BuildByID(lctx, id)
	c.mu.Lock()
	if err == nil {
		nb := newBlob(b)
		f.b = &nb
		c.putLocked(f.b)
	} else {
		f.err = err
	}
	delete(c.loading, id)
	c.mu.Unlock()
	close(f.done)
	return f.b, f.err
}
