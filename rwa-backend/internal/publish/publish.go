// Package publish turns the latest stored snapshots into a build: it runs compute, writes the current-state
// tables and the history in one transaction, and makes the new build current. If anything fails the transaction
// rolls back and the previous build stays current.
package publish

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/compute"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/curated"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/db"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/model"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/runlog"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/store"
)

// ModeRebuild is the run mode of a recomputation from stored snapshots. It writes no history: a rebuild has no new
// observation, so a history point would duplicate the previous one.
const ModeRebuild = "rebuild"

// Options tunes one publication. History of a collection run covers only the underlyings (and their tokens and
// positions) the run refreshed, plus the basket and issuer aggregates; see keepRunHistory.
type Options struct {
	// WriteHistory inserts the *_history rows of the run. Collection and import runs do; rebuilds do not.
	WriteHistory bool
}

// OptionsForMode returns the default options of a run mode: history for everything but a rebuild.
func OptionsForMode(mode string) Options { return Options{WriteHistory: mode != ModeRebuild} }

// Result describes a published build.
type Result struct {
	BuildID      int64
	RunID        int64
	ExportSHA256 string
	ExportBytes  int
	Warnings     []string
	Summary      model.Summary
}

// ExcludedProfilesWarning is the run warning of a build that excluded n rights profiles.
func ExcludedProfilesWarning(n int) string {
	return fmt.Sprintf("%d rights profiles excluded (see journal)", n)
}

// Line is the 1.x style one-line summary printed by `rwa build`.
func (r *Result) Line() string {
	s := r.Summary
	return fmt.Sprintf("build %d: %d underlyings, %d tokens, %d issuers, %d confirmed / %d eligible opportunities",
		r.BuildID, s.UnderlyingsCount, s.TokensCount, s.IssuersCount, s.RawConfirmedOpportunitiesCount, s.EligibleOpportunitiesCount)
}

// Publisher builds and publishes exports.
type Publisher struct {
	pool     *pgxpool.Pool
	settings *model.Settings
	assets   []model.Asset // assets.json order: breaks fetched_at ties like 1.x
	log      *slog.Logger
	now      func() time.Time
	curated  *curated.Config // links, referrals and holder rights; nil = none of the files exist

	afterPublish func() // test seam: called by Rebuild right after its publish committed
}

// New returns a Publisher. assets is the assets.json list (its order is the tie-break for equal fetched_at).
func New(pool *pgxpool.Pool, settings *model.Settings, assets []model.Asset, log *slog.Logger) *Publisher {
	if log == nil {
		log = slog.Default()
	}
	return &Publisher{pool: pool, settings: settings, assets: assets, log: log, now: time.Now}
}

// WithCurated sets the curated config (links.json, rights_profiles.json, issuers.json). Without it the export carries
// no links and no rights profiles, as if the files did not exist.
func (p *Publisher) WithCurated(c *curated.Config) *Publisher {
	p.curated = c
	return p
}

// WithClock overrides the clock: the build time is generated_at, the instant links freshness is judged at and,
// by its UTC date, "today" for rights reviews (tests, parity).
func (p *Publisher) WithClock(now func() time.Time) *Publisher {
	p.now = now
	return p
}

// Publish publishes a build for runID in its own transaction. mode is the run mode: a rebuild writes no history.
func (p *Publisher) Publish(ctx context.Context, runID int64, mode string) (*Result, error) {
	return p.PublishWith(ctx, runID, OptionsForMode(mode))
}

// PublishWith is Publish with explicit options.
func (p *Publisher) PublishWith(ctx context.Context, runID int64, opts Options) (*Result, error) {
	log := p.log.With("run_id", runID, "step", "publish")
	started := time.Now()
	log.Info("publish started", "history", opts.WriteHistory)
	var res *Result
	err := db.WithTx(ctx, p.pool, func(tx pgx.Tx) error {
		var err error
		res, err = p.PublishTx(ctx, tx, runID, opts)
		return err
	})
	if err != nil {
		log.Error("publish failed, the previous build stays current", "err", err, "duration", time.Since(started))
		runlog.Flush(p.log)
		return nil, err
	}
	for _, w := range res.Warnings {
		log.Warn(w)
	}
	log.Info("build published", "build_id", res.BuildID, "bytes", res.ExportBytes, "history", opts.WriteHistory,
		"warnings", len(res.Warnings), "duration", time.Since(started), "summary", res.Line())
	runlog.Flush(p.log)
	return res, nil
}

