// Package collect is one collection run: the port of 1.x collect.py. It fetches CMC and Portals data per
// underlying and stores one snapshot per underlying in the database. It does not compute or publish.
package collect

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"path/filepath"
	"runtime/debug"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/config"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/db"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/model"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/runlog"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/source/cmc"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/source/httpx"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/source/portals"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/store"
)

// Version is the collector version recorded in collection_runs; main sets it.
var Version = "dev"

// ErrRunInProgress is returned when another collector run holds the advisory lock. No run row is created.
var ErrRunInProgress = errors.New("collect: another run is in progress")

// RunKeyFormat is the layout of run_key (1.x run_id).
const RunKeyFormat = "20060102T150405Z"

const lockQuery = `SELECT pg_try_advisory_lock(hashtext('rwa-collector-run'))`

// Options selects what a run does.
type Options struct {
	Mode    string   // "refresh" | "add"
	Symbols []string // nil = all enabled
	Trigger string   // schedule|startup|manual|cli
}

// Result is the outcome of a run.
type Result struct {
	RunID       int64
	RunKey      string
	Status      string // succeeded|partial|failed
	Completed   int
	Failed      int
	NothingToDo bool
	Err         error // fatal error of the run, if any

	// Extra counters for reports.
	Selected        int
	Warnings        []string
	CMCRequests     int
	PortalsRequests int
	CMCCredits      int
}

// Runner performs collection runs.
type Runner struct {
	pool     *pgxpool.Pool
	store    *store.Store
	cfg      config.Config
	settings *model.Settings
	assets   []model.Asset
	now      func() time.Time
	log      *slog.Logger

	cmcBase, portalsBase string
	httpClient           *http.Client
}

// NewRunner creates a Runner. log may be nil.
func NewRunner(pool *pgxpool.Pool, cfg config.Config, settings *model.Settings, assets []model.Asset, log *slog.Logger) *Runner {
	if log == nil {
		log = slog.Default()
	}
	return &Runner{pool: pool, store: store.New(pool), cfg: cfg, settings: settings, assets: assets, now: time.Now, log: log,
		cmcBase: cmc.BaseURL, portalsBase: portals.BaseURL}
}

// WithBaseURLs overrides the provider hosts (tests). An empty value keeps the default.
func (r *Runner) WithBaseURLs(cmcURL, portalsURL string) *Runner {
	if cmcURL != "" {
		r.cmcBase = cmcURL
	}
	if portalsURL != "" {
		r.portalsBase = portalsURL
	}
	return r
}

// WithHTTPClient overrides the transport of both provider clients (tests).
func (r *Runner) WithHTTPClient(c *http.Client) *Runner {
	r.httpClient = c
	return r
}

// WithClock overrides the clock (tests).
func (r *Runner) WithClock(now func() time.Time) *Runner {
	r.now = now
	return r
}

func (r *Runner) newClient(provider, base string, headers map[string]string, rawDir string, rpm int, log *slog.Logger) *httpx.Client {
	return httpx.New(httpx.Config{
		Provider:          provider,
		BaseURL:           base,
		Headers:           headers,
		Timeout:           time.Duration(r.settings.RequestTimeoutSeconds) * time.Second,
		RequestsPerMinute: rpm,
		MaxRetries:        r.settings.MaxRetries,
		RawDir:            rawDir,
		Logger:            log,
		HTTPClient:        r.httpClient,
		OnRequest: func(p, path string, _ url.Values, status int, d time.Duration) {
			log.Debug("request", "step", "collect."+p, "provider", p, "path", path, "status", status, "duration_ms", d.Milliseconds())
		},
	})
}

// resolution is the CMC record chosen for an underlying, or the reason none was.
type resolution struct {
	rwa    *cmc.RWAAsset
	reason string
}

// ErrInvalidOptions marks a request the runner can never serve (bad mode, trigger or symbols). A manual run request that
// fails with it is rejected, not retried.
var ErrInvalidOptions = errors.New("collect: invalid options")

