package publish_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/model"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/publish"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/store"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/testdb"
)

const snapshotsDir = "../../testdata/snapshots/v1"

func isin(s string) *string { return &s }

var assets = []model.Asset{
	{Ticker: "NVDA", Name: "NVIDIA", AssetType: "stock", ISIN: isin("US67066G1040"), Enabled: true},
	{Ticker: "AAPL", Name: "Apple", AssetType: "stock", ISIN: isin("US0378331005"), Enabled: true},
	{Ticker: "TSLA", Name: "Tesla", AssetType: "stock", ISIN: isin("US88160R1014"), Enabled: true},
}

type env struct {
	pool *pgxpool.Pool
	st   *store.Store
	pub  *publish.Publisher
}

func newEnv(t *testing.T) *env {
	t.Helper()
	pool, _ := testdb.Migrated(t)
	settings, err := model.LoadSettings(filepath.Join("..", "..", "config", "settings.json"))
	require.NoError(t, err)
	st := store.New(pool)
	_, err = st.SyncUnderlyings(context.Background(), assets)
	require.NoError(t, err)
	return &env{pool: pool, st: st, pub: publish.New(pool, settings, assets, slog.New(slog.DiscardHandler))}
}

// addRun creates a run and stores the 1.x snapshots of the given assets under it.
func (e *env) addRun(t *testing.T, key, mode string, idx ...int) int64 {
	t.Helper()
	ctx := context.Background()
	runID, err := e.st.CreateRun(ctx, store.RunParams{RunKey: key, Mode: mode, Trigger: "cli", StartedAt: time.Now(), CollectorVersion: "test", Settings: []byte("{}")})
	require.NoError(t, err)
	for _, i := range idx {
		payload, err := os.ReadFile(filepath.Join(snapshotsDir, assets[i].StorageID()+".json"))
		require.NoError(t, err)
		snap, err := model.ParseSnapshot(payload)
		require.NoError(t, err)
		_, err = e.st.InsertSnapshot(ctx, runID, assets[i].ID(), snap.FetchedAt, snap.Status, payload)
		require.NoError(t, err)
	}
	return runID
}

func (e *env) count(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	require.NoError(t, e.pool.QueryRow(context.Background(), query, args...).Scan(&n))
	return n
}

func TestPublishTwiceAndFailure(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()

	run1 := e.addRun(t, "r1", "refresh", 0, 1)
	res1, err := e.pub.Publish(ctx, run1, "refresh")
	require.NoError(t, err)
	require.NotZero(t, res1.BuildID)

	cur, err := e.st.CurrentBuild(ctx)
	require.NoError(t, err)
	require.Equal(t, res1.BuildID, cur.ID)
	var exp model.Export
	require.NoError(t, json.Unmarshal(cur.Export, &exp))
	require.Len(t, exp.Underlyings, 2)
	require.NotEmpty(t, exp.Tokens)
	require.NotNil(t, exp.RunID)
	require.Equal(t, run1, *exp.RunID)
	require.Contains(t, res1.Line(), "2 underlyings")

	for _, table := range []string{"summary_history", "underlying_history", "token_history", "issuer_history", "position_history"} {
		require.Positive(t, e.count(t, "SELECT count(*) FROM "+table+" WHERE run_id = $1", run1), table)
	}
	require.Equal(t, 1, e.count(t, "SELECT count(*) FROM summary_history"))
	require.Equal(t, len(exp.Tokens), e.count(t, "SELECT count(*) FROM tokens"))

	var firstBuild bool
	require.NoError(t, e.pool.QueryRow(ctx, `SELECT (details->>'first_build')::boolean FROM build_changes WHERE build_id = $1`, res1.BuildID).Scan(&firstBuild))
	require.True(t, firstBuild)

	// Second publish with a third underlying: new build is current and reports the new tokens.
	run2 := e.addRun(t, "r2", "add", 2)
	res2, err := e.pub.Publish(ctx, run2, "add")
	require.NoError(t, err)
	cur, err = e.st.CurrentBuild(ctx)
	require.NoError(t, err)
	require.Equal(t, res2.BuildID, cur.ID)
	require.Equal(t, 2, e.count(t, "SELECT count(*) FROM builds"))
	require.Equal(t, 1, e.count(t, "SELECT count(*) FROM builds WHERE is_current"))
	var ch store.BuildChangesRow
	require.NoError(t, e.pool.QueryRow(ctx, `SELECT previous_build_id, new_tokens, new_positions_count FROM build_changes WHERE build_id = $1`, res2.BuildID).
		Scan(&ch.PreviousBuildID, &ch.NewTokens, &ch.NewPositionsCount))
	require.NotNil(t, ch.PreviousBuildID)
	require.Equal(t, res1.BuildID, *ch.PreviousBuildID)
	require.NotEmpty(t, ch.NewTokens)
	require.Equal(t, 2, e.count(t, "SELECT count(*) FROM summary_history"))

	// A rebuild writes a build but no history.
	run3 := e.addRun(t, "r3", "rebuild")
	res3, err := e.pub.Publish(ctx, run3, "rebuild")
	require.NoError(t, err)
	require.Equal(t, 0, e.count(t, "SELECT count(*) FROM summary_history WHERE run_id = $1", run3))
	require.Equal(t, 0, e.count(t, "SELECT count(*) FROM token_history WHERE run_id = $1", run3))
	require.Equal(t, 2, e.count(t, "SELECT count(*) FROM summary_history"))

	// Failure: a run id that does not exist violates a foreign key; the previous build stays current.
	_, err = e.pub.Publish(ctx, 999999, "refresh")
	require.Error(t, err)
	cur, err = e.st.CurrentBuild(ctx)
	require.NoError(t, err)
	require.Equal(t, res3.BuildID, cur.ID)
	require.Equal(t, 3, e.count(t, "SELECT count(*) FROM builds"))

	// Cancelled context: nothing changes either.
	cctx, cancel := context.WithCancel(ctx)
	cancel()
	_, err = e.pub.Publish(cctx, run3, "rebuild")
	require.Error(t, err)
	cur, err = e.st.CurrentBuild(ctx)
	require.NoError(t, err)
	require.Equal(t, res3.BuildID, cur.ID)
}

