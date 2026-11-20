package store_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/db"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/model"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/store"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/testdb"
)

const (
	nvdaID  = "US67066G1040"
	aaplID  = "US0378331005"
	tokenID = 36992
)

// qh is QueryHistory returning just the points.
func qh(st *store.Store, ctx context.Context, entity, id string, metrics []string, from, to *time.Time, step string, limit int) ([]store.HistoryPoint, error) {
	r, err := st.QueryHistory(ctx, entity, id, metrics, from, to, step, limit)
	if err != nil {
		return nil, err
	}
	return r.Points, nil
}

func ptr[T any](v T) *T { return &v }

func dec(s string) model.Decimal { return model.MustDecimal(s) }

func day(d, h int) time.Time { return time.Date(2026, 10, d, h, 0, 0, 0, time.UTC) }

func assets() []model.Asset {
	return []model.Asset{
		{Ticker: "NVDA", Name: "NVIDIA", AssetType: "stock", ISIN: ptr(nvdaID), Enabled: true},
		{Ticker: "AAPL", Name: "Apple", AssetType: "stock", ISIN: ptr(aaplID), Enabled: true},
	}
}

type env struct {
	ctx  context.Context
	pool *pgxpool.Pool
	st   *store.Store
}

func newEnv(t *testing.T) *env {
	t.Helper()
	pool, _ := testdb.Migrated(t)
	e := &env{ctx: context.Background(), pool: pool, st: store.New(pool)}
	_, err := e.st.SyncUnderlyings(e.ctx, assets())
	require.NoError(t, err)
	return e
}

func (e *env) run(t *testing.T, key string, started time.Time) int64 {
	t.Helper()
	id, err := e.st.CreateRun(e.ctx, store.RunParams{RunKey: key, Mode: "refresh", Trigger: "cli", StartedAt: started,
		CollectorVersion: "test", Settings: []byte(`{"schema_version":1}`)})
	require.NoError(t, err)
	return id
}

func snapshotJSON(id string) []byte {
	return []byte(fmt.Sprintf(`{"schema_version":1,"underlying_id":%q,"status":"ok","cmc":{"rwa":{"tokens":[]}},"portals":{"records":{}}}`, id))
}

