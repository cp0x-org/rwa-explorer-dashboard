// Package importer loads the data of the 1.x pipeline (rwa-data-pipeline/data) into the database as one
// run with mode=import, then publishes a build from it. Snapshot documents are stored byte for byte; their
// fetched_at starts the history at the 1.x collection date, not at the date of the move.
package importer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/compute"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/db"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/model"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/publish"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/store"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/v1files"
)

// Mode and trigger of the import run.
const (
	Mode    = "import"
	Trigger = "import"
	// logoSource is logos.source of imported files.
	logoSource = "import-v1"
)

// Result describes an import.
type Result struct {
	RunID     int64
	RunKey    string
	Skipped   bool // a run with this run_key already exists: nothing was changed
	Snapshots int
	Logos     int
	Build     *publish.Result
}

// Importer imports 1.x data.
type Importer struct {
	pool         *pgxpool.Pool
	assets       []model.Asset
	settings     *model.Settings
	settingsJSON []byte // settings.json as read, stored in collection_runs.settings
	version      string
	log          *slog.Logger
	now          func() time.Time
	force        bool
}

// ErrNotEmpty is returned when the database already has collection runs and the import was not forced.
var ErrNotEmpty = errors.New("importer: the database already has collection runs; the import is meant for an empty database (use --force to import anyway)")

// WithForce allows importing into a database that already has collection runs (the imported snapshots become the
// newest data of their underlyings only if they are newer than the stored ones).
func (im *Importer) WithForce(force bool) *Importer {
	im.force = force
	return im
}

// New returns an Importer. settingsJSON is the content of settings.json (recorded on the run).
func New(pool *pgxpool.Pool, assets []model.Asset, settings *model.Settings, settingsJSON []byte, version string, log *slog.Logger) *Importer {
	if log == nil {
		log = slog.Default()
	}
	return &Importer{pool: pool, assets: assets, settings: settings, settingsJSON: settingsJSON, version: version, log: log, now: time.Now}
}

// v1Run is the part of data/runs/<run_id>.json the importer uses.
type v1Run struct {
	RunID      string   `json:"run_id"`
	StartedAt  string   `json:"started_at"`
	Warnings   []string `json:"warnings"`
	CMCCredits int      `json:"cmc_credits"`
	Requests   struct {
		CMC     int `json:"cmc"`
		Portals int `json:"portals"`
	} `json:"requests"`
}

// newestV1Run reads the newest data/runs/*.json (run ids are timestamps, so the last name is the newest).
func newestV1Run(dataDir string) (*v1Run, error) {
	names, err := filepath.Glob(filepath.Join(dataDir, "runs", "*.json"))
	if err != nil {
		return nil, fmt.Errorf("importer: list runs: %w", err)
	}
	if len(names) == 0 {
		return nil, nil
	}
	slices.Sort(names)
	b, err := os.ReadFile(names[len(names)-1])
	if err != nil {
		return nil, fmt.Errorf("importer: read run file: %w", err)
	}
	var r v1Run
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, fmt.Errorf("importer: parse %s: %w", names[len(names)-1], err)
	}
	if r.RunID == "" {
		r.RunID = strings.TrimSuffix(filepath.Base(names[len(names)-1]), ".json")
	}
	return &r, nil
}

// Import imports dataDir (the 1.x rwa-data-pipeline/data folder) in one transaction. It is idempotent: when a run with
// the run_key of the newest 1.x run exists, it changes nothing and returns Result{Skipped: true}.
func (im *Importer) Import(ctx context.Context, dataDir string) (*Result, error) {
	now := im.now().UTC()
	v1, err := newestV1Run(dataDir)
	if err != nil {
		return nil, err
	}
	runKey := "import-" + now.Format("20060102T150405Z")
	startedAt := now
	if v1 != nil {
		runKey = v1.RunID
		if t, ok := compute.ParseTime(v1.StartedAt); ok {
			startedAt = t
		}
	} else if key, at, err := im.newestSnapshotRun(dataDir); err != nil {
		return nil, err
	} else if key != "" {
		// No data/runs/*.json: the newest snapshot names the 1.x run it came from.
		runKey, startedAt = key, at
	}
	lock, err := db.TryLock(ctx, im.pool, db.CollectorLockName, im.log)
	if errors.Is(err, db.ErrLocked) {
		return nil, errors.New("importer: a collector run is in progress; try again when it has finished")
	}
	if err != nil {
		return nil, fmt.Errorf("importer: %w", err)
	}
	defer lock.Release(ctx)

	if id, err := store.New(im.pool).RunIDByKey(ctx, runKey); err == nil {
		im.log.Info("import skipped: a run with this run_key already exists", "run_key", runKey, "run_id", id)
		return &Result{RunID: id, RunKey: runKey, Skipped: true}, nil
	} else if !errors.Is(err, store.ErrNotFound) {
		return nil, fmt.Errorf("importer: %w", err)
	}
	if !im.force {
		has, err := store.New(im.pool).HasRuns(ctx)
		if err != nil {
			return nil, fmt.Errorf("importer: %w", err)
		}
		if has {
			return nil, ErrNotEmpty
		}
	}

	in, err := loadInputs(dataDir)
	if err != nil {
		return nil, err
	}

	res := &Result{RunKey: runKey}
	err = db.WithTx(ctx, im.pool, func(tx pgx.Tx) error {
		return im.importTx(ctx, tx, in, v1, runKey, startedAt, res)
	})
	if err != nil {
		return nil, err
	}
	im.log.Info("import done", "run_id", res.RunID, "run_key", runKey, "snapshots", res.Snapshots, "logos", res.Logos,
		"build_id", res.Build.BuildID)
	return res, nil
}