// Run performs one collection run. A fatal error of the run is reported in Result.Err and Status "failed" (the
// returned error is nil); the returned error is reserved for failures before the run row exists (bad options,
// database, ErrRunInProgress) and for context cancellation or a deadline (ctx.Err(); the run is recorded as failed
// with error "interrupted" or "timeout"). Whenever a run row exists the returned Result carries its RunID, also with
// a non-nil error. A panic after the run row exists finishes the run as failed and is returned as an error.
func (r *Runner) Run(ctx context.Context, opts Options) (res *Result, err error) {
	mode := opts.Mode
	if mode == "" {
		mode = ModeRefresh
	}
	if mode != ModeRefresh && mode != ModeAdd {
		return nil, fmt.Errorf("%w: invalid mode %q", ErrInvalidOptions, mode)
	}
	trigger := opts.Trigger
	if trigger == "" {
		trigger = "manual"
	}
	if !slices.Contains([]string{"schedule", "startup", "manual", "cli"}, trigger) {
		return nil, fmt.Errorf("%w: invalid trigger %q", ErrInvalidOptions, trigger)
	}
	selected, err := SelectAssets(r.assets, opts.Symbols)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidOptions, err)
	}

	lock, err := db.TryLock(ctx, r.pool, db.CollectorLockName, r.log)
	if errors.Is(err, db.ErrLocked) {
		return nil, ErrRunInProgress
	}
	if err != nil {
		return nil, fmt.Errorf("collect: %w", err)
	}
	defer lock.Release(ctx)

	// Under the lock: no other collector or import changes the underlyings table or the snapshots meanwhile.
	if _, err := r.store.SyncUnderlyings(ctx, r.assets); err != nil {
		return nil, fmt.Errorf("collect: %w", err)
	}
	var have map[string]bool
	if mode == ModeAdd {
		if have, err = r.store.UnderlyingsWithSnapshot(ctx); err != nil {
			return nil, fmt.Errorf("collect: %w", err)
		}
	}
	todo := Todo(selected, mode, have)
	if len(todo) == 0 {
		r.log.Info("nothing to add: every selected underlying already has a snapshot", "selected", len(selected))
		return &Result{NothingToDo: true}, nil
	}

	started := r.now().UTC()
	runKey := started.Format(RunKeyFormat)
	settingsJSON, err := json.Marshal(r.settings)
	if err != nil {
		return nil, fmt.Errorf("collect: marshal settings: %w", err)
	}
	runID, err := r.store.CreateRun(ctx, store.RunParams{RunKey: runKey, Mode: mode, Trigger: trigger, StartedAt: started,
		CollectorVersion: Version, Settings: settingsJSON})
	if err != nil {
		return nil, fmt.Errorf("collect: %w", err)
	}
	log := r.log.With("run_id", runID, "run_key", runKey)
	defer func() {
		p := recover()
		if p == nil {
			return
		}
		log.Error("run panicked", "step", "run", "panic", fmt.Sprint(p), "stack", string(debug.Stack()))
		fctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		text := fmt.Sprintf("panic: %v", p)
		if ferr := r.store.FinishRun(fctx, runID, RunFailed, store.RunCounters{Selected: len(selected)}, nil, text); ferr != nil {
			log.Error("finish run after panic failed", "step", "run", "err", ferr)
		}
		runlog.Flush(r.log)
		res = &Result{RunID: runID, RunKey: runKey, Status: RunFailed, Selected: len(selected)}
		err = fmt.Errorf("collect: run %d: %s", runID, text)
	}()

	rawDir := ""
	if r.cfg.RawCaptureDir != "" {
		rawDir = filepath.Join(r.cfg.RawCaptureDir, runKey)
	}
	log.Info("run started", "step", "run", "mode", mode, "trigger", trigger, "selected", len(selected), "to_fetch", len(todo),
		"already_have_snapshot", len(selected)-len(todo), "raw_dir", rawDir)

	cmcHTTP := r.newClient("cmc", r.cmcBase, map[string]string{"X-CMC_PRO_API_KEY": r.cfg.CMCAPIKey}, rawDir, r.settings.RequestsPerMinute.CMC, log)
	portalsHTTP := r.newClient("portals", r.portalsBase, map[string]string{"Authorization": "Bearer " + r.cfg.PortalsAPIKey}, rawDir, r.settings.RequestsPerMinute.Portals, log)

	rn := &run{Runner: r, log: log, id: runID, key: runKey, mode: mode, started: time.Now(), recorded: map[string]bool{},
		cmcHTTP: cmcHTTP, portalsHTTP: portalsHTTP, cmc: cmc.New(cmcHTTP), portals: portals.New(portalsHTTP)}
	rn.execute(ctx, todo)
	rn.skipUnprocessed(ctx, todo)

	res = &Result{RunID: runID, RunKey: runKey, Completed: rn.completed, Failed: rn.failed, Selected: len(selected),
		Warnings: rn.warnings, CMCRequests: int(cmcHTTP.Requests()), PortalsRequests: int(portalsHTTP.Requests()),
		CMCCredits: int(cmcHTTP.Credits())}
	res.Status = DecideStatus(rn.completed, rn.failed, rn.fatal != nil, rn.interrupted)
	errText := ""
	switch {
	case rn.interrupted && errors.Is(ctx.Err(), context.DeadlineExceeded):
		errText = ErrTextTimeout
	case rn.interrupted:
		errText = ErrTextInterrupted
	case rn.fatal != nil:
		errText = rn.fatal.Error()
		res.Err = rn.fatal
	}
	fctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	if err := r.store.FinishRun(fctx, runID, res.Status, store.RunCounters{Selected: len(selected), Completed: rn.completed,
		Failed: rn.failed, CMCRequests: res.CMCRequests, PortalsRequests: res.PortalsRequests, CMCCredits: res.CMCCredits},
		rn.warnings, errText); err != nil {
		return res, fmt.Errorf("collect: %w", err)
	}
	level := slog.LevelInfo
	switch res.Status {
	case RunPartial:
		level = slog.LevelWarn
	case RunFailed:
		level = slog.LevelError
	}
	log.Log(ctx, level, "run finished", "step", "run", "duration_ms", time.Since(rn.started).Milliseconds(), "status", res.Status, "completed", rn.completed, "failed", rn.failed, "warnings", len(rn.warnings),
		"cmc_requests", res.CMCRequests, "portals_requests", res.PortalsRequests, "cmc_credits", res.CMCCredits,
		"elapsed", fmtDuration(time.Since(rn.started)), "error", errText)
	runlog.Flush(r.log)
	if rn.interrupted {
		return res, ctx.Err()
	}
	return res, nil
}