func TestExportCarriesItsBuildID(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	for i, key := range []string{"r1", "r2"} {
		run := e.addRun(t, key, "refresh", 0, 1)
		res, err := e.pub.Publish(ctx, run, "refresh")
		require.NoError(t, err)

		var buildID int64
		var stored []byte
		var sha string
		require.NoError(t, e.pool.QueryRow(ctx, `SELECT id, export, export_sha256 FROM builds WHERE id = $1`, res.BuildID).Scan(&buildID, &stored, &sha))
		var exp model.Export
		require.NoError(t, json.Unmarshal(stored, &exp))
		require.NotNil(t, exp.BuildID, "export %d has build_id null", i)
		require.Equal(t, buildID, *exp.BuildID)
		sum := sha256.Sum256(stored)
		require.Equal(t, hex.EncodeToString(sum[:]), sha, "export_sha256 matches the stored bytes")
		require.Equal(t, sha, res.ExportSHA256)
		require.Equal(t, len(stored), res.ExportBytes)
	}
}

// A failure after the dimensions and the history were written (here: the build_changes insert) leaves nothing behind.
func TestPublishRollsBackDimensionsAndHistory(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	run := e.addRun(t, "r1", "refresh", 0, 1)
	_, err := e.pool.Exec(ctx, `
		CREATE FUNCTION fail_changes() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected failure'; END $$;
		CREATE TRIGGER fail_changes BEFORE INSERT ON build_changes FOR EACH ROW EXECUTE FUNCTION fail_changes();`)
	require.NoError(t, err)

	_, err = e.pub.Publish(ctx, run, "refresh")
	require.ErrorContains(t, err, "injected failure")
	for _, table := range []string{"summary_history", "underlying_history", "token_history", "issuer_history", "position_history",
		"tokens", "deployments", "positions", "token_positions", "issuers", "builds", "build_changes"} {
		require.Zero(t, e.count(t, "SELECT count(*) FROM "+table), table)
	}
	require.Zero(t, e.count(t, `SELECT count(*) FROM underlyings WHERE last_run_id IS NOT NULL`), "dimension updates rolled back")
	var warnings string
	require.NoError(t, e.pool.QueryRow(ctx, `SELECT warnings::text FROM collection_runs WHERE id = $1`, run).Scan(&warnings))
	require.JSONEq(t, "[]", warnings)

	// Without the fault the same run publishes.
	_, err = e.pool.Exec(ctx, `DROP TRIGGER fail_changes ON build_changes`)
	require.NoError(t, err)
	_, err = e.pub.Publish(ctx, run, "refresh")
	require.NoError(t, err)
	require.Equal(t, 1, e.count(t, "SELECT count(*) FROM builds"))
	require.Positive(t, e.count(t, "SELECT count(*) FROM token_history"))
}

func TestConcurrentPublishesAreSerialized(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	runs := []int64{e.addRun(t, "r1", "refresh", 0), e.addRun(t, "r2", "refresh", 1), e.addRun(t, "r3", "refresh", 2)}
	var wg sync.WaitGroup
	errs := make([]error, len(runs))
	for i, r := range runs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = e.pub.Publish(ctx, r, "refresh")
		}()
	}
	wg.Wait()
	for _, err := range errs {
		require.NoError(t, err)
	}
	require.Equal(t, 3, e.count(t, "SELECT count(*) FROM builds"))
	require.Equal(t, 1, e.count(t, "SELECT count(*) FROM builds WHERE is_current"))
	// Each build's previous_build_id is the build that was current before it: a chain, no two builds share a parent.
	require.Equal(t, 1, e.count(t, "SELECT count(*) FROM build_changes WHERE previous_build_id IS NULL"))
	require.Equal(t, 3, e.count(t, "SELECT count(DISTINCT coalesce(previous_build_id, 0)) FROM build_changes"))
}

