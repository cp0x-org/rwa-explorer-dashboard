package collect

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/config"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/model"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/runlog"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/store"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/testdb"
)

// journalRunner is newRunner with a logger that journals into run_events at the given level.
func journalRunner(t *testing.T, pool *pgxpool.Pool, f *fakes, assets []model.Asset, level slog.Level) *Runner {
	t.Helper()
	h := runlog.New(slog.NewTextHandler(io.Discard, nil), runlog.Options{Level: level})
	h.SetSink(store.New(pool))
	t.Cleanup(h.Close)
	settings := loadSettings(t)
	settings.RequestsPerMinute = model.RequestsPerMinute{CMC: 600000, Portals: 600000}
	settings.MaxRetries = 1
	settings.RequestTimeoutSeconds = 5
	cfg := config.Config{CMCAPIKey: "cmc-test-key", PortalsAPIKey: "portals-test-key"}
	return NewRunner(pool, cfg, settings, assets, slog.New(h)).WithBaseURLs(f.cmcURL, f.portalsURL)
}

func steps(events []store.RunEvent) map[string]int {
	m := map[string]int{}
	for _, e := range events {
		m[e.Step]++
	}
	return m
}

func TestRunJournalInfo(t *testing.T) {
	pool, _ := testdb.Migrated(t)
	f := startFakes(t, nil)
	r := journalRunner(t, pool, f, twoAssets(), slog.LevelInfo)
	ctx := context.Background()

	res, err := r.Run(ctx, Options{Mode: ModeRefresh, Trigger: "cli"})
	require.NoError(t, err)
	require.Equal(t, RunSucceeded, res.Status)

	// Run end flushes: no waiting for the timer.
	events, next, err := store.New(pool).ListRunEvents(ctx, res.RunID, 0, "debug", 2000)
	require.NoError(t, err)
	require.Nil(t, next)
	got := steps(events)
	for _, s := range []string{"collect.cmc", "collect.portals", "collect.underlying", "run"} {
		assert.Positive(t, got[s], "step %s: %v", s, got)
	}
	assert.Zero(t, countLevel(events, "debug"), "no per-request lines at info")
	assert.Zero(t, countLevel(events, "error"))

	first, last := events[0], events[len(events)-1]
	assert.Equal(t, "run started", first.Message)
	assert.Equal(t, "run finished", last.Message)
	assert.Equal(t, "run", last.Step)
	assert.Equal(t, "succeeded", attr(t, last, "status"))

	saved := map[string]bool{}
	for _, e := range events {
		if e.Message == "saved" {
			assert.Equal(t, "collect.underlying", e.Step)
			assert.NotNil(t, attr(t, e, "duration_ms"))
			saved[attr(t, e, "underlying").(string)] = true
		}
		assert.NotContains(t, string(e.Attrs), "test-key", "no secrets in the journal")
	}
	assert.Len(t, saved, 2, "one saved line per underlying")

	d, err := store.New(pool).GetRun(ctx, res.RunID)
	require.NoError(t, err)
	assert.Equal(t, len(events), d.EventsCount)
	assert.Zero(t, d.ErrorsCount)
}

func TestRunJournalDebugHasRequests(t *testing.T) {
	pool, _ := testdb.Migrated(t)
	f := startFakes(t, nil)
	r := journalRunner(t, pool, f, twoAssets(), slog.LevelDebug)
	ctx := context.Background()

	res, err := r.Run(ctx, Options{Mode: ModeRefresh, Trigger: "cli"})
	require.NoError(t, err)

	events, _, err := store.New(pool).ListRunEvents(ctx, res.RunID, 0, "debug", 2000)
	require.NoError(t, err)
	var cmcReq, portalsReq int
	for _, e := range events {
		if e.Message != "request" {
			continue
		}
		assert.Equal(t, "debug", e.Level)
		for _, k := range []string{"provider", "path", "status", "duration_ms"} {
			assert.NotNil(t, attr(t, e, k), k)
		}
		switch e.Step {
		case "collect.cmc":
			cmcReq++
		case "collect.portals":
			portalsReq++
		default:
			t.Errorf("request with step %q", e.Step)
		}
	}
	assert.Equal(t, res.CMCRequests, cmcReq)
	assert.Equal(t, res.PortalsRequests, portalsReq)

	infoOnly, _, err := store.New(pool).ListRunEvents(ctx, res.RunID, 0, "info", 2000)
	require.NoError(t, err)
	assert.Less(t, len(infoOnly), len(events))
}

func TestRunJournalFailedUnderlying(t *testing.T) {
	pool, _ := testdb.Migrated(t)
	f := startFakes(t, nil)
	f.rwaInfoStatus.Store(500) // a warning, the run still succeeds
	r := journalRunner(t, pool, f, twoAssets(), slog.LevelInfo)
	ctx := context.Background()
	res, err := r.Run(ctx, Options{Mode: ModeRefresh, Trigger: "cli"})
	require.NoError(t, err)
	require.NotEmpty(t, res.Warnings)

	warns, _, err := store.New(pool).ListRunEvents(ctx, res.RunID, 0, "warn", 2000)
	require.NoError(t, err)
	require.NotEmpty(t, warns)
	assert.Equal(t, "collect.cmc", warns[0].Step)
}

func attr(t *testing.T, e store.RunEvent, key string) any {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.Unmarshal(e.Attrs, &m))
	return m[key]
}

func countLevel(events []store.RunEvent, level string) int {
	n := 0
	for _, e := range events {
		if e.Level == level {
			n++
		}
	}
	return n
}
