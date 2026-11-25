package collect

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/curated"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/model"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/source/cmc"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/source/deals"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/source/portals"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/store"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/testdb"
)

// A snapshot without tokens never replaces one that had tokens (collect.py::sync): the previous snapshot stays and the
// underlying counts as failed, with the message of the pipeline.
func TestEmptySnapshotNeverReplacesOneWithTokens(t *testing.T) {
	pool, _ := testdb.Migrated(t)
	f := startFakes(t, nil)
	base := time.Now()
	r := newRunner(t, pool, f, twoAssets()).WithClock(func() time.Time { return base })
	ctx := context.Background()

	res, err := r.Run(ctx, Options{Mode: ModeRefresh, Symbols: []string{"NVDA"}})
	require.NoError(t, err)
	require.Equal(t, RunSucceeded, res.Status)
	counts, err := store.New(pool).LatestSnapshotTokenCounts(ctx)
	require.NoError(t, err)
	tokens := counts["US67066G1040"]
	require.Positive(t, tokens)

	// CMC suddenly knows nothing about NVDA: the answer would store a not_in_cmc snapshot.
	f.emptyQuotes.Store(true)
	r.WithClock(func() time.Time { return base.Add(2 * time.Hour) })
	res, err = r.Run(ctx, Options{Mode: ModeRefresh, Symbols: []string{"NVDA"}})
	require.NoError(t, err)
	assert.Equal(t, RunFailed, res.Status, "the only underlying failed and nothing completed")
	assert.Zero(t, res.Completed)
	assert.Equal(t, 1, res.Failed)
	assert.Equal(t, 1, countRows(t, pool, `SELECT count(*) FROM raw_snapshots`), "the previous snapshot is the only one")

	run, err := store.New(pool).GetRun(ctx, res.RunID)
	require.NoError(t, err)
	require.Len(t, run.Underlyings, 1)
	assert.Equal(t, "failed", run.Underlyings[0].Status)
	require.NotNil(t, run.Underlyings[0].Error)
	assert.Equal(t, fmt.Sprintf("not_in_cmc: previous snapshot with %d tokens kept", tokens), *run.Underlyings[0].Error)
	assert.Nil(t, run.Error, "an underlying failure is not a fatal error of the run")

	// With the roster back, a refresh replaces the snapshot as usual.
	f.emptyQuotes.Store(false)
	r.WithClock(func() time.Time { return base.Add(4 * time.Hour) })
	res, err = r.Run(ctx, Options{Mode: ModeRefresh, Symbols: []string{"NVDA"}})
	require.NoError(t, err)
	assert.Equal(t, RunSucceeded, res.Status)
	assert.Equal(t, 2, countRows(t, pool, `SELECT count(*) FROM raw_snapshots`))
}

// A snapshot that had no tokens may be replaced by another one without tokens, and an underlying that was never
// collected gets its not_in_cmc snapshot: the guard protects data, it does not forbid empty answers.
func TestEmptySnapshotMayReplaceAnEmptyOne(t *testing.T) {
	pool, _ := testdb.Migrated(t)
	f := startFakes(t, nil)
	f.emptyQuotes.Store(true)
	base := time.Now()
	r := newRunner(t, pool, f, twoAssets()).WithClock(func() time.Time { return base })
	ctx := context.Background()

	res, err := r.Run(ctx, Options{Mode: ModeRefresh})
	require.NoError(t, err)
	assert.Equal(t, RunSucceeded, res.Status)
	assert.Equal(t, 2, res.Completed)
	r.WithClock(func() time.Time { return base.Add(time.Hour) })
	res, err = r.Run(ctx, Options{Mode: ModeRefresh})
	require.NoError(t, err)
	assert.Equal(t, RunSucceeded, res.Status)
	assert.Equal(t, 4, countRows(t, pool, `SELECT count(*) FROM raw_snapshots`))
}

func linksWithDeals(t *testing.T, slugs ...string) *curated.Config {
	t.Helper()
	platforms := ""
	for i, s := range slugs {
		if i > 0 {
			platforms += ","
		}
		platforms += `"p` + s + `":{"home":"https://example.org","hosts":["example.org"],"deal":"` + s + `"}`
	}
	links, err := curated.ParseLinks("links.json", []byte(`{"schema_version":1,"platforms":{`+platforms+`}}`))
	require.NoError(t, err)
	return &curated.Config{Links: links}
}

const dealsBody = `{"deals":[
	{"slug":"backpack","title":"Backpack","link":"https://backpack.exchange/refer/cp0x","bonus":"b","page_url":"https://cp0x.com/deals/backpack"},
	{"slug":"hyperliquid","title":"HL","link":"http://insecure.example.org","bonus":"x"},
	{"slug":"other","title":"Other","link":"https://other.example.org/r"}]}`

func dealsServer(t *testing.T, status *atomic.Int64, hits *atomic.Int64) *deals.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		if st := int(status.Load()); st != 200 {
			http.Error(w, "down", st)
			return
		}
		_, _ = w.Write([]byte(dealsBody))
	}))
	t.Cleanup(srv.Close)
	return deals.New(nil).WithURL(srv.URL)
}