func TestRebuildKeysDoNotCollideWithinASecond(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	run := e.addRun(t, "r1", "refresh", 0, 1)
	_, err := e.pub.Publish(ctx, run, "refresh")
	require.NoError(t, err)
	for range 3 {
		_, _, err := e.pub.Rebuild(ctx, publish.RebuildParams{Trigger: "cli", CollectorVersion: "test", SettingsJSON: []byte("{}")})
		require.NoError(t, err)
	}
	require.Equal(t, 3, e.count(t, "SELECT count(*) FROM collection_runs WHERE mode = 'rebuild'"))
}

// The publish committed, then the caller's context ended: the rebuild run must still be finished.
func TestRebuildFinishesRunAfterContextCancel(t *testing.T) {
	e := newEnv(t)
	run := e.addRun(t, "r1", "refresh", 0, 1)
	_, err := e.pub.Publish(context.Background(), run, "refresh")
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	e.pub.SetAfterPublishHook(cancel)
	runID, res, err := e.pub.Rebuild(ctx, publish.RebuildParams{Trigger: "cli", CollectorVersion: "test", SettingsJSON: []byte("{}")})
	require.NoError(t, err)
	require.NotNil(t, res)
	var status string
	require.NoError(t, e.pool.QueryRow(context.Background(), `SELECT status FROM collection_runs WHERE id = $1`, runID).Scan(&status))
	require.Equal(t, "succeeded", status)
}

// A run that refreshed one underlying adds history points for it only; summary and issuer history are written per run.
func TestHistoryOnlyForRefreshedUnderlyings(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	runA := e.addRun(t, "a", "refresh", 0, 1)
	_, err := e.pub.Publish(ctx, runA, "refresh")
	require.NoError(t, err)

	// Run B: a newer snapshot of NVDA only (same payload, later fetched_at).
	runB, err := e.st.CreateRun(ctx, store.RunParams{RunKey: "b", Mode: "refresh", Trigger: "cli", StartedAt: time.Now(), CollectorVersion: "test", Settings: []byte("{}")})
	require.NoError(t, err)
	payload, err := os.ReadFile(filepath.Join(snapshotsDir, assets[0].StorageID()+".json"))
	require.NoError(t, err)
	snap, err := model.ParseSnapshot(payload)
	require.NoError(t, err)
	newer := snap.FetchedAt.Add(24 * time.Hour)
	var doc map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(payload, &doc))
	doc["fetched_at"], err = json.Marshal(newer.UTC().Format("2006-01-02T15:04:05Z"))
	require.NoError(t, err)
	payload, err = json.Marshal(doc)
	require.NoError(t, err)
	snap, err = model.ParseSnapshot(payload)
	require.NoError(t, err)
	_, err = e.st.InsertSnapshot(ctx, runB, assets[0].ID(), snap.FetchedAt, snap.Status, payload)
	require.NoError(t, err)
	_, err = e.pub.Publish(ctx, runB, "refresh")
	require.NoError(t, err)

	uh := func(id string) int {
		return e.count(t, "SELECT count(*) FROM underlying_history WHERE underlying_id = $1", id)
	}
	require.Equal(t, 2, uh(assets[0].ID()))
	require.Equal(t, 1, uh(assets[1].ID()))
	require.Zero(t, e.count(t, "SELECT count(*) FROM underlying_history WHERE run_id = $1 AND underlying_id = $2", runB, assets[1].ID()))

	// Tokens and positions of the refreshed underlying get a second point, the others do not.
	require.Positive(t, e.count(t, "SELECT count(*) FROM token_history WHERE run_id = $1", runB))
	require.Positive(t, e.count(t, "SELECT count(*) FROM position_history WHERE run_id = $1", runB))
	require.Less(t, e.count(t, "SELECT count(*) FROM token_history WHERE run_id = $1", runB), e.count(t, "SELECT count(*) FROM token_history WHERE run_id = $1", runA))
	require.Zero(t, e.count(t, `SELECT count(*) FROM (SELECT token_id, observed_at FROM token_history GROUP BY 1, 2 HAVING count(*) > 1) d`), "no duplicate points")
	require.Zero(t, e.count(t, `SELECT count(*) FROM (SELECT position_key, observed_at FROM position_history GROUP BY 1, 2 HAVING count(*) > 1) d`), "no duplicate points")

	// Aggregates: one row per run.
	for _, run := range []int64{runA, runB} {
		require.Equal(t, 1, e.count(t, "SELECT count(*) FROM summary_history WHERE run_id = $1", run))
		require.Positive(t, e.count(t, "SELECT count(*) FROM issuer_history WHERE run_id = $1", run))
	}
	require.Equal(t, e.count(t, "SELECT count(*) FROM issuer_history WHERE run_id = $1", runA), e.count(t, "SELECT count(*) FROM issuer_history WHERE run_id = $1", runB))
}
