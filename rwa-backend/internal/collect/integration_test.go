package collect

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/compute"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/config"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/db"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/model"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/store"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/testdb"
)

// body returns the recorded response body of testdata/raw/<provider>/<name>.json.
func body(t *testing.T, provider, name string) json.RawMessage {
	t.Helper()
	b, err := os.ReadFile("../../testdata/raw/" + provider + "/" + name + ".json")
	require.NoError(t, err)
	var rec struct {
		Body json.RawMessage `json:"body"`
	}
	require.NoError(t, json.Unmarshal(b, &rec))
	return rec.Body
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// fakes serve the recorded fixtures. portalsHook may take over a Portals request (return true when handled).
type fakes struct {
	cmcURL, portalsURL string
	cmcCalls           atomic.Int64
	portalsCalls       atomic.Int64
	rwaInfoStatus      atomic.Int64 // when non-zero, /v5/real-world-assets/info answers with this HTTP status
	emptyQuotes        atomic.Bool  // when set, CMC knows no tokenized assets (rwa_assets is empty)
}

func startFakes(t *testing.T, portalsHook func(w http.ResponseWriter, r *http.Request) bool) *fakes {
	t.Helper()
	f := &fakes{}
	quotes, cryptoInfo := body(t, "cmc", "quotes_latest"), body(t, "cmc", "crypto_info")
	issuer, rwaInfo := body(t, "cmc", "issuer_small"), body(t, "cmc", "rwa_info")
	networks, platforms, search := body(t, "portals", "networks"), body(t, "portals", "platforms"), body(t, "portals", "tokens_search_aapl")

	cmcSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.cmcCalls.Add(1)
		switch r.URL.Path {
		case "/v5/real-world-assets/quotes/latest":
			if f.emptyQuotes.Load() {
				writeJSON(w, 200, map[string]any{"data": map[string]any{"rwa_assets": []any{}}, "status": map[string]any{"error_code": "0"}})
				return
			}
			writeJSON(w, 200, quotes)
		case "/v2/cryptocurrency/info":
			var all struct {
				Data map[string]json.RawMessage `json:"data"`
			}
			_ = json.Unmarshal(cryptoInfo, &all)
			data := map[string]json.RawMessage{}
			for _, id := range strings.Split(r.URL.Query().Get("id"), ",") {
				if v, ok := all.Data[id]; ok {
					data[id] = v
				} else {
					data[id] = json.RawMessage(`{"id":` + id + `,"symbol":"T` + id + `","name":"Token ` + id + `","contract_address":[]}`)
				}
			}
			writeJSON(w, 200, map[string]any{"data": data, "status": map[string]any{"error_code": 0, "credit_count": 1}})
		case "/v5/real-world-assets/issuers":
			writeJSON(w, 200, issuer)
		case "/v5/real-world-assets/info":
			if st := f.rwaInfoStatus.Load(); st != 0 {
				writeJSON(w, int(st), map[string]string{"message": "boom"})
				return
			}
			writeJSON(w, 200, rwaInfo)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(cmcSrv.Close)
	portalsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.portalsCalls.Add(1)
		if portalsHook != nil && portalsHook(w, r) {
			return
		}
		switch r.URL.Path {
		case "/v1/networks":
			writeJSON(w, 200, networks)
		case "/v2/platforms":
			writeJSON(w, 200, platforms)
		case "/v2/tokens":
			if r.URL.Query().Get("ids") != "" {
				writeJSON(w, 200, map[string]any{"tokens": []any{}, "totalItems": 0, "more": false, "page": 0, "pageItems": 0})
				return
			}
			writeJSON(w, 200, search)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(portalsSrv.Close)
	f.cmcURL, f.portalsURL = cmcSrv.URL, portalsSrv.URL
	return f
}

func newRunner(t *testing.T, pool *pgxpool.Pool, f *fakes, assets []model.Asset) *Runner {
	t.Helper()
	settings := loadSettings(t)
	settings.RequestsPerMinute = model.RequestsPerMinute{CMC: 600000, Portals: 600000}
	settings.MaxRetries = 1
	settings.RequestTimeoutSeconds = 5
	cfg := config.Config{CMCAPIKey: "cmc-test-key", PortalsAPIKey: "portals-test-key"}
	return NewRunner(pool, cfg, settings, assets, nil).WithBaseURLs(f.cmcURL, f.portalsURL)
}

func twoAssets() []model.Asset { return testAssets()[:2] } // NVDA, AAPL

func countRows(t *testing.T, pool *pgxpool.Pool, query string, args ...any) int {
	t.Helper()
	var n int
	require.NoError(t, pool.QueryRow(context.Background(), query, args...).Scan(&n))
	return n
}

func TestRunStoresSnapshots(t *testing.T) {
	pool, _ := testdb.Migrated(t)
	f := startFakes(t, nil)
	r := newRunner(t, pool, f, twoAssets())
	ctx := context.Background()

	res, err := r.Run(ctx, Options{Mode: ModeRefresh, Trigger: "cli"})
	require.NoError(t, err)
	require.NoError(t, res.Err)
	assert.Equal(t, RunSucceeded, res.Status)
	assert.Equal(t, 2, res.Completed)
	assert.Zero(t, res.Failed)
	assert.NotZero(t, res.CMCRequests)
	assert.NotZero(t, res.PortalsRequests)

	st := store.New(pool)
	run, err := st.GetRun(ctx, res.RunID)
	require.NoError(t, err)
	assert.Equal(t, res.RunKey, run.RunKey)
	assert.Equal(t, "succeeded", run.Status)
	assert.Equal(t, "refresh", run.Mode)
	assert.Equal(t, "cli", run.Trigger)
	assert.NotNil(t, run.FinishedAt)
	assert.Nil(t, run.Error)
	assert.Equal(t, 2, run.Counters.Selected)
	assert.Equal(t, 2, run.Counters.Completed)
	assert.Equal(t, int(f.cmcCalls.Load()), run.Counters.CMCRequests)
	assert.Equal(t, int(f.portalsCalls.Load()), run.Counters.PortalsRequests)
	assert.NotZero(t, run.Counters.CMCCredits)
	assert.JSONEq(t, "[]", string(run.Warnings))
	require.Len(t, run.Underlyings, 2)
	for _, u := range run.Underlyings {
		assert.Equal(t, "ok", u.Status, u.UnderlyingID)
		assert.Nil(t, u.Error)
		assert.NotNil(t, u.StartedAt)
		assert.NotNil(t, u.FinishedAt)
		assert.NotZero(t, u.PortalsRequests)
	}

	rows, err := st.LatestSnapshotsForEnabled(ctx)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	var snaps []*model.Snapshot
	for _, row := range rows {
		assert.Equal(t, res.RunID, row.RunID)
		snap, err := model.ParseSnapshot(row.Payload)
		require.NoError(t, err)
		assert.Equal(t, "ok", snap.Status)
		assert.Equal(t, res.RunKey, snap.RunID)
		assert.Equal(t, row.UnderlyingID, snap.UnderlyingID)
		require.NotNil(t, snap.Portals)
		assert.NotEmpty(t, snap.Portals.Records)
		snaps = append(snaps, snap)
	}
	built, err := compute.Build(snaps, nil, r.settings, nil)
	require.NoError(t, err)
	assert.NotEmpty(t, built.Export.Tokens)

	// Dimensions learned along the way.
	assert.Equal(t, 2, countRows(t, pool, `SELECT count(*) FROM underlyings WHERE enabled AND cmc_rwa_id IS NOT NULL AND status = 'ok' AND last_run_id = $1`, res.RunID))
	assert.Positive(t, countRows(t, pool, `SELECT count(*) FROM issuers WHERE metadata_status = 'cmc'`))
	assert.Positive(t, countRows(t, pool, `SELECT count(*) FROM platforms`))
	assert.Positive(t, countRows(t, pool, `SELECT count(*) FROM networks WHERE active AND last_seen_run_id = $1`, res.RunID))
	assert.Equal(t, 1, countRows(t, pool, `SELECT count(*) FROM underlyings WHERE ticker = 'NVDA' AND website IS NOT NULL AND industry IS NOT NULL`))

	// The advisory lock was released: a second run works (a later clock gives it a new run key).
	r.WithClock(func() time.Time { return time.Now().Add(time.Hour) })
	res2, err := r.Run(ctx, Options{Mode: ModeRefresh, Trigger: "cli", Symbols: []string{"nvda"}})
	require.NoError(t, err)
	assert.Equal(t, 1, res2.Completed)

	// add: both underlyings have snapshots now -> nothing to do, no run row.
	runs := countRows(t, pool, `SELECT count(*) FROM collection_runs`)
	res3, err := r.Run(ctx, Options{Mode: ModeAdd, Trigger: "cli"})
	require.NoError(t, err)
	assert.True(t, res3.NothingToDo)
	assert.Equal(t, runs, countRows(t, pool, `SELECT count(*) FROM collection_runs`))

	_, err = r.Run(ctx, Options{Mode: ModeRefresh, Symbols: []string{"NOPE"}})
	require.ErrorContains(t, err, "unknown or disabled symbols: NOPE")
}

func TestRunInProgress(t *testing.T) {
	pool, _ := testdb.Migrated(t)
	f := startFakes(t, nil)
	r := newRunner(t, pool, f, twoAssets())
	ctx := context.Background()

	conn, err := pool.Acquire(ctx)
	require.NoError(t, err)
	defer conn.Release()
	var locked bool
	require.NoError(t, conn.QueryRow(ctx, lockQuery).Scan(&locked))
	require.True(t, locked)
	defer func() { _, _ = conn.Exec(ctx, `SELECT pg_advisory_unlock(hashtext('rwa-collector-run'))`) }()

	_, err = r.Run(ctx, Options{Mode: ModeRefresh})
	require.ErrorIs(t, err, ErrRunInProgress)
	assert.Zero(t, countRows(t, pool, `SELECT count(*) FROM collection_runs`))
}

func TestRunAddOnlyMissing(t *testing.T) {
	pool, _ := testdb.Migrated(t)
	f := startFakes(t, nil)
	r := newRunner(t, pool, f, twoAssets())
	ctx := context.Background()

	res, err := r.Run(ctx, Options{Mode: ModeRefresh, Symbols: []string{"NVDA"}})
	require.NoError(t, err)
	require.Equal(t, 1, res.Completed)
	r.WithClock(func() time.Time { return time.Now().Add(time.Hour) }) // another run key

	res, err = r.Run(ctx, Options{Mode: ModeAdd})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Completed, "only AAPL is new")
	assert.Equal(t, 2, res.Selected, "selected_count is the number of selected tickers, not of fetched ones")
	assert.Equal(t, 2, countRows(t, pool, `SELECT selected_count FROM collection_runs WHERE id = $1`, res.RunID))
	assert.Equal(t, 1, countRows(t, pool, `SELECT count(*) FROM snapshots WHERE run_id = $1`, res.RunID))
	assert.Equal(t, 2, countRows(t, pool, `SELECT count(*) FROM snapshots`))
}

func TestRunAuthFailureFailsRun(t *testing.T) {
	pool, _ := testdb.Migrated(t)
	f := startFakes(t, func(w http.ResponseWriter, _ *http.Request) bool {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"message": "invalid api key"})
		return true
	})
	r := newRunner(t, pool, f, twoAssets())
	ctx := context.Background()

	res, err := r.Run(ctx, Options{Mode: ModeRefresh})
	require.NoError(t, err)
	assert.Equal(t, RunFailed, res.Status)
	require.Error(t, res.Err)
	assert.ErrorContains(t, res.Err, "401")
	assert.Zero(t, res.Completed)
	assert.Zero(t, countRows(t, pool, `SELECT count(*) FROM snapshots`))

	run, err := store.New(pool).GetRun(ctx, res.RunID)
	require.NoError(t, err)
	assert.Equal(t, "failed", run.Status)
	require.NotNil(t, run.Error)
	assert.Contains(t, *run.Error, "401")
	assert.NotNil(t, run.FinishedAt)
	assert.NotContains(t, *run.Error, "portals-test-key")
}

