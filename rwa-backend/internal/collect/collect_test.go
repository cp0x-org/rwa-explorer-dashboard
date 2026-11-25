package collect

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"slices"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/compute"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/model"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/source/cmc"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/source/portals"
)

func testAssets() []model.Asset {
	isin := func(s string) *string { return &s }
	return []model.Asset{
		{Ticker: "NVDA", Name: "NVIDIA", AssetType: "stock", ISIN: isin("US67066G1040"), Enabled: true},
		{Ticker: "AAPL", Name: "Apple", AssetType: "stock", ISIN: isin("US0378331005"), Enabled: true},
		{Ticker: "OLD", Name: "Old", AssetType: "stock", Enabled: false},
		{Ticker: "ARKK", Name: "Ark", AssetType: "etf", Enabled: true},
	}
}

func tickers(as []model.Asset) []string {
	var out []string
	for _, a := range as {
		out = append(out, a.Ticker)
	}
	return out
}

func TestSelectAssets(t *testing.T) {
	as := testAssets()
	all, err := SelectAssets(as, nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"NVDA", "AAPL", "ARKK"}, tickers(all), "disabled assets are dropped")

	some, err := SelectAssets(as, []string{" aapl ", "NVDA", ""})
	require.NoError(t, err)
	assert.Equal(t, []string{"NVDA", "AAPL"}, tickers(some), "assets.json order, case-insensitive")

	_, err = SelectAssets(as, []string{"NVDA", "OLD", "ZZZ"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown or disabled symbols: OLD, ZZZ")
}

func TestTodo(t *testing.T) {
	sel, err := SelectAssets(testAssets(), nil)
	require.NoError(t, err)
	have := map[string]bool{"US67066G1040": true}
	assert.Equal(t, []string{"NVDA", "AAPL", "ARKK"}, tickers(Todo(sel, ModeRefresh, have)), "refresh fetches everything")
	assert.Equal(t, []string{"AAPL", "ARKK"}, tickers(Todo(sel, ModeAdd, have)), "add skips underlyings with a snapshot")
	assert.Empty(t, Todo(sel, ModeAdd, map[string]bool{"US67066G1040": true, "US0378331005": true, "etf:ticker:ARKK": true}))
}

func TestDecideStatus(t *testing.T) {
	for _, tc := range []struct {
		name               string
		completed, failed  int
		fatal, interrupted bool
		want               string
	}{
		{"all ok", 3, 0, false, false, RunSucceeded},
		{"some failed", 2, 1, false, false, RunPartial},
		{"fatal before anything", 0, 0, true, false, RunFailed},
		{"auth failure mid-run", 2, 1, true, false, RunPartial},
		{"auth failure on the first underlying", 0, 1, true, false, RunFailed},
		{"interrupted", 2, 0, false, true, RunFailed},
		{"all underlyings failed, no fatal", 0, 3, false, false, RunFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, DecideStatus(tc.completed, tc.failed, tc.fatal, tc.interrupted))
		})
	}
}

// v1Snapshot is the recorded 1.x snapshot of NVDA.
func v1Snapshot(t *testing.T) (raw []byte, doc map[string]any) {
	t.Helper()
	raw, err := os.ReadFile("../../testdata/snapshots/nvda_snapshot.json")
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &doc))
	return raw, doc
}

// fakePortals replays the portals part of a recorded snapshot.
type fakePortals struct {
	exact, records map[string]json.RawMessage
	searches       map[string]portals.SearchMeta
	searched       []string
	err            error
}

func (f *fakePortals) Lookup(_ context.Context, _ []string, _ int) (map[string]json.RawMessage, error) {
	return f.exact, f.err
}

func (f *fakePortals) Search(_ context.Context, term string, _ int) (map[string]json.RawMessage, portals.SearchMeta, error) {
	f.searched = append(f.searched, term)
	if f.err != nil {
		return nil, portals.SearchMeta{}, f.err
	}
	return f.records, f.searches[term], nil
}