// The collector fetches the deals once per run after Portals: only the named slugs are stored, an unusable link is a
// warning, a dead source is a warning and the previous deals stay; none of it fails the run.
func TestRunFetchesDealsOncePerRun(t *testing.T) {
	pool, _ := testdb.Migrated(t)
	f := startFakes(t, nil)
	var status, hits atomic.Int64
	status.Store(200)
	base := time.Now()
	r := journalRunner(t, pool, f, twoAssets(), slog.LevelInfo).
		WithCurated(linksWithDeals(t, "backpack", "hyperliquid")).WithDealsClient(dealsServer(t, &status, &hits)).
		WithClock(func() time.Time { return base })
	ctx := context.Background()

	res, err := r.Run(ctx, Options{Mode: ModeRefresh})
	require.NoError(t, err)
	assert.Equal(t, RunSucceeded, res.Status)
	assert.EqualValues(t, 1, hits.Load(), "once per run, not per underlying")
	assert.Equal(t, []string{"deals: skipped unusable links for hyperliquid"}, res.Warnings)

	st := store.New(pool)
	payload, err := st.LatestDeals(ctx)
	require.NoError(t, err)
	cache := curated.ParseDealsCache(payload)
	require.NotNil(t, cache)
	assert.Len(t, cache.Deals, 1, "only deals the links config names; the unusable one is skipped")
	assert.Contains(t, cache.Deals, "backpack")
	assert.NotContains(t, cache.Deals, "other")
	assert.Equal(t, 1, countRows(t, pool, `SELECT count(*) FROM raw_cp0x_deals`))

	events, _, err := st.ListRunEvents(ctx, res.RunID, 0, "debug", 2000)
	require.NoError(t, err)
	assert.Positive(t, steps(events)["collect.deals"], "step=collect.deals in the journal")

	// A dead deals source: a warning, the run goes on, the previous row stays.
	status.Store(503)
	r.WithClock(func() time.Time { return base.Add(time.Hour) })
	res, err = r.Run(ctx, Options{Mode: ModeRefresh})
	require.NoError(t, err)
	assert.Equal(t, RunSucceeded, res.Status)
	assert.Equal(t, 2, res.Completed)
	assert.Equal(t, []string{"deals: HTTP 503"}, res.Warnings)
	assert.Equal(t, 1, countRows(t, pool, `SELECT count(*) FROM raw_cp0x_deals`))
	run, err := st.GetRun(ctx, res.RunID)
	require.NoError(t, err)
	assert.Contains(t, string(run.Warnings), "deals: HTTP 503")
}

func TestRunWithoutLinksConfigDoesNotFetchDeals(t *testing.T) {
	pool, _ := testdb.Migrated(t)
	f := startFakes(t, nil)
	var status, hits atomic.Int64
	status.Store(200)
	for i, cur := range []*curated.Config{nil, {}} {
		at := time.Now().Add(time.Duration(i) * time.Hour)
		r := newRunner(t, pool, f, twoAssets()).WithCurated(cur).WithDealsClient(dealsServer(t, &status, &hits)).
			WithClock(func() time.Time { return at })
		res, err := r.Run(context.Background(), Options{Mode: ModeRefresh, Symbols: []string{"NVDA"}})
		require.NoError(t, err)
		assert.Equal(t, RunSucceeded, res.Status)
	}
	assert.Zero(t, hits.Load())
	assert.Zero(t, countRows(t, pool, `SELECT count(*) FROM raw_cp0x_deals`))
}

type recordingPortals struct{ terms []string }

func (p *recordingPortals) Lookup(context.Context, []string, int) (map[string]json.RawMessage, error) {
	return map[string]json.RawMessage{}, nil
}

func (p *recordingPortals) Search(_ context.Context, term string, _ int) (map[string]json.RawMessage, portals.SearchMeta, error) {
	p.terms = append(p.terms, term)
	return map[string]json.RawMessage{}, portals.SearchMeta{Complete: true}, nil
}

// An alias keeps a version whose symbol differs from the ticker among the tokenized ones, and its symbol is searched in
// Portals like the ticker (collect.py::collect_asset passes asset.symbol_aliases to classify_token).
func TestAliasKeepsTheVersionInTheSearchTerms(t *testing.T) {
	token := func(id int64, symbol string) model.CMCToken { return model.CMCToken{CryptoID: &id, Symbol: &symbol} }
	rwa := &cmc.RWAAsset{Raw: json.RawMessage(`{}`)}
	rwa.Tokens = []model.CMCToken{token(1, "SPCX"), token(2, "SPACEXon"), token(3, "XYZ")}
	settings := loadSettings(t)
	for _, tc := range []struct {
		aliases []string
		want    []string
	}{{nil, []string{"SPCX"}}, {[]string{"SPACEX"}, []string{"SPCX", "SPACEXon"}}} {
		asset := model.Asset{Ticker: "SPCX", Name: "SpaceX", AssetType: "stock", Enabled: true, SymbolAliases: tc.aliases}
		api := &recordingPortals{}
		_, status, err := collectAsset(context.Background(), slog.New(slog.DiscardHandler), asset, rwa, "", shared{}, api, settings, "run", time.Now)
		require.NoError(t, err)
		assert.Equal(t, StatusOK, status)
		assert.Equal(t, tc.want, api.terms, "aliases %v", tc.aliases)
	}
}