func TestRunOneUnderlyingFailsRunPartial(t *testing.T) {
	pool, _ := testdb.Migrated(t)
	f := startFakes(t, func(w http.ResponseWriter, r *http.Request) bool {
		if r.URL.Path == "/v2/tokens" && strings.Contains(r.URL.Query().Get("search"), "AAPL") {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "boom"})
			return true
		}
		return false
	})
	r := newRunner(t, pool, f, twoAssets())
	ctx := context.Background()

	res, err := r.Run(ctx, Options{Mode: ModeRefresh})
	require.NoError(t, err)
	assert.Equal(t, RunPartial, res.Status)
	assert.NoError(t, res.Err)
	assert.Equal(t, 1, res.Completed)
	assert.Equal(t, 1, res.Failed)

	run, err := store.New(pool).GetRun(ctx, res.RunID)
	require.NoError(t, err)
	assert.Equal(t, "partial", run.Status)
	assert.Nil(t, run.Error)
	byID := map[string]store.RunUnderlying{}
	for _, u := range run.Underlyings {
		byID[u.UnderlyingID] = u
	}
	require.Len(t, byID, 2)
	assert.Equal(t, "ok", byID["US67066G1040"].Status)
	aapl := byID["US0378331005"]
	assert.Equal(t, "failed", aapl.Status)
	require.NotNil(t, aapl.Error)
	assert.Contains(t, *aapl.Error, "500")
	assert.Equal(t, 1, countRows(t, pool, `SELECT count(*) FROM snapshots`))
	assert.Zero(t, countRows(t, pool, `SELECT count(*) FROM snapshots WHERE underlying_id = 'US0378331005'`))
	assert.Equal(t, 1, countRows(t, pool, `SELECT count(*) FROM underlyings WHERE id = 'US0378331005' AND status IS NULL`), "failed underlying keeps its status")
}