// fromSnapshot rebuilds the inputs collectAsset gets from the sources out of a recorded snapshot.
func fromSnapshot(t *testing.T, raw []byte) (*cmc.RWAAsset, shared, *fakePortals) {
	t.Helper()
	var doc struct {
		CMC struct {
			RWA  json.RawMessage            `json:"rwa"`
			Info map[string]json.RawMessage `json:"info"`
		} `json:"cmc"`
		Portals struct {
			Networks []string                      `json:"networks"`
			Exact    map[string]json.RawMessage    `json:"exact"`
			Searches map[string]portals.SearchMeta `json:"searches"`
			Records  map[string]json.RawMessage    `json:"records"`
		} `json:"portals"`
	}
	require.NoError(t, json.Unmarshal(raw, &doc))
	rwa := &cmc.RWAAsset{Raw: doc.CMC.RWA}
	require.NoError(t, json.Unmarshal(doc.CMC.RWA, &rwa.CMCRwa))
	sh := shared{info: map[int]cmc.Info{}, networks: doc.Portals.Networks}
	for k, v := range doc.CMC.Info {
		var id int
		require.NoError(t, json.Unmarshal([]byte(k), &id))
		in := cmc.Info{Raw: v}
		require.NoError(t, json.Unmarshal(v, &in.CMCInfo))
		sh.info[id] = in
	}
	return rwa, sh, &fakePortals{exact: doc.Portals.Exact, records: doc.Portals.Records, searches: doc.Portals.Searches}
}

func fixedClock() time.Time { return time.Date(2026, 10, 8, 21, 11, 23, 0, time.UTC) }

func nvdaAsset() model.Asset { return testAssets()[0] }

func loadSettings(t *testing.T) *model.Settings {
	t.Helper()
	s, err := model.LoadSettings("../../config/settings.json")
	require.NoError(t, err)
	return s
}

// The payload equals the recorded 1.x snapshot (everything but run_id), with the same field set at every level.
func TestCollectAssetMatchesV1Snapshot(t *testing.T) {
	raw, want := v1Snapshot(t)
	rwa, sh, api := fromSnapshot(t, raw)

	payload, status, err := collectAsset(context.Background(), slog.New(slog.DiscardHandler), nvdaAsset(), rwa, "", sh, api,
		loadSettings(t), "20261008T211051Z", fixedClock)
	require.NoError(t, err)
	assert.Equal(t, StatusOK, status)
	assert.Equal(t, []string{"NVDA"}, api.searched, "search terms follow MinimalTerms(ticker + tokenized symbols)")

	var got map[string]any
	require.NoError(t, json.Unmarshal(payload, &got))
	assert.Equal(t, keys(want), keys(got))
	assert.Equal(t, keys(want["cmc"].(map[string]any)), keys(got["cmc"].(map[string]any)))
	assert.Equal(t, keys(want["portals"].(map[string]any)), keys(got["portals"].(map[string]any)))
	// The recorded fixture predates date_added/logo in the info subset; 1.x now writes them (null when absent).
	for id, info := range got["cmc"].(map[string]any)["info"].(map[string]any) {
		m := info.(map[string]any)
		assert.Equal(t, slices.Sorted(slices.Values(infoKeys)), keys(m), id)
		for _, k := range []string{"date_added", "logo"} {
			if _, old := want["cmc"].(map[string]any)["info"].(map[string]any)[id].(map[string]any)[k]; !old {
				assert.Nil(t, m[k], id+" "+k)
				delete(m, k)
			}
		}
	}
	for term, meta := range want["portals"].(map[string]any)["searches"].(map[string]any) {
		assert.Equal(t, keys(meta.(map[string]any)), keys(got["portals"].(map[string]any)["searches"].(map[string]any)[term].(map[string]any)), term)
	}
	for _, k := range []string{"cmc", "portals"} {
		for sub, v := range want[k].(map[string]any) {
			if sub == "looked_up" || sub == "info" { // the fixture predates the current deployment parsing and info subset
				continue
			}
			assert.Equal(t, v, got[k].(map[string]any)[sub], k+"."+sub)
		}
	}
}

func keys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func TestPayloadRoundTripsAndBuilds(t *testing.T) {
	raw, _ := v1Snapshot(t)
	rwa, sh, api := fromSnapshot(t, raw)
	settings := loadSettings(t)
	payload, _, err := collectAsset(context.Background(), slog.New(slog.DiscardHandler), nvdaAsset(), rwa, "", sh, api, settings, "k", fixedClock)
	require.NoError(t, err)

	snap, err := model.ParseSnapshot(payload)
	require.NoError(t, err)
	assert.Equal(t, 1, snap.SchemaVersion)
	assert.Equal(t, "US67066G1040", snap.UnderlyingID)
	assert.Equal(t, "ok", snap.Status)
	assert.Equal(t, fixedClock(), snap.FetchedAt)
	require.NotNil(t, snap.Portals)
	assert.NotEmpty(t, snap.Portals.Records)

	res, err := compute.Build([]*model.Snapshot{snap}, nil, settings, nil)
	require.NoError(t, err)
	assert.NotEmpty(t, res.Export.Tokens)
}