func TestFullCycle(t *testing.T) {
	e := newEnv(t)
	ctx, st := e.ctx, e.st

	runID := e.run(t, "20261018T201453Z", day(18, 20))
	require.NoError(t, st.UpsertRunUnderlying(ctx, store.RunUnderlying{RunID: runID, UnderlyingID: nvdaID, Status: "ok", StartedAt: ptr(day(18, 20)), PortalsRequests: 2}))
	require.NoError(t, st.UpsertRunUnderlying(ctx, store.RunUnderlying{RunID: runID, UnderlyingID: aaplID, Status: "failed", Error: ptr("portals 500")}))

	// snapshots
	for _, id := range []string{nvdaID, aaplID} {
		_, err := st.InsertSnapshot(ctx, runID, id, day(18, 20), "ok", snapshotJSON(id))
		require.NoError(t, err)
	}
	latest, err := st.LatestSnapshotsForEnabled(ctx)
	require.NoError(t, err)
	require.Len(t, latest, 2)
	assert.JSONEq(t, string(snapshotJSON(aaplID)), string(latest[0].Payload))
	var bytes int
	require.NoError(t, e.pool.QueryRow(ctx, `SELECT payload_bytes FROM snapshots WHERE underlying_id = $1`, nvdaID).Scan(&bytes))
	assert.Equal(t, len(snapshotJSON(nvdaID)), bytes)

	// dimensions
	require.NoError(t, st.UpsertUnderlyingFacts(ctx, []store.UnderlyingFactsRow{{ID: nvdaID, CMCRwaID: ptr(2), CMCName: ptr("Nvidia Corp"),
		Website: ptr("https://www.nvidia.com"), Status: ptr("ok"), LastRunID: runID}}))
	require.NoError(t, st.UpsertIssuers(ctx, []store.IssuerRow{{ID: "6878977dcbbf471de3366e85", Name: ptr("Backed Assets"), RosterSize: ptr(40),
		Roster: []byte(`[{"crypto_id":36992,"symbol":"NVDAX","rwa_id":2}]`), MetadataStatus: "cmc", FetchedAt: ptr(day(18, 20))}}))
	require.NoError(t, st.UpsertTokens(ctx, runID, []store.TokenRow{{ID: tokenID, UnderlyingID: nvdaID, IssuerID: ptr("6878977dcbbf471de3366e85"),
		Symbol: ptr("NVDAX"), Name: ptr("NVIDIA tokenized stock (xStock)"), CMCSlug: ptr("nvidia-tokenized-stock-xstock"), Kind: "tokenized",
		DateAdded: ptr(time.Date(2025, 7, 1, 7, 20, 20, 0, time.UTC)), Coverage: "search"}}))
	require.NoError(t, st.UpsertDeployments(ctx, runID, []store.DeploymentRow{{TokenID: tokenID, CMCNetwork: "Ethereum",
		Address: "0xc845b2894dbddd03858fd2d643b4ef725fe0849d", AddressRaw: "0xC845b2894dBDDd03858fd2D643B4eF725fE0849d",
		Network: ptr("ethereum"), PositionKey: ptr("ethereum:0xc845b2894dbddd03858fd2d643b4ef725fe0849d"), Status: "indexed",
		PortalsSymbol: ptr("NVDAX"), Decimals: ptr(18), PortalsPlatform: ptr("xstocks")}}))
	require.NoError(t, st.UpsertPlatforms(ctx, []store.PlatformRow{{Slug: "xstocks", Name: "xStocks"}}))
	require.NoError(t, st.UpsertNetworks(ctx, runID, []store.NetworkRow{{Slug: "ethereum", CMCNames: []string{"Ethereum"}}}))
	posKey := "ethereum:0x01c3f0e865293792a76deaf6fe744ed6a58b3e9885d2a760baf8d4fd22fcd477"
	require.NoError(t, st.UpsertPositions(ctx, runID, []store.PositionRow{{Key: posKey, Network: "ethereum", Platform: "uniswap-v4",
		Symbol: ptr("NVDAX/USDC"), Address: posKey[9:], PositionKind: "pool_id", Category: "dex_lp", Tags: []string{"#clp"},
		Components:         []byte(`[{"key":"ethereum:0xc845b2894dbddd03858fd2d643b4ef725fe0849d","symbol":"NVDAX"}]`),
		LiquiditySemantics: "pool_tvl", IsPortable: ptr(false), LiquidityUSD: dec("1151.2302119347073"), APYPercent: dec("2.259"),
		VolumeUSD1d: dec("0.0000001"), SourceUpdatedAt: ptr(day(18, 19)), Flags: []string{"range_details_unknown"}, Eligibility: "low_liquidity"}}))
	require.NoError(t, st.UpsertTokenPositions(ctx, runID, []store.TokenPositionRow{{TokenID: tokenID, PositionKey: posKey, Link: "direct",
		Depth: ptr(1), Path: []string{"ethereum:0xc845b2894dbddd03858fd2d643b4ef725fe0849d", posKey}}}))

	// NUMERIC values keep their exact digits; nil decimals are NULL, not zero.
	var liq, vol string
	var base *string
	require.NoError(t, e.pool.QueryRow(ctx, `SELECT liquidity_usd::text, volume_usd_1d::text, base_apy_percent::text FROM positions WHERE key = $1`, posKey).Scan(&liq, &vol, &base))
	assert.Equal(t, "1151.2302119347073", liq)
	assert.Equal(t, "0.0000001", vol)
	assert.Nil(t, base)

	// history
	counts := store.HistoryCounts{RawConfirmedOpportunitiesCount: 12, EligibleOpportunitiesCount: 3}
	require.NoError(t, st.InsertSummaryHistory(ctx, []store.SummaryHistoryRow{{RunID: runID, ObservedAt: day(18, 20), UnderlyingsCount: 125,
		TokensCount: 563, HistoryCounts: counts, MedianReportedAPYPercent: dec("10.503"), TokenizedMarketCapUSD: dec("148973893.9")}}))
	require.NoError(t, st.InsertUnderlyingHistory(ctx, []store.UnderlyingHistoryRow{{RunID: runID, UnderlyingID: nvdaID, ObservedAt: day(18, 20),
		Status: "ok", TokenizedMarketCapUSD: dec("148973893.90937608"), VersionsCount: 8, HistoryCounts: counts}}))
	require.NoError(t, st.InsertTokenHistory(ctx, []store.TokenHistoryRow{{RunID: runID, TokenID: tokenID, ObservedAt: day(18, 20),
		PriceUSD: dec("211.7537324392806"), UsefulnessScore: 0, RankInUnderlying: ptr(1), HistoryCounts: counts,
		Breakdowns: []byte(`{"by_network":{"ethereum":{"raw":11,"eligible":0}},"by_category":{},"by_platform":{}}`)}}))
	require.NoError(t, st.InsertIssuerHistory(ctx, []store.IssuerHistoryRow{{RunID: runID, IssuerID: "6878977dcbbf471de3366e85", ObservedAt: day(18, 20),
		IssuerUsefulnessScore: 7, BasketRelativeScore: dec("1.25"), RosterSize: ptr(40), HistoryCounts: counts, Breakdowns: []byte(`{}`)}}))
	require.NoError(t, st.InsertPositionHistory(ctx, []store.PositionHistoryRow{{RunID: runID, PositionKey: posKey, ObservedAt: day(18, 20),
		LiquidityUSD: dec("1151.23"), APYPercent: dec("2.259"), Eligibility: "low_liquidity", Flags: nil}}))
	require.NoError(t, st.InsertSummaryHistory(ctx, nil), "empty insert is a no-op")

	// build, in one transaction
	var build1 int64
	require.NoError(t, db.WithTx(ctx, e.pool, func(tx pgx.Tx) error {
		txs := store.New(tx)
		id, err := txs.InsertBuild(ctx, tx, store.BuildRow{RunID: runID, GeneratedAt: day(18, 21), SchemaVersion: 1, ScoreVersion: "eligible-count-v1",
			Settings: []byte(`{"max_graph_depth":3}`), Summary: []byte(`{"tokens_count":563}`), Export: []byte(`{"schema_version":1,"tokens":[]}`), ExportSHA256: "abc"})
		if err != nil {
			return err
		}
		build1 = id
		if err := txs.InsertBuildChanges(ctx, tx, store.BuildChangesRow{BuildID: id, NewTokens: []int32{tokenID}, EligibleDelta: 3}); err != nil {
			return err
		}
		return txs.SetCurrentBuild(ctx, tx, id)
	}))
	cur, err := st.CurrentBuild(ctx)
	require.NoError(t, err)
	assert.Equal(t, build1, cur.ID)
	assert.True(t, cur.IsCurrent)
	assert.JSONEq(t, `{"schema_version":1,"tokens":[]}`, string(cur.Export))
	meta, err := st.CurrentBuildMeta(ctx)
	require.NoError(t, err)
	assert.Equal(t, "abc", meta.ExportSHA256)
	assert.JSONEq(t, `{"tokens_count":563}`, string(meta.Summary))

	// finish the run
	require.NoError(t, st.FinishRun(ctx, runID, "partial", store.RunCounters{Selected: 2, Completed: 1, Failed: 1, CMCRequests: 3, PortalsRequests: 2, CMCCredits: 5}, []string{"AAPL failed"}, ""))
	d, err := st.GetRun(ctx, runID)
	require.NoError(t, err)
	assert.Equal(t, "partial", d.Status)
	assert.Equal(t, 1, d.Counters.Failed)
	assert.NotNil(t, d.FinishedAt)
	assert.Nil(t, d.Error)
	assert.JSONEq(t, `["AAPL failed"]`, string(d.Warnings))
	require.Len(t, d.Underlyings, 2)
	assert.Equal(t, aaplID, d.Underlyings[0].UnderlyingID)
	assert.Equal(t, "portals 500", *d.Underlyings[0].Error)
	last, err := st.LastRun(ctx)
	require.NoError(t, err)
	assert.Equal(t, runID, last.ID)
	_, err = st.GetRun(ctx, 999999)
	assert.ErrorIs(t, err, store.ErrNotFound)
	assert.ErrorIs(t, st.FinishRun(ctx, 999999, "failed", store.RunCounters{}, nil, "x"), store.ErrNotFound)
}