// newestSnapshotRun returns the 1.x run_id and fetched_at of the newest snapshot of an enabled asset, or "" when no
// snapshot names a run.
func (im *Importer) newestSnapshotRun(dataDir string) (string, time.Time, error) {
	var key string
	var at time.Time
	for _, a := range im.assets {
		if !a.Enabled {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dataDir, "snapshots", a.StorageID()+".json")) //nolint:gosec // operator-provided data dir
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return "", time.Time{}, fmt.Errorf("importer: read snapshot %s: %w", a.Ticker, err)
		}
		snap, err := model.ParseSnapshot(b)
		if err != nil {
			return "", time.Time{}, fmt.Errorf("importer: snapshot %s: %w", a.Ticker, err)
		}
		if snap.RunID != "" && snap.FetchedAt.After(at) {
			key, at = snap.RunID, snap.FetchedAt
		}
	}
	return key, at, nil
}

// inputs are the files read before the transaction starts.
type inputs struct {
	dataDir   string
	issuers   map[string]compute.IssuerMeta
	rwaInfo   map[string]compute.RwaInfo
	platforms map[string]string
}

func loadInputs(dataDir string) (*inputs, error) {
	in := &inputs{dataDir: dataDir}
	var err error
	if in.issuers, err = v1files.LoadIssuers(filepath.Join(dataDir, "issuers.json")); err != nil {
		return nil, fmt.Errorf("importer: %w", err)
	}
	if in.rwaInfo, err = v1files.LoadRwaInfo(filepath.Join(dataDir, "rwa_info.json")); err != nil {
		return nil, fmt.Errorf("importer: %w", err)
	}
	if in.platforms, err = v1files.LoadPlatformNames(filepath.Join(dataDir, "portals_platforms.json")); err != nil {
		return nil, fmt.Errorf("importer: %w", err)
	}
	return in, nil
}

func (im *Importer) importTx(ctx context.Context, tx pgx.Tx, in *inputs, v1 *v1Run, runKey string, startedAt time.Time, res *Result) error {
	st := store.New(tx)
	runID, err := st.CreateRun(ctx, store.RunParams{RunKey: runKey, Mode: Mode, Trigger: Trigger, StartedAt: startedAt,
		CollectorVersion: im.version, Settings: im.settingsJSON})
	if err != nil {
		return fmt.Errorf("importer: %w", err)
	}
	res.RunID = runID

	if _, err := st.SyncUnderlyings(ctx, im.assets); err != nil {
		return fmt.Errorf("importer: %w", err)
	}

	var issuerRows []store.IssuerRow
	for _, id := range sortedKeys(in.issuers) {
		m := in.issuers[id]
		row := store.IssuerRow{ID: id, Name: m.Name, Website: m.Website, CMCLogoURL: m.Logo, RosterSize: m.RosterSize,
			Roster: m.Roster, MetadataStatus: "cmc"}
		if m.FetchedAt != nil {
			if t, ok := compute.ParseTime(*m.FetchedAt); ok {
				row.FetchedAt = &t
			}
		}
		issuerRows = append(issuerRows, row)
	}
	if err := st.UpsertIssuers(ctx, issuerRows); err != nil {
		return fmt.Errorf("importer: %w", err)
	}
	var platformRows []store.PlatformRow
	for _, slug := range sortedKeys(in.platforms) {
		platformRows = append(platformRows, store.PlatformRow{Slug: slug, Name: in.platforms[slug]})
	}
	if err := st.UpsertPlatforms(ctx, platformRows); err != nil {
		return fmt.Errorf("importer: %w", err)
	}

	if err := im.importSnapshots(ctx, st, in, runID, res); err != nil {
		return err
	}
	if res.Logos, err = importLogos(ctx, st, in.dataDir); err != nil {
		return err
	}

	var warnings []string
	counters := store.RunCounters{Selected: res.Snapshots, Completed: res.Snapshots}
	if v1 != nil {
		warnings = v1.Warnings
		counters.CMCRequests, counters.PortalsRequests, counters.CMCCredits = v1.Requests.CMC, v1.Requests.Portals, v1.CMCCredits
	}
	if err := st.FinishRun(ctx, runID, "succeeded", counters, warnings, ""); err != nil {
		return fmt.Errorf("importer: %w", err)
	}
	// Publish last: it appends its own warnings to the run and reads the snapshots written above.
	build, err := publish.New(im.pool, im.settings, im.assets, im.log).PublishTx(ctx, tx, runID, publish.Options{WriteHistory: true})
	if err != nil {
		return fmt.Errorf("importer: %w", err)
	}
	res.Build = build
	return nil
}

