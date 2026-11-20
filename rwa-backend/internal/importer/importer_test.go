package importer_test

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/compute"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/db"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/importer"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/model"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/store"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/testdb"
)

const dataDir = "../../../rwa-data-pipeline/data"

// TestImportV1Parity imports the real 1.x data and compares the stored export with the 1.x export: 0 differences.
func TestImportV1Parity(t *testing.T) {
	if _, err := os.Stat(filepath.Join(dataDir, "snapshots")); err != nil {
		t.Skipf("1.x data not found at %s: %v", dataDir, err)
	}
	cfg := filepath.Join("..", "..", "config")
	settings, err := model.LoadSettings(filepath.Join(cfg, "settings.json"))
	require.NoError(t, err)
	settingsJSON, err := os.ReadFile(filepath.Join(cfg, "settings.json"))
	require.NoError(t, err)
	assets, err := model.LoadAssets(filepath.Join(cfg, "assets.json"))
	require.NoError(t, err)
	want, err := os.ReadFile(filepath.Join("..", "..", "testdata", "golden", "rwa.json"))
	require.NoError(t, err)

	pool, _ := testdb.Migrated(t)
	ctx := context.Background()
	im := importer.New(pool, assets, settings, settingsJSON, "test", slog.New(slog.DiscardHandler))

	res, err := im.Import(ctx, dataDir)
	require.NoError(t, err)
	require.False(t, res.Skipped)
	require.Equal(t, "20261018T201453Z", res.RunKey)
	require.Equal(t, 125, res.Snapshots)
	require.Positive(t, res.Logos)

	st := store.New(pool)
	cur, err := st.CurrentBuild(ctx)
	require.NoError(t, err)
	require.Equal(t, res.Build.BuildID, cur.ID)
	diffs, err := compute.DiffExports(want, cur.Export, compute.DiffOptions{IgnorePaths: []string{"generated_at", "build_id", "run_id"}})
	require.NoError(t, err)
	for _, d := range diffs[:min(20, len(diffs))] {
		t.Log(d)
	}
	require.Empty(t, diffs, "differences against the 1.x export")

	run, err := st.GetRun(ctx, res.RunID)
	require.NoError(t, err)
	require.Equal(t, "succeeded", run.Status)
	require.Equal(t, "import", run.Mode)
	require.Len(t, run.Underlyings, 125)
	var points int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM summary_history WHERE observed_at < '2026-10-19'`).Scan(&points))
	require.Equal(t, 1, points, "history starts at the 1.x fetch date")

	// Second import: no-op.
	res2, err := im.Import(ctx, dataDir)
	require.NoError(t, err)
	require.True(t, res2.Skipped)
	require.Equal(t, res.RunID, res2.RunID)
	var runs, builds int
	require.NoError(t, pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM collection_runs), (SELECT count(*) FROM builds)`).Scan(&runs, &builds))
	require.Equal(t, 1, runs)
	require.Equal(t, 1, builds)
}

func newImporter(t *testing.T) (*importer.Importer, *pgxpool.Pool) {
	t.Helper()
	cfg := filepath.Join("..", "..", "config")
	settings, err := model.LoadSettings(filepath.Join(cfg, "settings.json"))
	require.NoError(t, err)
	settingsJSON, err := os.ReadFile(filepath.Join(cfg, "settings.json"))
	require.NoError(t, err)
	assets, err := model.LoadAssets(filepath.Join(cfg, "assets.json"))
	require.NoError(t, err)
	pool, _ := testdb.Migrated(t)
	return importer.New(pool, assets, settings, settingsJSON, "test", slog.New(slog.DiscardHandler)), pool
}

func requireV1Data(t *testing.T) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(dataDir, "snapshots")); err != nil {
		t.Skipf("1.x data not found at %s: %v", dataDir, err)
	}
}

func TestImportRefusesNonEmptyDatabaseUnlessForced(t *testing.T) {
	requireV1Data(t)
	im, pool := newImporter(t)
	ctx := context.Background()
	_, err := store.New(pool).CreateRun(ctx, store.RunParams{RunKey: "other", Mode: "refresh", Trigger: "cli", StartedAt: time.Now(), CollectorVersion: "t"})
	require.NoError(t, err)

	_, err = im.Import(ctx, dataDir)
	require.ErrorIs(t, err, importer.ErrNotEmpty)
	var snaps int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM snapshots`).Scan(&snaps))
	require.Zero(t, snaps)

	res, err := im.WithForce(true).Import(ctx, dataDir)
	require.NoError(t, err)
	require.Equal(t, 125, res.Snapshots)
}

func TestImportWaitsForNoCollectorRun(t *testing.T) {
	requireV1Data(t)
	im, pool := newImporter(t)
	ctx := context.Background()
	lock, err := db.TryLock(ctx, pool, db.CollectorLockName, nil)
	require.NoError(t, err)
	_, err = im.Import(ctx, dataDir)
	require.ErrorContains(t, err, "collector run is in progress")
	lock.Release(ctx)
	_, err = im.Import(ctx, dataDir)
	require.NoError(t, err)
}

// Without data/runs/*.json the run key comes from the newest snapshot's run_id, not from the clock.
func TestImportRunKeyFromSnapshotsWithoutRunFiles(t *testing.T) {
	requireV1Data(t)
	im, _ := newImporter(t)
	tmp := t.TempDir()
	entries, err := os.ReadDir(dataDir)
	require.NoError(t, err)
	for _, e := range entries {
		if e.Name() == "runs" {
			continue
		}
		abs, err := filepath.Abs(filepath.Join(dataDir, e.Name()))
		require.NoError(t, err)
		require.NoError(t, os.Symlink(abs, filepath.Join(tmp, e.Name())))
	}
	res, err := im.Import(context.Background(), tmp)
	require.NoError(t, err)
	require.Equal(t, "20261018T201453Z", res.RunKey)
}