func TestSetCurrentBuild(t *testing.T) {
	e := newEnv(t)
	ctx, st := e.ctx, e.st
	_, err := st.CurrentBuild(ctx)
	assert.ErrorIs(t, err, store.ErrNotFound)
	_, err = st.CurrentBuildMeta(ctx)
	assert.ErrorIs(t, err, store.ErrNotFound)
	_, err = st.BuildByID(ctx, 1)
	assert.ErrorIs(t, err, store.ErrNotFound)

	runID := e.run(t, "r1", day(18, 20))
	mk := func(sha string) (id int64) {
		require.NoError(t, db.WithTx(ctx, e.pool, func(tx pgx.Tx) error {
			var err error
			id, err = st.InsertBuild(ctx, tx, store.BuildRow{RunID: runID, GeneratedAt: day(18, 21), SchemaVersion: 1, ScoreVersion: "v1", Export: []byte(`{}`), ExportSHA256: sha})
			if err != nil {
				return err
			}
			return st.SetCurrentBuild(ctx, tx, id)
		}))
		return id
	}
	b1 := mk("one")
	b2 := mk("two")

	cur, err := st.CurrentBuild(ctx)
	require.NoError(t, err)
	assert.Equal(t, b2, cur.ID)
	first, err := st.BuildByID(ctx, b1)
	require.NoError(t, err)
	assert.False(t, first.IsCurrent)
	var n int
	require.NoError(t, e.pool.QueryRow(ctx, `SELECT count(*) FROM builds WHERE is_current`).Scan(&n))
	assert.Equal(t, 1, n)

	// the partial unique index forbids a second current build
	_, err = e.pool.Exec(ctx, `UPDATE builds SET is_current = true WHERE id = $1`, b1)
	var pgErr *pgconn.PgError
	require.ErrorAs(t, err, &pgErr)
	assert.Equal(t, "23505", pgErr.Code)
	assert.Equal(t, "builds_current_idx", pgErr.ConstraintName)

	// a failed transaction leaves the current build untouched
	boom := fmt.Errorf("boom")
	err = db.WithTx(ctx, e.pool, func(tx pgx.Tx) error {
		id, err := st.InsertBuild(ctx, tx, store.BuildRow{RunID: runID, GeneratedAt: day(18, 22), SchemaVersion: 1, ScoreVersion: "v1", Export: []byte(`{}`), ExportSHA256: "three"})
		require.NoError(t, err)
		require.NoError(t, st.SetCurrentBuild(ctx, tx, id))
		return boom
	})
	require.ErrorIs(t, err, boom)
	cur, err = st.CurrentBuild(ctx)
	require.NoError(t, err)
	assert.Equal(t, b2, cur.ID)

	// unknown id: ErrNotFound and nothing is cleared
	err = db.WithTx(ctx, e.pool, func(tx pgx.Tx) error { return st.SetCurrentBuild(ctx, tx, 424242) })
	assert.ErrorIs(t, err, store.ErrNotFound)
	cur, err = st.CurrentBuild(ctx)
	require.NoError(t, err)
	assert.Equal(t, b2, cur.ID)
}

func TestRetentionKeepsLatestSnapshot(t *testing.T) {
	e := newEnv(t)
	ctx, st := e.ctx, e.st
	r1 := e.run(t, "r1", day(1, 6))
	r2 := e.run(t, "r2", day(10, 6))
	r3 := e.run(t, "r3", day(20, 6))
	ins := func(run int64, id string, at time.Time, status string) {
		_, err := st.InsertSnapshot(ctx, run, id, at, status, snapshotJSON(id))
		require.NoError(t, err)
	}
	// NVDA: two old snapshots and a fresh one. AAPL: only old ones, the newest is not ok.
	ins(r1, nvdaID, day(1, 7), "ok")
	ins(r2, nvdaID, day(10, 7), "ok")
	ins(r3, nvdaID, day(20, 7), "ok")
	ins(r1, aaplID, day(1, 7), "ok")
	ins(r2, aaplID, day(10, 7), "not_in_cmc")

	count := func() (n int) {
		require.NoError(t, e.pool.QueryRow(ctx, `SELECT count(*) FROM snapshots`).Scan(&n))
		return n
	}
	deleted, err := st.DeleteSnapshotsOlderThan(ctx, day(25, 0))
	require.NoError(t, err)
	// removed: NVDA r1, NVDA r2. Kept: NVDA r3 (latest), AAPL r2 (latest, not ok), AAPL r1 (latest successful).
	assert.EqualValues(t, 2, deleted)
	assert.Equal(t, 3, count())

	latest, err := st.LatestSnapshotsForEnabled(ctx)
	require.NoError(t, err)
	got := map[string]time.Time{}
	for _, s := range latest {
		got[s.UnderlyingID] = s.FetchedAt
	}
	assert.True(t, day(20, 7).Equal(got[nvdaID]))
	assert.True(t, day(10, 7).Equal(got[aaplID]), "latest snapshot per underlying regardless of status")

	deleted, err = st.DeleteSnapshotsOlderThan(ctx, day(30, 0))
	require.NoError(t, err)
	assert.EqualValues(t, 0, deleted, "nothing else is deletable")
}