// Error texts of a run that ended because its context ended.
const (
	ErrTextInterrupted = "interrupted" // the process was asked to stop
	ErrTextTimeout     = "timeout"     // the run exceeded its deadline (RUN_TIMEOUT)
)

// run is the state of one run in progress.
type run struct {
	*Runner
	log     *slog.Logger
	id      int64
	key     string
	mode    string
	started time.Time

	cmcHTTP, portalsHTTP *httpx.Client
	cmc                  *cmc.Client
	portals              *portals.Client

	warnings    []string
	rwaInfo     map[string]cmc.RWAInfo
	completed   int
	failed      int
	fatal       error
	interrupted bool
	recorded    map[string]bool // underlyings with a run_underlyings row
}

func (rn *run) warn(format string, args ...any) { rn.warnAt("run", format, args...) }

// warnAt records a run warning and logs it under the given journal step.
func (rn *run) warnAt(step, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	rn.warnings = append(rn.warnings, msg)
	rn.log.Warn(msg, "step", step)
}

// stop classifies an error that ends the run: a cancelled context is an interruption, anything else fatal.
func (rn *run) stop(ctx context.Context, err error) {
	if ctx.Err() != nil {
		rn.interrupted = true
		return
	}
	rn.fatal = err
	rn.log.Error("fatal", "step", "run", "err", err)
}