// importSnapshots stores the snapshot document of every enabled asset verbatim, its run_underlyings row, and the
// CMC facts of the underlying (rwa id and name from the snapshot; website, logo, industry from rwa_info.json).
func (im *Importer) importSnapshots(ctx context.Context, st *store.Store, in *inputs, runID int64, res *Result) error {
	var facts []store.UnderlyingFactsRow
	for _, a := range im.assets {
		if !a.Enabled {
			continue
		}
		payload, err := os.ReadFile(filepath.Join(in.dataDir, "snapshots", a.StorageID()+".json")) //nolint:gosec // operator-provided data dir
		if errors.Is(err, os.ErrNotExist) {
			im.log.Warn("no 1.x snapshot for an enabled asset, skipped", "ticker", a.Ticker)
			continue
		}
		if err != nil {
			return fmt.Errorf("importer: read snapshot %s: %w", a.Ticker, err)
		}
		snap, err := model.ParseSnapshot(payload)
		if err != nil {
			return fmt.Errorf("importer: snapshot %s: %w", a.Ticker, err)
		}
		if _, err := st.InsertSnapshot(ctx, runID, a.ID(), snap.FetchedAt, snap.Status, payload); err != nil {
			return fmt.Errorf("importer: %w", err)
		}
		fetched := snap.FetchedAt
		if err := st.UpsertRunUnderlying(ctx, store.RunUnderlying{RunID: runID, UnderlyingID: a.ID(), Status: snap.Status,
			StartedAt: &fetched, FinishedAt: &fetched}); err != nil {
			return fmt.Errorf("importer: %w", err)
		}
		status := snap.Status
		f := store.UnderlyingFactsRow{ID: a.ID(), Status: &status, LastRunID: runID}
		if snap.CMC != nil && snap.CMC.RWA != nil {
			rwa := snap.CMC.RWA
			f.CMCRwaID, f.CMCName = rwa.RwaID, rwa.Name
			if rwa.RwaID != nil {
				info := in.rwaInfo[strconv.Itoa(*rwa.RwaID)]
				f.Website, f.CMCLogoURL, f.Industry = info.Website, info.Logo, info.Industry
			}
		}
		facts = append(facts, f)
		res.Snapshots++
	}
	if err := st.UpsertUnderlyingFacts(ctx, facts); err != nil {
		return fmt.Errorf("importer: %w", err)
	}
	return nil
}

var logoContentTypes = map[string]string{
	"png": "image/png", "jpg": "image/jpeg", "jpeg": "image/jpeg", "svg": "image/svg+xml",
	"webp": "image/webp", "ico": "image/x-icon", "gif": "image/gif",
}

// importLogos copies data/logos/{tokens,issuers,underlyings}/<entity>.<ext> into the logos table. Files are visited
// in name order and an existing row is never replaced, so when an entity has several files the first by name wins,
// like logos.py::find_logo.
func importLogos(ctx context.Context, st *store.Store, dataDir string) (int, error) {
	n := 0
	for _, kind := range []string{"tokens", "issuers", "underlyings"} {
		dir := filepath.Join(dataDir, "logos", kind)
		entries, err := os.ReadDir(dir)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return n, fmt.Errorf("importer: list logos %s: %w", kind, err)
		}
		for _, e := range entries { // ReadDir sorts by file name
			// The extension keeps its case (X.PNG stays "PNG", as 1.x served it); the content type is looked up in lower
			// case, unknown extensions are stored as application/octet-stream. Files without an extension are skipped.
			ext := strings.TrimPrefix(filepath.Ext(e.Name()), ".")
			if e.IsDir() || strings.HasPrefix(e.Name(), ".") || ext == "" {
				continue
			}
			ct, ok := logoContentTypes[strings.ToLower(ext)]
			if !ok {
				ct = "application/octet-stream"
			}
			b, err := os.ReadFile(filepath.Join(dir, e.Name())) //nolint:gosec // operator-provided data dir
			if err != nil {
				return n, fmt.Errorf("importer: read logo %s/%s: %w", kind, e.Name(), err)
			}
			sum := sha256.Sum256(b)
			inserted, err := st.InsertLogoIfAbsent(ctx, store.Logo{Kind: kind, EntityID: strings.TrimSuffix(e.Name(), filepath.Ext(e.Name())),
				Ext: ext, ContentType: ct, Bytes: b, SHA256: hex.EncodeToString(sum[:]), Source: logoSource})
			if err != nil {
				return n, fmt.Errorf("importer: %w", err)
			}
			if inserted {
				n++
			}
		}
	}
	return n, nil
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