func TestLatestSnapshotsSkipsDisabled(t *testing.T) {
	e := newEnv(t)
	r := e.run(t, "r1", day(1, 6))
	for _, id := range []string{nvdaID, aaplID} {
		_, err := e.st.InsertSnapshot(e.ctx, r, id, day(1, 7), "ok", snapshotJSON(id))
		require.NoError(t, err)
	}
	a := assets()
	a[1].Enabled = false
	_, err := e.st.SyncUnderlyings(e.ctx, a)
	require.NoError(t, err)
	latest, err := e.st.LatestSnapshotsForEnabled(e.ctx)
	require.NoError(t, err)
	require.Len(t, latest, 1)
	assert.Equal(t, nvdaID, latest[0].UnderlyingID)

	// an asset dropped from assets.json is disabled too
	_, err = e.st.SyncUnderlyings(e.ctx, a[:0])
	require.NoError(t, err)
	latest, err = e.st.LatestSnapshotsForEnabled(e.ctx)
	require.NoError(t, err)
	assert.Empty(t, latest)
}

func TestRunRequests(t *testing.T) {
	e := newEnv(t)
	ctx, st := e.ctx, e.st
	_, err := st.TakeNextRunRequest(ctx)
	assert.ErrorIs(t, err, store.ErrNotFound)

	first, err := st.CreateRunRequest(ctx, "admin-api", "refresh", nil)
	require.NoError(t, err)
	second, err := st.CreateRunRequest(ctx, "cli", "add", []string{"NVDA", "AAPL"})
	require.NoError(t, err)

	got, err := st.TakeNextRunRequest(ctx)
	require.NoError(t, err)
	assert.Equal(t, first, got.ID)
	assert.NotNil(t, got.TakenAt)
	assert.Nil(t, got.Symbols)

	got2, err := st.TakeNextRunRequest(ctx)
	require.NoError(t, err)
	assert.Equal(t, second, got2.ID)
	assert.Equal(t, []string{"NVDA", "AAPL"}, got2.Symbols)

	_, err = st.TakeNextRunRequest(ctx)
	assert.ErrorIs(t, err, store.ErrNotFound)

	runID := e.run(t, "r1", day(1, 6))
	require.NoError(t, st.AttachRunToRequest(ctx, first, runID))
	assert.ErrorIs(t, st.AttachRunToRequest(ctx, 999, runID), store.ErrNotFound)
	var attached int64
	require.NoError(t, e.pool.QueryRow(ctx, `SELECT run_id FROM run_requests WHERE id = $1`, first).Scan(&attached))
	assert.Equal(t, runID, attached)
}

func TestTakeNextRunRequestConcurrent(t *testing.T) {
	e := newEnv(t)
	for i := 0; i < 5; i++ {
		_, err := e.st.CreateRunRequest(e.ctx, "cli", "refresh", nil)
		require.NoError(t, err)
	}
	results := make(chan int64, 8)
	for i := 0; i < 8; i++ {
		go func() {
			r, err := e.st.TakeNextRunRequest(e.ctx)
			if err != nil {
				results <- 0
				return
			}
			results <- r.ID
		}()
	}
	seen := map[int64]bool{}
	for i := 0; i < 8; i++ {
		if id := <-results; id != 0 {
			assert.False(t, seen[id], "request %d taken twice", id)
			seen[id] = true
		}
	}
	assert.Len(t, seen, 5)
}

func TestListRuns(t *testing.T) {
	e := newEnv(t)
	for i := 1; i <= 5; i++ {
		e.run(t, fmt.Sprintf("r%d", i), day(i, 6))
	}
	page1, err := e.st.ListRuns(e.ctx, 2, nil)
	require.NoError(t, err)
	require.Len(t, page1, 2)
	assert.Equal(t, "r5", page1[0].RunKey)
	assert.Equal(t, "r4", page1[1].RunKey)
	page2, err := e.st.ListRuns(e.ctx, 2, &store.RunCursor{StartedAt: page1[1].StartedAt, ID: page1[1].ID})
	require.NoError(t, err)
	require.Len(t, page2, 2)
	assert.Equal(t, "r3", page2[0].RunKey)
	last, err := e.st.LastRun(e.ctx)
	require.NoError(t, err)
	assert.Equal(t, "r5", last.RunKey)
}