// PublishTx publishes inside the caller's transaction, which also sees the caller's uncommitted writes (the
// importer stores its snapshots and publishes in one transaction). The caller commits or rolls back.
//
// It first takes the transaction-scoped advisory lock hashtext('rwa-publish'): publishes (collector, `rwa build`,
// import) are serialized, so the previous export read below is the one this build replaces.
func (p *Publisher) PublishTx(ctx context.Context, tx pgx.Tx, runID int64, opts Options) (*Result, error) {
	st := store.New(tx)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('rwa-publish'))`); err != nil {
		return nil, fmt.Errorf("publish: lock: %w", err)
	}

	snapRows, err := st.LatestSnapshotsForEnabled(ctx)
	if err != nil {
		return nil, fmt.Errorf("publish: %w", err)
	}
	if len(snapRows) == 0 {
		return nil, errors.New("publish: no snapshots for enabled underlyings")
	}
	snaps, err := p.parseSnapshots(snapRows)
	if err != nil {
		return nil, err
	}

	issuerRows, err := st.ListIssuers(ctx)
	if err != nil {
		return nil, fmt.Errorf("publish: %w", err)
	}
	platformNames, err := st.ListPlatformNames(ctx)
	if err != nil {
		return nil, fmt.Errorf("publish: %w", err)
	}
	infoRows, err := st.ListUnderlyingInfo(ctx)
	if err != nil {
		return nil, fmt.Errorf("publish: %w", err)
	}
	logoExts, err := st.LogoExts(ctx)
	if err != nil {
		return nil, fmt.Errorf("publish: %w", err)
	}

	rwaInfo := make(map[string]compute.RwaInfo, len(infoRows))
	for _, r := range infoRows {
		rwaInfo[strconv.Itoa(r.CMCRwaID)] = compute.RwaInfo{Website: r.Website, Logo: r.CMCLogoURL, Industry: r.Industry}
	}

	// Same order as export.py::build_export_with_warnings: build, finalize, links, rights; one instant for all.
	now := p.now()
	result, err := compute.Build(snaps, issuerCache(issuerRows), p.settings, platformNames, compute.WithSymbolAliases(compute.SymbolAliases(p.assets)))
	if err != nil {
		return nil, fmt.Errorf("publish: compute: %w", err)
	}
	exp := compute.Finalize(result, now, rwaInfo, compute.MapLogos(logoExts))
	var deals *curated.DealsCache
	switch payload, err := st.LatestDeals(ctx); {
	case err == nil:
		deals = curated.ParseDealsCache(payload)
	case !errors.Is(err, store.ErrNotFound):
		return nil, fmt.Errorf("publish: %w", err)
	}
	curatedWarnings := curated.Apply(exp, p.curated, deals, result.Side.PositionMetadata, now)
	exp.RunID = &runID
	// The export names its own build: reserve the id now, insert the build with it below.
	buildID, err := st.ReserveBuildID(ctx)
	if err != nil {
		return nil, fmt.Errorf("publish: %w", err)
	}
	exp.BuildID = &buildID

	export, err := json.Marshal(exp)
	if err != nil {
		return nil, fmt.Errorf("publish: marshal export: %w", err)
	}
	sum := sha256.Sum256(export)
	shaHex := hex.EncodeToString(sum[:])

	dims, err := compute.DimensionRows(result, runID, p.settings, platformNames, rwaInfo)
	if err != nil {
		return nil, fmt.Errorf("publish: %w", err)
	}
	warnings := slices.Clone(dims.Warnings)
	for _, w := range curatedWarnings {
		if !slices.Contains(warnings, w) {
			warnings = append(warnings, w)
		}
	}

	if n := exp.Summary.RightsExcludedProfilesCount; n > 0 {
		// The per-profile reasons are in the journal (the rights warnings); the run carries one summary line.
		warnings = append(warnings, ExcludedProfilesWarning(n))
	}

	if err := writeDimensions(ctx, st, runID, dims); err != nil {
		return nil, err
	}
	if opts.WriteHistory {
		hist, err := compute.HistoryRows(exp, &result.Side, runID)
		if err != nil {
			return nil, fmt.Errorf("publish: %w", err)
		}
		keepRunHistory(hist, snapRows, runID)
		if err := writeHistory(ctx, st, hist); err != nil {
			return nil, err
		}
		for _, w := range hist.Warnings {
			if !slices.Contains(warnings, w) {
				warnings = append(warnings, w)
			}
		}
	}

	prev, prevID, err := previousExport(ctx, st)
	if err != nil {
		return nil, err
	}
	changes := compute.Changes(prev, exp)
	if prev != nil {
		changes.PreviousBuildID = &prevID
	}

	settingsJSON, err := json.Marshal(exp.Settings)
	if err != nil {
		return nil, fmt.Errorf("publish: marshal settings: %w", err)
	}
	summaryJSON, err := json.Marshal(exp.Summary)
	if err != nil {
		return nil, fmt.Errorf("publish: marshal summary: %w", err)
	}
	generatedAt, ok := compute.ParseTime(exp.GeneratedAt)
	if !ok {
		return nil, fmt.Errorf("publish: bad generated_at %q", exp.GeneratedAt)
	}
	if _, err := st.PublishBuild(ctx, tx, store.BuildRow{
		ID: buildID, RunID: runID, GeneratedAt: generatedAt, SchemaVersion: exp.SchemaVersion, ScoreVersion: exp.ScoreVersion,
		Settings: settingsJSON, Summary: summaryJSON, Export: export, ExportSHA256: shaHex,
	}, &changes); err != nil {
		return nil, fmt.Errorf("publish: %w", err)
	}
	if err := st.AppendRunWarnings(ctx, runID, warnings); err != nil {
		return nil, fmt.Errorf("publish: %w", err)
	}
	return &Result{BuildID: buildID, RunID: runID, ExportSHA256: shaHex, ExportBytes: len(export), Warnings: warnings, Summary: exp.Summary}, nil
}

// keepRunHistory drops the underlying, token and position history rows whose source snapshot is not from run runID.
// The export is computed from the latest snapshot of every underlying, so a `--symbols` run would otherwise rewrite
// the points of all other underlyings at their old observed_at, duplicating them under a new run_id. The entity rows
// carry the fetched_at of their source snapshot (a position: of the newest snapshot that supplied it), so a row is kept
// when that time equals the fetched_at of a snapshot of this run. Summary and issuer history are basket aggregates and
// are always written. An import run owns every snapshot, so it keeps everything.
func keepRunHistory(h *compute.History, snapRows []store.SnapshotRow, runID int64) {
	own := map[time.Time]bool{}
	for _, r := range snapRows {
		if r.RunID == runID {
			// Exports carry fetched_at formatted; compare at that precision.
			if t, ok := compute.ParseTime(compute.FormatTime(r.FetchedAt)); ok {
				own[t] = true
			}
		}
	}
	h.Underlyings = slices.DeleteFunc(h.Underlyings, func(r store.UnderlyingHistoryRow) bool { return !own[r.ObservedAt] })
	h.Tokens = slices.DeleteFunc(h.Tokens, func(r store.TokenHistoryRow) bool { return !own[r.ObservedAt] })
	h.Positions = slices.DeleteFunc(h.Positions, func(r store.PositionHistoryRow) bool { return !own[r.ObservedAt] })
}

// parseSnapshots parses the payloads in assets.json order (the order compute expects for equal fetched_at);
// underlyings missing from assets keep their relative order at the end.
func (p *Publisher) parseSnapshots(rows []store.SnapshotRow) ([]*model.Snapshot, error) {
	pos := make(map[string]int, len(p.assets))
	for i, a := range p.assets {
		pos[a.ID()] = i
	}
	rows = slices.Clone(rows)
	slices.SortStableFunc(rows, func(a, b store.SnapshotRow) int {
		pa, oka := pos[a.UnderlyingID]
		pb, okb := pos[b.UnderlyingID]
		switch {
		case oka && okb:
			return pa - pb
		case oka:
			return -1
		case okb:
			return 1
		}
		return 0
	})
	snaps := make([]*model.Snapshot, 0, len(rows))
	for _, r := range rows {
		s, err := model.ParseSnapshot(r.Payload)
		if err != nil {
			return nil, fmt.Errorf("publish: snapshot of %s: %w", r.UnderlyingID, err)
		}
		snaps = append(snaps, s)
	}
	return snaps, nil
}

// issuerCache rebuilds the issuers.json cache from the issuers table: only rows with CMC metadata are cache entries
// (stub rows, metadata_status unknown, stand for issuers the cache never had).
func issuerCache(rows []store.IssuerRow) map[string]compute.IssuerMeta {
	out := map[string]compute.IssuerMeta{}
	for _, r := range rows {
		if r.MetadataStatus != "cmc" {
			continue
		}
		m := compute.IssuerMeta{Name: r.Name, Website: r.Website, Logo: r.CMCLogoURL, RosterSize: r.RosterSize, Roster: r.Roster}
		if r.FetchedAt != nil {
			s := compute.FormatTime(*r.FetchedAt)
			m.FetchedAt = &s
		}
		out[r.ID] = m
	}
	return out
}

func writeDimensions(ctx context.Context, st *store.Store, runID int64, d *compute.Dimensions) error {
	steps := []struct {
		name string
		fn   func() error
	}{
		{"underlying facts", func() error { return st.UpsertUnderlyingFacts(ctx, d.UnderlyingFacts) }},
		{"issuers", func() error { return st.UpsertIssuers(ctx, d.Issuers) }},
		{"tokens", func() error { return st.UpsertTokens(ctx, runID, d.Tokens) }},
		{"deployments", func() error { return st.UpsertDeployments(ctx, runID, d.Deployments) }},
		{"platforms", func() error { return st.UpsertPlatforms(ctx, d.Platforms) }},
		{"networks", func() error { return st.UpsertNetworks(ctx, runID, d.Networks) }},
		{"positions", func() error { return st.UpsertPositions(ctx, runID, d.Positions) }},
		{"token positions", func() error { return st.UpsertTokenPositions(ctx, runID, d.TokenPositions) }},
		{"deactivate tokens", func() error { _, err := st.DeactivateMissingTokens(ctx, runID); return err }},
		{"deactivate deployments", func() error { _, err := st.DeactivateMissingDeployments(ctx, runID); return err }},
		{"deactivate networks", func() error { _, err := st.DeactivateMissingNetworks(ctx, runID); return err }},
		{"deactivate positions", func() error { _, err := st.DeactivateMissingPositions(ctx, runID); return err }},
		{"deactivate token positions", func() error { _, err := st.DeactivateMissingTokenPositions(ctx, runID); return err }},
	}
	for _, s := range steps {
		if err := s.fn(); err != nil {
			return fmt.Errorf("publish: write %s: %w", s.name, err)
		}
	}
	return nil
}

func writeHistory(ctx context.Context, st *store.Store, h *compute.History) error {
	if err := st.InsertSummaryHistory(ctx, []store.SummaryHistoryRow{h.Summary}); err != nil {
		return fmt.Errorf("publish: write summary history: %w", err)
	}
	if err := st.InsertUnderlyingHistory(ctx, h.Underlyings); err != nil {
		return fmt.Errorf("publish: write underlying history: %w", err)
	}
	if err := st.InsertTokenHistory(ctx, h.Tokens); err != nil {
		return fmt.Errorf("publish: write token history: %w", err)
	}
	if err := st.InsertIssuerHistory(ctx, h.Issuers); err != nil {
		return fmt.Errorf("publish: write issuer history: %w", err)
	}
	if err := st.InsertPositionHistory(ctx, h.Positions); err != nil {
		return fmt.Errorf("publish: write position history: %w", err)
	}
	return nil
}

// previousExport loads the current build's export and id, or nil when there is none.
func previousExport(ctx context.Context, st *store.Store) (*model.Export, int64, error) {
	b, err := st.CurrentBuild(ctx)
	if errors.Is(err, store.ErrNotFound) {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, fmt.Errorf("publish: previous build: %w", err)
	}
	var prev model.Export
	if err := json.Unmarshal(b.Export, &prev); err != nil {
		return nil, 0, fmt.Errorf("publish: parse previous build %d: %w", b.ID, err)
	}
	return &prev, b.ID, nil
}