func (rn *run) execute(ctx context.Context, todo []model.Asset) {
	resolved, sh, ok := rn.globalSteps(ctx, todo)
	if !ok {
		return
	}
	rn.log.Info("Portals per underlying: exact lookup, then text search per term", "step", "collect.underlying", "to_go", len(todo))
	loopStarted := time.Now()
	width := len(strconv.Itoa(len(todo)))
	for n, asset := range todo {
		if ctx.Err() != nil {
			rn.interrupted = true
			return
		}
		res := resolved[asset.ID()]
		prefix := fmt.Sprintf("[%*d/%d] %s", width, n+1, len(todo), asset.Ticker)
		ulog := rn.log.With("step", "collect.underlying", "underlying", asset.Ticker, "progress", prefix)
		t0 := time.Now()
		startedAt := rn.now().UTC()
		reqBefore := rn.portalsHTTP.Requests()
		payload, status, err := collectAsset(ctx, ulog, asset, res.rwa, res.reason, sh, rn.portals, rn.settings, rn.key, rn.now)
		ru := store.RunUnderlying{RunID: rn.id, UnderlyingID: asset.ID(), StartedAt: &startedAt}
		if err == nil {
			err = rn.save(ctx, asset, res, status, payload)
		}
		ru.PortalsRequests = int(rn.portalsHTTP.Requests() - reqBefore)
		ru.FinishedAt = ptr(rn.now().UTC())
		if err != nil {
			if ctx.Err() != nil {
				rn.interrupted = true
				return
			}
			rn.failed++
			ru.Status = "failed"
			msg := err.Error()
			ru.Error = &msg
			ulog.Warn("failed, previous snapshot kept", "err", err)
			rn.recordUnderlying(ctx, ru)
			if errors.Is(err, httpx.ErrAuth) {
				rn.fatal = err
				rn.log.Error("fatal", "step", "run", "err", err)
				return
			}
			continue
		}
		rn.completed++
		ru.Status = status
		rn.recordUnderlying(ctx, ru)
		perAsset := time.Since(loopStarted) / time.Duration(n+1)
		eta := perAsset * time.Duration(len(todo)-n-1)
		args := []any{"status", status, "took", fmtDuration(time.Since(t0)), "duration_ms", time.Since(t0).Milliseconds(), "elapsed", fmtDuration(time.Since(rn.started))}
		if n+1 < len(todo) {
			args = append(args, "eta", fmtDuration(eta))
		}
		ulog.Info("saved", args...)
	}
}

func (rn *run) recordUnderlying(ctx context.Context, ru store.RunUnderlying) {
	if err := rn.store.UpsertRunUnderlying(context.WithoutCancel(ctx), ru); err != nil {
		rn.warn("run_underlyings %s: %v", ru.UnderlyingID, err)
		return
	}
	rn.recorded[ru.UnderlyingID] = true
}

// skipUnprocessed gives every underlying the run did not reach (fatal error, interruption, timeout) a run_underlyings
// row with status skipped, so the run detail lists all selected underlyings.
func (rn *run) skipUnprocessed(ctx context.Context, todo []model.Asset) {
	for _, a := range todo {
		if rn.recorded[a.ID()] {
			continue
		}
		rn.recordUnderlying(ctx, store.RunUnderlying{RunID: rn.id, UnderlyingID: a.ID(), Status: "skipped"})
	}
}

// save inserts the snapshot and the CMC facts of the underlying in one transaction. website, logo and industry are
// written only when this run's CMC metadata has an entry for the underlying (1.x kept its cache otherwise).
func (rn *run) save(ctx context.Context, asset model.Asset, res resolution, status string, payload []byte) error {
	fetched, err := model.ParseSnapshot(payload)
	if err != nil {
		return fmt.Errorf("own payload: %w", err)
	}
	facts := store.UnderlyingFactsRow{ID: asset.ID(), Status: &status, LastRunID: rn.id, KeepInfo: true}
	if res.rwa != nil {
		facts.CMCRwaID = res.rwa.RwaID
		facts.CMCName = res.rwa.Name
		if info, ok := rn.rwaInfo[rwaKey(res.rwa)]; ok {
			facts.Website, facts.CMCLogoURL, facts.Industry = info.Website, info.Logo, info.Industry
			facts.KeepInfo = false
		}
	}
	return db.WithTx(ctx, rn.pool, func(tx pgx.Tx) error {
		st := store.New(tx)
		if _, err := st.InsertSnapshot(ctx, rn.id, asset.ID(), fetched.FetchedAt, status, payload); err != nil {
			return err
		}
		return st.UpsertUnderlyingFacts(ctx, []store.UnderlyingFactsRow{facts})
	})
}

func rwaKey(a *cmc.RWAAsset) string {
	if a.RwaID == nil {
		return ""
	}
	return strconv.Itoa(*a.RwaID)
}