func TestDeactivateMissing(t *testing.T) {
	e := newEnv(t)
	ctx, st := e.ctx, e.st
	r1 := e.run(t, "r1", day(1, 6))
	r2 := e.run(t, "r2", day(2, 6))
	tok := func(id int64) store.TokenRow {
		return store.TokenRow{ID: id, UnderlyingID: nvdaID, Kind: "tokenized", Coverage: "search"}
	}
	require.NoError(t, st.UpsertTokens(ctx, r1, []store.TokenRow{tok(1), tok(2)}))
	require.NoError(t, st.UpsertTokens(ctx, r2, []store.TokenRow{tok(2), tok(3)}))
	n, err := st.DeactivateMissingTokens(ctx, r2)
	require.NoError(t, err)
	assert.EqualValues(t, 1, n)
	active := func() map[int64]bool {
		rows, err := e.pool.Query(ctx, `SELECT id, active FROM tokens`)
		require.NoError(t, err)
		defer rows.Close()
		out := map[int64]bool{}
		for rows.Next() {
			var id int64
			var a bool
			require.NoError(t, rows.Scan(&id, &a))
			out[id] = a
		}
		return out
	}
	assert.Equal(t, map[int64]bool{1: false, 2: true, 3: true}, active())

	var first int64
	require.NoError(t, e.pool.QueryRow(ctx, `SELECT first_seen_run_id FROM tokens WHERE id = 2`).Scan(&first))
	assert.Equal(t, r1, first, "first_seen_run_id is kept on update")

	// reappearing reactivates
	r3 := e.run(t, "r3", day(3, 6))
	require.NoError(t, st.UpsertTokens(ctx, r3, []store.TokenRow{tok(1)}))
	assert.True(t, active()[1])

	// every other entity: rows not seen in r3 flip to inactive, seen ones stay active
	exec := func(q string, args ...any) {
		_, err := e.pool.Exec(ctx, q, args...)
		require.NoError(t, err)
	}
	flag := func(table, where string) (a bool) {
		require.NoError(t, e.pool.QueryRow(ctx, `SELECT active FROM `+table+` WHERE `+where).Scan(&a))
		return a
	}
	pos := func(k string) store.PositionRow {
		return store.PositionRow{Key: k, Network: "ethereum", Platform: "p", Address: k, PositionKind: "contract", Category: "vault", LiquiditySemantics: "vault_tvl", Eligibility: "eligible"}
	}
	dep := func(addr string) store.DeploymentRow {
		return store.DeploymentRow{TokenID: 1, CMCNetwork: "Ethereum", Address: addr, AddressRaw: addr, Status: "indexed"}
	}
	require.NoError(t, st.UpsertDeployments(ctx, r2, []store.DeploymentRow{dep("a"), dep("b")}))
	require.NoError(t, st.UpsertNetworks(ctx, r2, []store.NetworkRow{{Slug: "ethereum"}, {Slug: "bsc"}}))
	require.NoError(t, st.UpsertPositions(ctx, r2, []store.PositionRow{pos("p1"), pos("p2")}))
	require.NoError(t, st.UpsertTokenPositions(ctx, r2, []store.TokenPositionRow{{TokenID: 1, PositionKey: "p1", Link: "direct", Depth: ptr(1)}, {TokenID: 1, PositionKey: "p2", Link: "unconfirmed"}}))
	require.NoError(t, st.UpsertDeployments(ctx, r3, []store.DeploymentRow{dep("a")}))
	require.NoError(t, st.UpsertNetworks(ctx, r3, []store.NetworkRow{{Slug: "ethereum"}}))
	require.NoError(t, st.UpsertPositions(ctx, r3, []store.PositionRow{pos("p1")}))
	require.NoError(t, st.UpsertTokenPositions(ctx, r3, []store.TokenPositionRow{{TokenID: 1, PositionKey: "p1", Link: "direct", Depth: ptr(1)}}))
	for _, f := range []func(context.Context, int64) (int64, error){st.DeactivateMissingDeployments, st.DeactivateMissingNetworks,
		st.DeactivateMissingPositions, st.DeactivateMissingTokenPositions} {
		n, err := f(ctx, r3)
		require.NoError(t, err)
		assert.EqualValues(t, 1, n)
	}
	assert.True(t, flag("deployments", "address = 'a'"))
	assert.False(t, flag("deployments", "address = 'b'"))
	assert.True(t, flag("networks", "slug = 'ethereum'"))
	assert.False(t, flag("networks", "slug = 'bsc'"))
	assert.True(t, flag("positions", "key = 'p1'"))
	assert.False(t, flag("positions", "key = 'p2'"))
	assert.True(t, flag("token_positions", "position_key = 'p1'"))
	assert.False(t, flag("token_positions", "position_key = 'p2'"))

	// rows with a NULL last_seen_run_id (networks allows it) are deactivated too
	exec(`UPDATE networks SET last_seen_run_id = NULL WHERE slug = 'ethereum'`)
	n, err = st.DeactivateMissingNetworks(ctx, r3)
	require.NoError(t, err)
	assert.EqualValues(t, 1, n)
}