func TestRunInterrupted(t *testing.T) {
	pool, _ := testdb.Migrated(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var searches atomic.Int64
	f := startFakes(t, func(_ http.ResponseWriter, r *http.Request) bool {
		if r.URL.Path == "/v2/tokens" && r.URL.Query().Get("search") != "" && searches.Add(1) == 1 {
			cancel() // SIGTERM arrives while the first underlying is being collected
		}
		return false
	})
	r := newRunner(t, pool, f, twoAssets())

	res, err := r.Run(ctx, Options{Mode: ModeRefresh})
	require.ErrorIs(t, err, context.Canceled)
	require.NotNil(t, res)
	assert.Equal(t, RunFailed, res.Status)

	var status, errText string
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT status, error FROM collection_runs WHERE id = $1`, res.RunID).Scan(&status, &errText))
	assert.Equal(t, "failed", status)
	assert.Equal(t, "interrupted", errText)
	assert.Zero(t, countRows(t, pool, `SELECT count(*) FROM snapshots`))
}

func runUnderlyingStatuses(t *testing.T, pool *pgxpool.Pool, runID int64) map[string]string {
	t.Helper()
	rows, err := pool.Query(context.Background(), `SELECT underlying_id, status FROM run_underlyings WHERE run_id = $1`, runID)
	require.NoError(t, err)
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var id, st string
		require.NoError(t, rows.Scan(&id, &st))
		out[id] = st
	}
	require.NoError(t, rows.Err())
	return out
}

// An interruption after the first underlying completed: its snapshot stays, the rest are listed as skipped.
func TestRunInterruptedAfterSomeCompleted(t *testing.T) {
	pool, _ := testdb.Migrated(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f := startFakes(t, func(_ http.ResponseWriter, r *http.Request) bool {
		if r.URL.Path == "/v2/tokens" && strings.Contains(r.URL.Query().Get("search"), "AAPL") {
			cancel() // SIGTERM arrives while the second underlying is being collected
		}
		return false
	})
	assets := []model.Asset{testAssets()[0], testAssets()[1], testAssets()[3]} // NVDA, AAPL, ARKK
	r := newRunner(t, pool, f, assets)

	res, err := r.Run(ctx, Options{Mode: ModeRefresh})
	require.ErrorIs(t, err, context.Canceled)
	require.NotNil(t, res)
	assert.Equal(t, RunFailed, res.Status, "an interruption is failed even after some underlyings completed")
	assert.Equal(t, 1, res.Completed)

	var status, errText string
	var selected, completed int
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT status, error, selected_count, completed_count FROM collection_runs WHERE id = $1`, res.RunID).
		Scan(&status, &errText, &selected, &completed))
	assert.Equal(t, "failed", status)
	assert.Equal(t, "interrupted", errText)
	assert.Equal(t, 3, selected)
	assert.Equal(t, 1, completed)
	assert.Equal(t, map[string]string{"US67066G1040": "ok", "US0378331005": "skipped", "etf:ticker:ARKK": "skipped"}, runUnderlyingStatuses(t, pool, res.RunID))
	assert.Equal(t, 1, countRows(t, pool, `SELECT count(*) FROM snapshots`))
}