// globalSteps are [1/5]..[5/5]: everything shared by all underlyings. It reports false when the run must end.
func (rn *run) globalSteps(ctx context.Context, todo []model.Asset) (map[string]resolution, shared, bool) {
	var sh shared
	tickers := make([]string, len(todo))
	for i, a := range todo {
		tickers[i] = a.Ticker
	}

	rn.log.Info("[1/5] CMC: tokenized assets (RWA quotes), one request", "step", "collect.cmc", "n", 1, "tickers", len(tickers))
	quotes, err := rn.cmc.FetchRWAQuotes(ctx, tickers)
	if err != nil {
		rn.stop(ctx, err)
		return nil, sh, false
	}
	known, err := rn.store.LatestKnownRWAIDs(ctx)
	if err != nil {
		rn.stop(ctx, err)
		return nil, sh, false
	}
	resolved := map[string]resolution{}
	var notResolved []string
	found := 0
	for _, a := range todo {
		var knownID *int
		if id, ok := known[a.ID()]; ok {
			knownID = &id
		}
		rwa, reason := cmc.PickRWAAsset(quotes[a.Ticker], a, knownID)
		resolved[a.ID()] = resolution{rwa, reason}
		if rwa != nil {
			found++
		} else {
			notResolved = append(notResolved, fmt.Sprintf("%s (%s)", a.Ticker, reason))
		}
	}
	rn.log.Info("tickers tracked by CMC", "step", "collect.cmc", "n", 1, "found", found, "not_resolved", notResolved)

	var cryptoIDs []int
	issuerIDs := map[string]bool{}
	issuerLabels := map[string]*string{}
	var rwaIDs []int
	for _, a := range todo {
		rwa := resolved[a.ID()].rwa
		if rwa == nil {
			continue
		}
		if rwa.RwaID != nil {
			rwaIDs = append(rwaIDs, *rwa.RwaID)
		}
		for _, t := range rwa.Tokens {
			if t.CryptoID != nil && *t.CryptoID != 0 {
				cryptoIDs = append(cryptoIDs, int(*t.CryptoID))
			}
			if t.IssuerID != nil && *t.IssuerID != "" {
				issuerIDs[*t.IssuerID] = true
				if issuerLabels[*t.IssuerID] == nil {
					issuerLabels[*t.IssuerID] = t.IssuerName
				}
			}
		}
	}

	rn.log.Info("[2/5] CMC: contract addresses (cryptocurrency/info)", "step", "collect.cmc", "n", 2, "tokens", len(uniqueInts(cryptoIDs)), "batch", cmc.InfoBatch)
	sh.info, sh.invalid, err = rn.cmc.FetchCryptoInfo(ctx, cryptoIDs)
	if err != nil {
		rn.stop(ctx, err)
		return nil, sh, false
	}
	if len(sh.invalid) > 0 {
		rn.warnAt("collect.cmc", "CMC rejected ids for /v2/cryptocurrency/info: %s", formatIDs(sh.invalid))
	}

	var haveIssuers map[string]bool
	if rn.mode == ModeAdd {
		if haveIssuers, err = rn.store.KnownIssuerIDs(ctx); err != nil {
			rn.stop(ctx, err)
			return nil, sh, false
		}
	}
	var toFetch []string
	for id := range issuerIDs {
		if rn.mode == ModeRefresh || !haveIssuers[id] {
			toFetch = append(toFetch, id)
		}
	}
	sort.Strings(toFetch)
	rn.log.Info("[3/5] CMC: issuers (one request each)", "step", "collect.cmc", "n", 3, "to_fetch", len(toFetch), "total", len(issuerIDs))
	for _, id := range toFetch {
		iss, err := rn.cmc.FetchIssuer(ctx, id)
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, httpx.ErrAuth) {
				rn.stop(ctx, err)
				return nil, sh, false
			}
			rn.warnAt("collect.cmc", "issuer %s: %v", id, err)
			continue
		}
		if err := rn.upsertIssuer(ctx, iss, issuerLabels[id]); err != nil {
			rn.stop(ctx, err)
			return nil, sh, false
		}
		rn.log.Info("issuer fetched", "step", "collect.cmc", "n", 3, "issuer", id, "name", deref(iss.Name), "roster_size", iss.RosterSize)
	}

	rn.log.Info("[4/5] CMC: RWA metadata (website, logo, industry)", "step", "collect.cmc", "n", 4, "underlyings", len(rwaIDs))
	rn.rwaInfo = map[string]cmc.RWAInfo{}
	if len(rwaIDs) > 0 {
		info, err := rn.cmc.FetchRWAInfo(ctx, rwaIDs)
		switch {
		case err == nil:
			rn.rwaInfo = info
		case ctx.Err() != nil || errors.Is(err, httpx.ErrAuth):
			rn.stop(ctx, err)
			return nil, sh, false
		default:
			rn.warnAt("collect.cmc", "rwa info: %v", err)
		}
	}
	rn.log.Info("CMC done", "step", "collect.cmc", "n", 4, "requests", rn.cmcHTTP.Requests(), "credits", rn.cmcHTTP.Credits())

	rn.log.Info("[5/5] Portals: supported networks and platform names", "step", "collect.portals", "n", 5)
	sh.networks, err = rn.portals.FetchNetworks(ctx)
	if err != nil {
		rn.stop(ctx, err)
		return nil, sh, false
	}
	if err := rn.store.UpsertNetworks(ctx, rn.id, rn.networkRows(sh.networks)); err != nil {
		rn.stop(ctx, err)
		return nil, sh, false
	}
	fetchPlatforms := rn.mode == ModeRefresh
	if !fetchPlatforms {
		n, err := rn.store.PlatformCount(ctx)
		if err != nil {
			rn.stop(ctx, err)
			return nil, sh, false
		}
		fetchPlatforms = n == 0
	}
	if fetchPlatforms {
		names, err := rn.portals.FetchPlatformNames(ctx)
		if err != nil {
			rn.stop(ctx, err)
			return nil, sh, false
		}
		rows := make([]store.PlatformRow, 0, len(names))
		for slug, name := range names {
			rows = append(rows, store.PlatformRow{Slug: slug, Name: name})
		}
		sort.Slice(rows, func(i, j int) bool { return rows[i].Slug < rows[j].Slug })
		if err := rn.store.UpsertPlatforms(ctx, rows); err != nil {
			rn.stop(ctx, err)
			return nil, sh, false
		}
	}
	rn.log.Info("Portals networks", "step", "collect.portals", "n", 5, "count", len(sh.networks), "networks", sh.networks)
	return resolved, sh, true
}