func TestQueryHistory(t *testing.T) {
	e := newEnv(t)
	ctx, st := e.ctx, e.st
	// three runs on 2026-10-18 (06, 12, 20 UTC), one on the 19th; last run of each day wins for step=day
	hours := []struct {
		d, h int
		cap  string
	}{{18, 6, "100"}, {18, 12, "110.5"}, {18, 20, "120"}, {19, 6, "130"}}
	var runs []int64
	for i, h := range hours {
		r := e.run(t, fmt.Sprintf("r%d", i), day(h.d, h.h))
		runs = append(runs, r)
		require.NoError(t, st.InsertUnderlyingHistory(ctx, []store.UnderlyingHistoryRow{{RunID: r, UnderlyingID: nvdaID, ObservedAt: day(h.d, h.h),
			Status: "ok", TokenizedMarketCapUSD: dec(h.cap), VersionsCount: i + 1}}))
	}
	// another underlying must not leak in
	require.NoError(t, st.InsertUnderlyingHistory(ctx, []store.UnderlyingHistoryRow{{RunID: runs[0], UnderlyingID: aaplID, ObservedAt: day(18, 6),
		Status: "ok", TokenizedMarketCapUSD: dec("1"), VersionsCount: 1}}))

	pts, err := qh(st, ctx, store.EntityUnderlying, nvdaID, []string{"tokenized_market_cap_usd", "versions_count", "status"}, nil, nil, store.StepRun, 0)
	require.NoError(t, err)
	require.Len(t, pts, 4)
	assert.Equal(t, "100", pts[0].Values["tokenized_market_cap_usd"])
	assert.Equal(t, "110.5", pts[1].Values["tokenized_market_cap_usd"])
	assert.EqualValues(t, 4, pts[3].Values["versions_count"])
	assert.Equal(t, "ok", pts[0].Values["status"])
	assert.Equal(t, runs[0], pts[0].RunID)
	assert.True(t, pts[0].ObservedAt.Equal(day(18, 6)))

	pts, err = qh(st, ctx, store.EntityUnderlying, nvdaID, []string{"tokenized_market_cap_usd"}, nil, nil, store.StepDay, 0)
	require.NoError(t, err)
	require.Len(t, pts, 2)
	assert.Equal(t, "120", pts[0].Values["tokenized_market_cap_usd"], "last run of the day")
	assert.Equal(t, runs[2], pts[0].RunID)
	assert.Equal(t, "130", pts[1].Values["tokenized_market_cap_usd"])

	pts, err = qh(st, ctx, store.EntityUnderlying, nvdaID, []string{"tokenized_market_cap_usd"}, nil, nil, store.StepWeek, 0)
	require.NoError(t, err)
	require.Len(t, pts, 2, "Sunday 18 and Monday 19 October 2026 are in different ISO weeks")
	assert.Equal(t, "120", pts[0].Values["tokenized_market_cap_usd"], "last observation of the week")
	assert.Equal(t, "130", pts[1].Values["tokenized_market_cap_usd"])

	from, to := day(18, 12), day(18, 20)
	pts, err = qh(st, ctx, store.EntityUnderlying, nvdaID, []string{"versions_count"}, &from, &to, store.StepRun, 0)
	require.NoError(t, err)
	assert.Len(t, pts, 2)
	pts, err = qh(st, ctx, store.EntityUnderlying, nvdaID, []string{"versions_count"}, nil, nil, store.StepRun, 1)
	require.NoError(t, err)
	assert.Len(t, pts, 1, "limit")

	_, err = qh(st, ctx, store.EntityUnderlying, nvdaID, []string{"versions_count", "payload; DROP TABLE builds"}, nil, nil, store.StepRun, 0)
	require.ErrorIs(t, err, store.ErrUnknownMetric)
	var um *store.UnknownMetricError
	require.ErrorAs(t, err, &um)
	assert.Equal(t, "payload; DROP TABLE builds", um.Metric)
	_, err = qh(st, ctx, store.EntityUnderlying, nvdaID, []string{"price_usd"}, nil, nil, "", 0)
	assert.ErrorIs(t, err, store.ErrUnknownMetric, "metric of another entity")
	_, err = qh(st, ctx, "bogus", "x", nil, nil, nil, "", 0)
	assert.ErrorIs(t, err, store.ErrUnknownEntity)
	_, err = qh(st, ctx, store.EntityUnderlying, nvdaID, nil, nil, nil, "month", 0)
	assert.ErrorIs(t, err, store.ErrUnknownStep)

	// all metrics when none are named; token ids are numeric strings
	require.NoError(t, st.UpsertTokens(ctx, runs[0], []store.TokenRow{{ID: tokenID, UnderlyingID: nvdaID, Kind: "tokenized", Coverage: "search"}}))
	require.NoError(t, st.InsertTokenHistory(ctx, []store.TokenHistoryRow{{RunID: runs[0], TokenID: tokenID, ObservedAt: day(18, 6),
		PriceUSD: dec("211.75"), UsefulnessScore: 2, Breakdowns: []byte(`{"by_network":{"ethereum":{"raw":1,"eligible":0}}}`)}}))
	pts, err = qh(st, ctx, store.EntityToken, "36992", nil, nil, nil, store.StepDay, 0)
	require.NoError(t, err)
	require.Len(t, pts, 1)
	assert.Equal(t, "211.75", pts[0].Values["price_usd"])
	assert.Nil(t, pts[0].Values["market_cap_usd"])
	assert.Nil(t, pts[0].Values["rank_in_underlying"])
	assert.IsType(t, map[string]any{}, pts[0].Values["breakdowns"])
	_, err = qh(st, ctx, store.EntityToken, "abc", nil, nil, nil, "", 0)
	assert.Error(t, err)

	// summary has no id; positions return arrays
	require.NoError(t, st.InsertSummaryHistory(ctx, []store.SummaryHistoryRow{{RunID: runs[0], ObservedAt: day(18, 6), UnderlyingsCount: 125}}))
	pts, err = qh(st, ctx, store.EntitySummary, "", []string{"underlyings_count"}, nil, nil, "", 0)
	require.NoError(t, err)
	require.Len(t, pts, 1)
	assert.EqualValues(t, 125, pts[0].Values["underlyings_count"])

	pk := "ethereum:0xabc"
	require.NoError(t, st.UpsertPositions(ctx, runs[0], []store.PositionRow{{Key: pk, Network: "ethereum", Platform: "p", Address: "0xabc",
		PositionKind: "contract", Category: "vault", LiquiditySemantics: "vault_tvl", Eligibility: "eligible"}}))
	require.NoError(t, st.InsertPositionHistory(ctx, []store.PositionHistoryRow{{RunID: runs[0], PositionKey: pk, ObservedAt: day(18, 6),
		LiquidityUSD: dec("25000"), Eligibility: "eligible", Flags: []string{"apy_missing"}}}))
	pts, err = qh(st, ctx, store.EntityPosition, pk, []string{"liquidity_usd", "flags", "eligibility", "apy_percent"}, nil, nil, "", 0)
	require.NoError(t, err)
	require.Len(t, pts, 1)
	assert.Equal(t, "25000", pts[0].Values["liquidity_usd"])
	assert.Equal(t, []any{"apy_missing"}, pts[0].Values["flags"])
	assert.Nil(t, pts[0].Values["apy_percent"])

	// every whitelisted metric of every entity is a real column
	for _, ent := range []string{store.EntitySummary, store.EntityUnderlying, store.EntityToken, store.EntityIssuer, store.EntityPosition} {
		_, err := qh(st, ctx, ent, "1", nil, nil, nil, store.StepRun, 1)
		require.NoError(t, err, ent)
	}
}