// A deadline ends the run as failed with error "timeout", not "interrupted".
func TestRunTimeout(t *testing.T) {
	pool, _ := testdb.Migrated(t)
	f := startFakes(t, func(w http.ResponseWriter, r *http.Request) bool {
		if r.URL.Path == "/v2/tokens" {
			select { // a slow Portals
			case <-r.Context().Done():
			case <-time.After(5 * time.Second):
			}
			return true
		}
		return false
	})
	r := newRunner(t, pool, f, twoAssets())
	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()

	res, err := r.Run(ctx, Options{Mode: ModeRefresh})
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.NotNil(t, res)
	assert.Equal(t, RunFailed, res.Status)
	var status, errText string
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT status, error FROM collection_runs WHERE id = $1`, res.RunID).Scan(&status, &errText))
	assert.Equal(t, "failed", status)
	assert.Equal(t, "timeout", errText)
	assert.Equal(t, map[string]string{"US67066G1040": "skipped", "US0378331005": "skipped"}, runUnderlyingStatuses(t, pool, res.RunID))
}

// A failed CMC metadata request must not wipe website, logo and industry learned earlier.
func TestRunRWAInfoFailureKeepsStoredInfo(t *testing.T) {
	pool, _ := testdb.Migrated(t)
	f := startFakes(t, nil)
	base := time.Now()
	r := newRunner(t, pool, f, twoAssets()).WithClock(func() time.Time { return base })
	ctx := context.Background()

	_, err := r.Run(ctx, Options{Mode: ModeRefresh, Symbols: []string{"NVDA"}})
	require.NoError(t, err)
	var website, logo, industry *string
	read := func() {
		require.NoError(t, pool.QueryRow(ctx, `SELECT website, cmc_logo_url, industry FROM underlyings WHERE ticker = 'NVDA'`).Scan(&website, &logo, &industry))
	}
	read()
	require.NotNil(t, website)
	require.NotNil(t, industry)
	wantSite, wantLogo, wantIndustry := *website, logo, *industry

	f.rwaInfoStatus.Store(http.StatusInternalServerError)
	r = r.WithClock(func() time.Time { return base.Add(2 * time.Hour) })
	res, err := r.Run(ctx, Options{Mode: ModeRefresh, Symbols: []string{"NVDA"}})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Completed)
	assert.NotEmpty(t, res.Warnings)
	assert.Contains(t, strings.Join(res.Warnings, "\n"), "rwa info")
	read()
	require.NotNil(t, website)
	assert.Equal(t, wantSite, *website, "website kept")
	assert.Equal(t, wantLogo, logo, "logo kept")
	require.NotNil(t, industry)
	assert.Equal(t, wantIndustry, *industry, "industry kept")
	assert.Equal(t, 1, countRows(t, pool, `SELECT count(*) FROM underlyings WHERE ticker = 'NVDA' AND last_run_id = $1`, res.RunID), "other facts are still updated")
}

// A panic inside the run finishes the run as failed instead of leaving it running.
func TestRunPanicFinishesRun(t *testing.T) {
	pool, _ := testdb.Migrated(t)
	f := startFakes(t, nil)
	var calls atomic.Int64
	r := newRunner(t, pool, f, twoAssets()).WithClock(func() time.Time {
		if calls.Add(1) > 2 { // 1: run start; then the first snapshot
			panic("clock exploded")
		}
		return time.Now()
	})
	res, err := r.Run(context.Background(), Options{Mode: ModeRefresh})
	require.ErrorContains(t, err, "panic: clock exploded")
	require.NotNil(t, res)
	assert.NotZero(t, res.RunID)
	var status, errText string
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT status, error FROM collection_runs WHERE id = $1`, res.RunID).Scan(&status, &errText))
	assert.Equal(t, "failed", status)
	assert.Equal(t, "panic: clock exploded", errText)
	// The advisory lock was released too.
	_, err = newRunner(t, pool, f, twoAssets()).Run(context.Background(), Options{Mode: ModeRefresh, Symbols: []string{"nope"}})
	require.ErrorIs(t, err, ErrInvalidOptions)
}