func TestCollectAssetNonOKStatuses(t *testing.T) {
	log := slog.New(slog.DiscardHandler)
	settings := loadSettings(t)

	for _, reason := range []string{cmc.ReasonNotInCMC, cmc.ReasonRWAIDChanged, cmc.ReasonAmbiguous} {
		payload, status, err := collectAsset(context.Background(), log, nvdaAsset(), nil, reason, shared{}, &fakePortals{}, settings, "k", fixedClock)
		require.NoError(t, err)
		assert.Equal(t, reason, status)
		var doc map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(payload, &doc))
		assert.JSONEq(t, "null", string(doc["cmc"]))
		assert.JSONEq(t, "null", string(doc["portals"]))
		snap, err := model.ParseSnapshot(payload)
		require.NoError(t, err)
		assert.Nil(t, snap.CMC)
	}

	rwa := &cmc.RWAAsset{Raw: json.RawMessage(`{"name":"X","symbol":"NVDA","rwa_id":2,"asset_type":"stock","tokens":[]}`)}
	require.NoError(t, json.Unmarshal(rwa.Raw, &rwa.CMCRwa))
	payload, status, err := collectAsset(context.Background(), log, nvdaAsset(), rwa, "", shared{}, &fakePortals{}, settings, "k", fixedClock)
	require.NoError(t, err)
	assert.Equal(t, StatusNoTrackedTokens, status)
	var doc struct {
		CMC     map[string]json.RawMessage `json:"cmc"`
		Portals json.RawMessage            `json:"portals"`
	}
	require.NoError(t, json.Unmarshal(payload, &doc))
	assert.JSONEq(t, "{}", string(doc.CMC["info"]))
	assert.JSONEq(t, "[]", string(doc.CMC["invalid_ids"]))
	assert.JSONEq(t, "null", string(doc.Portals))
}

func TestCollectAssetSourceErrorStoresNothing(t *testing.T) {
	raw, _ := v1Snapshot(t)
	rwa, sh, api := fromSnapshot(t, raw)
	api.err = errors.New("boom")
	payload, _, err := collectAsset(context.Background(), slog.New(slog.DiscardHandler), nvdaAsset(), rwa, "", sh, api, loadSettings(t), "k", fixedClock)
	require.Error(t, err)
	assert.Nil(t, payload)
}

// The field set of a 1.x snapshot (testdata/snapshots/v1) equals ours at every level.
func TestFieldSetMatchesCurrentV1Snapshot(t *testing.T) {
	b, err := os.ReadFile("../../testdata/snapshots/v1/US67066G1040.json")
	require.NoError(t, err)
	var want map[string]any
	require.NoError(t, json.Unmarshal(b, &want))
	rwa, sh, api := fromSnapshot(t, b)
	payload, _, err := collectAsset(context.Background(), slog.New(slog.DiscardHandler), nvdaAsset(), rwa, "", sh, api, loadSettings(t),
		want["run_id"].(string), func() time.Time { return mustTime(t, want["fetched_at"].(string)) })
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(payload, &got))
	assert.Equal(t, want, got, "whole document equals the 1.x snapshot")
	assert.Equal(t, keys(want), keys(got))
	assert.Equal(t, keys(want["cmc"].(map[string]any)), keys(got["cmc"].(map[string]any)))
	assert.Equal(t, keys(want["portals"].(map[string]any)), keys(got["portals"].(map[string]any)))
	for id, info := range want["cmc"].(map[string]any)["info"].(map[string]any) {
		assert.Equal(t, keys(info.(map[string]any)), keys(got["cmc"].(map[string]any)["info"].(map[string]any)[id].(map[string]any)), id)
	}
	for term, meta := range want["portals"].(map[string]any)["searches"].(map[string]any) {
		assert.Equal(t, keys(meta.(map[string]any)), keys(got["portals"].(map[string]any)["searches"].(map[string]any)[term].(map[string]any)), term)
	}
}

func mustTime(t *testing.T, s string) time.Time {
	t.Helper()
	v, err := time.Parse(time.RFC3339, s)
	require.NoError(t, err)
	return v
}