func TestLogos(t *testing.T) {
	e := newEnv(t)
	ctx, st := e.ctx, e.st
	ok, err := st.HasLogo(ctx, "tokens", "36992")
	require.NoError(t, err)
	assert.False(t, ok)
	_, err = st.GetLogo(ctx, "tokens", "36992")
	assert.ErrorIs(t, err, store.ErrNotFound)

	l := store.Logo{Kind: "tokens", EntityID: "36992", Ext: "png", ContentType: "image/png", Bytes: []byte{0x89, 'P', 'N', 'G'}, SHA256: "aa", Source: "https://s2.coinmarketcap.com/static/img/coins/64x64/36992.png"}
	inserted, err := st.InsertLogoIfAbsent(ctx, l)
	require.NoError(t, err)
	assert.True(t, inserted)
	l2 := l
	l2.Bytes, l2.SHA256, l2.Source = []byte("other"), "bb", "manual"
	inserted, err = st.InsertLogoIfAbsent(ctx, l2)
	require.NoError(t, err)
	assert.False(t, inserted, "an existing logo is never overwritten")

	got, err := st.GetLogo(ctx, "tokens", "36992")
	require.NoError(t, err)
	assert.Equal(t, l.Bytes, got.Bytes)
	assert.Equal(t, "aa", got.SHA256)
	ok, err = st.HasLogo(ctx, "tokens", "36992")
	require.NoError(t, err)
	assert.True(t, ok)

	_, err = st.InsertLogoIfAbsent(ctx, store.Logo{Kind: "bogus", EntityID: "1", Ext: "png", ContentType: "image/png", Bytes: []byte{1}, SHA256: "x", Source: "manual"})
	assert.Error(t, err, "kind is constrained")
}

func TestBuildExportBytesVerbatim(t *testing.T) {
	e := newEnv(t)
	ctx, st := e.ctx, e.st
	runID := e.run(t, "r1", day(18, 20))
	export := []byte("{\"zz\":1,\"a\":2,\"mmm\":3}")
	sum := sha256.Sum256(export)
	sha := hex.EncodeToString(sum[:])
	var id int64
	require.NoError(t, db.WithTx(ctx, e.pool, func(tx pgx.Tx) error {
		var err error
		id, err = st.InsertBuild(ctx, tx, store.BuildRow{RunID: runID, GeneratedAt: day(18, 21), SchemaVersion: 1, ScoreVersion: "v1", Export: export, ExportSHA256: sha})
		if err != nil {
			return err
		}
		return st.SetCurrentBuild(ctx, tx, id)
	}))
	for _, get := range []func() (*store.Build, error){func() (*store.Build, error) { return st.CurrentBuild(ctx) }, func() (*store.Build, error) { return st.BuildByID(ctx, id) }} {
		b, err := get()
		require.NoError(t, err)
		assert.Equal(t, export, b.Export, "bytes come back unchanged, key order included")
		got := sha256.Sum256(b.Export)
		assert.Equal(t, b.ExportSHA256, hex.EncodeToString(got[:]))
	}
}

func TestSyncUnderlyingsTickerChanges(t *testing.T) {
	e := newEnv(t)
	ctx, st := e.ctx, e.st
	tickers := func() map[string]string { // enabled id -> ticker
		rows, err := e.pool.Query(ctx, `SELECT id, ticker FROM underlyings WHERE enabled`)
		require.NoError(t, err)
		defer rows.Close()
		out := map[string]string{}
		for rows.Next() {
			var id, tk string
			require.NoError(t, rows.Scan(&id, &tk))
			out[id] = tk
		}
		return out
	}
	// an ISIN is added to a previously ISIN-less ticker
	_, err := st.SyncUnderlyings(ctx, []model.Asset{{Ticker: "FOO", Name: "Foo", AssetType: "etf", Enabled: true}})
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"etf:ticker:FOO": "FOO"}, tickers())
	_, err = st.SyncUnderlyings(ctx, []model.Asset{{Ticker: "FOO", Name: "Foo", AssetType: "etf", ISIN: ptr("US0378331005"), Enabled: true}})
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"US0378331005": "FOO"}, tickers())

	// two tickers swapped between ISINs
	_, err = st.SyncUnderlyings(ctx, []model.Asset{
		{Ticker: "AAA", Name: "A", AssetType: "stock", ISIN: ptr(nvdaID), Enabled: true},
		{Ticker: "BBB", Name: "B", AssetType: "stock", ISIN: ptr(aaplID), Enabled: true},
	})
	require.NoError(t, err)
	_, err = st.SyncUnderlyings(ctx, []model.Asset{
		{Ticker: "BBB", Name: "B", AssetType: "stock", ISIN: ptr(nvdaID), Enabled: true},
		{Ticker: "AAA", Name: "A", AssetType: "stock", ISIN: ptr(aaplID), Enabled: true},
	})
	require.NoError(t, err)
	assert.Equal(t, map[string]string{nvdaID: "BBB", aaplID: "AAA"}, tickers())

	// a duplicate enabled ticker in the input is still rejected by the partial unique index
	_, err = st.SyncUnderlyings(ctx, []model.Asset{
		{Ticker: "DUP", Name: "A", AssetType: "stock", ISIN: ptr(nvdaID), Enabled: true},
		{Ticker: "DUP", Name: "B", AssetType: "stock", ISIN: ptr(aaplID), Enabled: true},
	})
	assert.Error(t, err)
}