func TestAbandonRunningRuns(t *testing.T) {
	pool, _ := testdb.Migrated(t)
	ctx := context.Background()
	st := store.New(pool)
	mk := func(key string) int64 {
		id, err := st.CreateRun(ctx, store.RunParams{RunKey: key, Mode: "refresh", Trigger: "cli", StartedAt: time.Now(), CollectorVersion: "t"})
		require.NoError(t, err)
		return id
	}
	stale := mk("stale")
	done := mk("done")
	require.NoError(t, st.FinishRun(ctx, done, "succeeded", store.RunCounters{}, nil, ""))

	// While another session holds the collector lock a run may really be in progress: nothing is touched.
	lock, err := db.TryLock(ctx, pool, db.CollectorLockName, nil)
	require.NoError(t, err)
	n, err := AbandonRunningRuns(ctx, pool, nil)
	require.NoError(t, err)
	assert.Zero(t, n)
	lock.Release(ctx)
	run, err := st.GetRun(ctx, stale)
	require.NoError(t, err)
	assert.Equal(t, "running", run.Status)

	n, err = AbandonRunningRuns(ctx, pool, nil)
	require.NoError(t, err)
	assert.EqualValues(t, 1, n)
	run, err = st.GetRun(ctx, stale)
	require.NoError(t, err)
	assert.Equal(t, "failed", run.Status)
	require.NotNil(t, run.Error)
	assert.Equal(t, "abandoned", *run.Error)
	assert.NotNil(t, run.FinishedAt)
	run, err = st.GetRun(ctx, done)
	require.NoError(t, err)
	assert.Equal(t, "succeeded", run.Status)
}

func TestFormatIDs(t *testing.T) {
	assert.Equal(t, "[39002, 39839]", formatIDs([]int{39002, 39839}))
	assert.Equal(t, "[]", formatIDs(nil))
}