// networkRows lists Portals networks with the CMC network names that settings map to them.
func (rn *run) networkRows(networks []string) []store.NetworkRow {
	names := map[string][]string{}
	for cmcName, slug := range rn.settings.ChainMapping {
		if slug != nil {
			names[*slug] = append(names[*slug], cmcName)
		}
	}
	rows := make([]store.NetworkRow, 0, len(networks))
	for _, n := range networks {
		cn := names[n]
		sort.Strings(cn)
		rows = append(rows, store.NetworkRow{Slug: n, CMCNames: cn})
	}
	return rows
}

func (rn *run) upsertIssuer(ctx context.Context, iss cmc.Issuer, label *string) error {
	roster, err := json.Marshal(iss.Roster)
	if err != nil {
		return fmt.Errorf("encode issuer roster: %w", err)
	}
	var fetched *time.Time
	if t, err := time.Parse(timeFormat, iss.FetchedAt); err == nil {
		fetched = &t
	}
	size := iss.RosterSize
	return rn.store.UpsertIssuers(ctx, []store.IssuerRow{{ID: iss.IssuerID, Name: iss.Name, CMCLabel: label, Website: iss.Website,
		CMCLogoURL: iss.Logo, RosterSize: &size, Roster: roster, MetadataStatus: "cmc", FetchedAt: fetched}})
}

// formatIDs renders ids like 1.x printed a Python list: [39002, 39839].
func formatIDs(ids []int) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = strconv.Itoa(id)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func uniqueInts(in []int) []int {
	out := slices.Clone(in)
	slices.Sort(out)
	return slices.Compact(out)
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func ptr[T any](v T) *T { return &v }

// fmtDuration is 1.x _fmt_seconds: whole seconds below 90 s, minutes above.
func fmtDuration(d time.Duration) string {
	if s := d.Seconds(); s >= 90 {
		return fmt.Sprintf("%.1fm", s/60)
	}
	return fmt.Sprintf("%.0fs", d.Seconds())
}