func TestQueryHistoryLimitReturnsNewest(t *testing.T) {
	e := newEnv(t)
	ctx, st := e.ctx, e.st
	for i := 1; i <= 6; i++ {
		r := e.run(t, fmt.Sprintf("r%d", i), day(i, 6))
		require.NoError(t, st.InsertUnderlyingHistory(ctx, []store.UnderlyingHistoryRow{{RunID: r, UnderlyingID: nvdaID, ObservedAt: day(i, 6), Status: "ok", VersionsCount: i}}))
	}
	res, err := st.QueryHistory(ctx, store.EntityUnderlying, nvdaID, []string{"versions_count"}, nil, nil, store.StepRun, 3)
	require.NoError(t, err)
	assert.True(t, res.Truncated)
	var got []any
	for _, p := range res.Points {
		got = append(got, p.Values["versions_count"])
	}
	assert.Equal(t, []any{int32(4), int32(5), int32(6)}, got, "the newest three, ascending")

	res, err = st.QueryHistory(ctx, store.EntityUnderlying, nvdaID, []string{"versions_count"}, nil, nil, store.StepDay, 3)
	require.NoError(t, err)
	assert.True(t, res.Truncated)
	assert.Len(t, res.Points, 3)
	assert.EqualValues(t, 6, res.Points[2].Values["versions_count"])

	res, err = st.QueryHistory(ctx, store.EntityUnderlying, nvdaID, []string{"versions_count"}, nil, nil, store.StepRun, 6)
	require.NoError(t, err)
	assert.False(t, res.Truncated)
	assert.Len(t, res.Points, 6)

	empty, err := st.QueryHistory(ctx, store.EntityUnderlying, "nope", []string{"versions_count"}, nil, nil, store.StepRun, 0)
	require.NoError(t, err)
	assert.NotNil(t, empty.Points)
	assert.Empty(t, empty.Points)
}

func TestTimesAreUTC(t *testing.T) {
	e := newEnv(t)
	ctx := e.ctx
	r := e.run(t, "r1", time.Date(2026, 10, 18, 23, 0, 0, 0, time.FixedZone("x", 3*3600)))
	run, err := e.st.GetRun(ctx, r)
	require.NoError(t, err)
	assert.Equal(t, time.UTC, run.StartedAt.Location())
	assert.True(t, run.StartedAt.Equal(day(18, 20)))
	require.NoError(t, e.st.InsertUnderlyingHistory(ctx, []store.UnderlyingHistoryRow{{RunID: r, UnderlyingID: nvdaID, ObservedAt: day(18, 20), Status: "ok"}}))
	pts, err := qh(e.st, ctx, store.EntityUnderlying, nvdaID, []string{"status"}, nil, nil, "", 0)
	require.NoError(t, err)
	assert.Equal(t, time.UTC, pts[0].ObservedAt.Location())
}

func TestPublishBuildAndRunRequestNormalization(t *testing.T) {
	e := newEnv(t)
	ctx, st := e.ctx, e.st
	runID := e.run(t, "r1", day(18, 20))
	var first, second int64
	require.NoError(t, db.WithTx(ctx, e.pool, func(tx pgx.Tx) (err error) {
		first, err = st.PublishBuild(ctx, tx, store.BuildRow{RunID: runID, GeneratedAt: day(18, 21), SchemaVersion: 1, ScoreVersion: "v1", Export: []byte(`{}`), ExportSHA256: "a"}, nil)
		return err
	}))
	require.NoError(t, db.WithTx(ctx, e.pool, func(tx pgx.Tx) (err error) {
		second, err = st.PublishBuild(ctx, tx, store.BuildRow{RunID: runID, GeneratedAt: day(18, 22), SchemaVersion: 1, ScoreVersion: "v1", Export: []byte(`{}`), ExportSHA256: "b"},
			&store.BuildChangesRow{PreviousBuildID: &first, NewTokens: []int32{tokenID}, EligibleDelta: 2})
		return err
	}))
	cur, err := st.CurrentBuild(ctx)
	require.NoError(t, err)
	assert.Equal(t, second, cur.ID)
	var delta int
	var prev int64
	require.NoError(t, e.pool.QueryRow(ctx, `SELECT eligible_delta, previous_build_id FROM build_changes WHERE build_id = $1`, second).Scan(&delta, &prev))
	assert.Equal(t, 2, delta)
	assert.Equal(t, first, prev)

	id, err := st.CreateRunRequest(ctx, "cli", "refresh", []string{})
	require.NoError(t, err)
	var isNull bool
	require.NoError(t, e.pool.QueryRow(ctx, `SELECT symbols IS NULL FROM run_requests WHERE id = $1`, id).Scan(&isNull))
	assert.True(t, isNull, "empty symbols means all enabled")

	runs, err := st.ListRuns(ctx, 0, &store.RunCursor{StartedAt: day(1, 0), ID: 1})
	require.NoError(t, err)
	assert.NotNil(t, runs)
	assert.Empty(t, runs)
}
