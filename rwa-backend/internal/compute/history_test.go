package compute

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/model"
)

func TestHistoryRowsFixture(t *testing.T) {
	res := loadFixture(t)
	exp := Finalize(res, time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC), nil, nil)
	h, err := HistoryRows(exp, &res.Side, 7)
	require.NoError(t, err)
	observed := time.Date(2026, 10, 8, 21, 11, 23, 0, time.UTC)

	s := h.Summary
	assert.Equal(t, int64(7), s.RunID)
	assert.True(t, observed.Equal(s.ObservedAt), "summary observed_at is newest_fetched_at")
	assert.Equal(t, 4, s.TokensCount)
	assert.Equal(t, 1, s.ExcludedTokensCount)
	assert.Equal(t, 13, s.DeploymentsCount)
	assert.Equal(t, 7, s.EligibleOpportunitiesCount)
	assert.Equal(t, 2, s.IndirectEligibleCount)
	assert.Equal(t, 5, s.YieldOpportunitiesCount)
	requireDec(t, "80.158", s.MaxReportedAPYPercent)
	requireDec(t, "151803344.10364377", s.TokenizedMarketCapUSD)
	requireDec(t, "90664811.44390228", s.TokenizedVolume24hUSD)
	// 7 eligible confirmed positions; the unconfirmed morpho market (271270) is not part of it
	requireDec(t, "2973347.457218343544", s.EligibleLiquidityUSD)

	require.Len(t, h.Underlyings, 1)
	u := h.Underlyings[0]
	assert.Equal(t, "US67066G1040", u.UnderlyingID)
	assert.True(t, observed.Equal(u.ObservedAt))
	assert.Equal(t, "ok", u.Status)
	requireDec(t, "2973347.457218343544", u.EligibleLiquidityUSD)
	assert.Equal(t, 4, u.VersionsCount)
	requireDec(t, "231.21263591548592", u.AverageTokenizedPriceUSD)

	require.Len(t, h.Tokens, 5)
	byID := map[int64]int{}
	for i, r := range h.Tokens {
		byID[r.TokenID] = i
	}
	nvdac := h.Tokens[byID[42461]]
	assert.Equal(t, 7, nvdac.UsefulnessScore)
	require.NotNil(t, nvdac.RankInUnderlying)
	assert.Equal(t, 1, *nvdac.RankInUnderlying)
	assert.Equal(t, 1, nvdac.UnconfirmedCandidatesCount)
	assert.Equal(t, 1, nvdac.DeploymentsCount)
	assert.Equal(t, 1, nvdac.IndexedDeploymentsCount)
	requireDec(t, "2973347.457218343544", nvdac.EligibleLiquidityUSD)
	requireDec(t, "231.2421159978036", nvdac.PriceUSD)
	var bd struct {
		ByNetwork  map[string]model.BreakdownCount `json:"by_network"`
		ByCategory json.RawMessage                 `json:"by_category"`
		ByPlatform json.RawMessage                 `json:"by_platform"`
	}
	require.NoError(t, json.Unmarshal(nvdac.Breakdowns, &bd))
	assert.Equal(t, model.BreakdownCount{Raw: 13, Eligible: 7}, bd.ByNetwork["base"])
	assert.JSONEq(t, `{"dex_lp":{"raw":7,"eligible":4},"vault":{"raw":4,"eligible":3},"yield_market":{"raw":2,"eligible":0}}`, string(bd.ByCategory))
	assert.Contains(t, string(nvdac.Breakdowns), `"by_platform":{"uniswap-v4":{"raw":4,"eligible":2},"beefy-clm"`, "key order is kept")
	nvdax := h.Tokens[byID[36992]]
	requireDec(t, "0", nvdax.EligibleLiquidityUSD, "no eligible position: a sum of nothing is 0")
	assert.Equal(t, 6, nvdax.DeploymentsCount)
	assert.Nil(t, h.Tokens[byID[38153]].RankInUnderlying, "excluded tokens are not ranked")

	require.Len(t, h.Issuers, 3)
	c := h.Issuers[0]
	assert.Equal(t, "6a72fe7072996604d45c6733", c.IssuerID)
	assert.Equal(t, 7, c.IssuerUsefulnessScore)
	requireDec(t, "100", c.BasketRelativeScore)
	requireDec(t, "2099106.17", c.DirectPoolsLiquidityUSD)
	assert.Contains(t, string(c.Breakdowns), `"by_underlying":{"NVDA":7}`)
	assert.Nil(t, c.RosterSize)

	require.Len(t, h.Positions, 17)
	for _, p := range h.Positions {
		assert.True(t, observed.Equal(p.ObservedAt), p.PositionKey)
		assert.NotEmpty(t, p.Eligibility)
	}
	for _, p := range h.Positions {
		if p.PositionKey == "base:0x853f5f1b92b16714fe6cda67caad0856b83c7ab9" {
			assert.Equal(t, "eligible", p.Eligibility)
			assert.Equal(t, []string{"sentinel_price", "range_details_unknown"}, p.Flags)
			requireDec(t, "80.158", p.APYPercent)
			require.NotNil(t, p.SourceUpdatedAt)
		}
	}
}

func TestHistoryRowsSharedPositionCountedOncePerEntity(t *testing.T) {
	eligible := func(key string, liquidity string) model.Opportunity {
		return model.Opportunity{Key: key, Eligibility: "eligible", LiquidityUSD: model.MustDecimal(liquidity)}
	}
	rel := func(keys ...string) []model.Relation {
		var out []model.Relation
		for _, k := range keys {
			out = append(out, model.Relation{Key: k, Depth: 1})
		}
		return out
	}
	issuer := "I"
	exp := &model.Export{
		GeneratedAt: "2026-11-01T00:00:00Z",
		Underlyings: []model.Underlying{{ID: "U"}},
		Tokens: []model.Token{
			{ID: 1, UnderlyingID: "U", IssuerID: &issuer, Kind: "tokenized", Relations: rel("shared", "only1")},
			{ID: 2, UnderlyingID: "U", IssuerID: &issuer, Kind: "wrapper", Relations: rel("shared", "only2")},
			{ID: 3, UnderlyingID: "U", IssuerID: &issuer, Kind: "derivative", Relations: rel("deriv")},
		},
		Issuers: []model.Issuer{{ID: "I"}},
		Opportunities: []model.Opportunity{
			eligible("shared", "100"), eligible("only1", "10"), eligible("only2", "1"), eligible("deriv", "1000"),
			{Key: "stale", Eligibility: "stale", LiquidityUSD: model.MustDecimal("5")},
		},
	}
	h, err := HistoryRows(exp, nil, 1)
	require.NoError(t, err)
	requireDec(t, "110", h.Tokens[0].EligibleLiquidityUSD)
	requireDec(t, "101", h.Tokens[1].EligibleLiquidityUSD)
	requireDec(t, "1000", h.Tokens[2].EligibleLiquidityUSD)
	requireDec(t, "111", h.Underlyings[0].EligibleLiquidityUSD, "shared counted once; derivatives excluded")
	requireDec(t, "111", h.Summary.EligibleLiquidityUSD)
	assert.True(t, h.Summary.TokenizedMarketCapUSD.IsNull(), "no underlying reports a cap: null, not 0")
	assert.True(t, time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC).Equal(h.Summary.ObservedAt), "falls back to generated_at")
	assert.True(t, h.Positions[0].ObservedAt.Equal(h.Summary.ObservedAt), "nil side falls back too")

	_, err = HistoryRows(&model.Export{}, nil, 1)
	require.Error(t, err)
}

func TestDimensionRowsFixture(t *testing.T) {
	res := loadFixture(t)
	settings, err := model.LoadSettings("../../config/settings.json")
	require.NoError(t, err)
	Finalize(res, time.Now(), map[string]RwaInfo{"2": {Website: ptr("https://www.nvidia.com"), Industry: ptr("Semiconductors")}}, nil)
	d, err := DimensionRows(res, 9, settings, map[string]string{"beefy": "Beefy", "aave": "Aave"}, map[string]RwaInfo{"2": {Industry: ptr("Semiconductors")}})
	require.NoError(t, err)

	require.Len(t, d.UnderlyingFacts, 1)
	assert.Equal(t, int64(9), d.UnderlyingFacts[0].LastRunID)
	assert.Equal(t, "Semiconductors", *d.UnderlyingFacts[0].Industry)
	assert.Equal(t, "https://www.nvidia.com", *d.UnderlyingFacts[0].Website)
	require.Len(t, d.Issuers, 4, "3 issuers of the export plus the stub of the derivatives-only issuer")
	assert.Equal(t, "unknown", d.Issuers[0].MetadataStatus)
	stub := d.Issuers[3]
	assert.Equal(t, "695e11f774b54210f3b95dc3", stub.ID)
	assert.Equal(t, "NA (Derivatives)", *stub.Name)
	assert.Equal(t, "NA (Derivatives)", *stub.CMCLabel)
	assert.Equal(t, "unknown", stub.MetadataStatus)
	issuerIDs := map[string]bool{}
	for _, i := range d.Issuers {
		issuerIDs[i.ID] = true
	}
	for _, tr := range d.Tokens {
		if tr.IssuerID != nil {
			assert.True(t, issuerIDs[*tr.IssuerID], "tokens.issuer_id FK: token %d -> %s", tr.ID, *tr.IssuerID)
		}
	}
	assert.Empty(t, d.Warnings)
	assert.Len(t, d.Tokens, 5)
	assert.Len(t, d.Deployments, 13)
	assert.Len(t, d.Positions, 17)
	assert.Equal(t, []platformPair{{"aave", "Aave"}, {"beefy", "Beefy"}}, platformPairs(d))

	var direct, indirect, unconfirmed int
	for _, tp := range d.TokenPositions {
		switch tp.Link {
		case "direct":
			direct++
			assert.Equal(t, 1, *tp.Depth)
		case "indirect":
			indirect++
			assert.Greater(t, *tp.Depth, 1)
		case "unconfirmed":
			unconfirmed++
			assert.Nil(t, tp.Depth)
		default:
			t.Fatalf("unexpected link %q", tp.Link)
		}
		assert.NotNil(t, tp.Path)
	}
	assert.Equal(t, 9+2+1, direct, "NVDAc 9 + NVDAX 2 + NVDAon 1 direct")
	assert.Equal(t, 4, indirect)
	assert.Equal(t, 1, unconfirmed)

	for _, dep := range d.Deployments {
		assert.NotEmpty(t, dep.AddressRaw, dep.CMCNetwork)
		assert.NotEmpty(t, dep.Status)
	}
	slugs := map[string][]string{}
	for _, n := range d.Networks {
		slugs[n.Slug] = n.CMCNames
	}
	assert.Equal(t, []string{"BNB Smart Chain (BEP20)"}, slugs["bsc"])
	assert.NotContains(t, slugs, "")
	assert.NotContains(t, slugs, "solana", "null-mapped CMC networks have no Portals slug")
}

type platformPair struct{ Slug, Name string }

func platformPairs(d *Dimensions) []platformPair {
	var out []platformPair
	for _, p := range d.Platforms {
		out = append(out, platformPair{p.Slug, p.Name})
	}
	return out
}

func TestChanges(t *testing.T) {
	opp := func(key, network, platform, eligibility string) model.Opportunity {
		return model.Opportunity{Key: key, Network: network, Platform: platform, Eligibility: eligibility}
	}
	mk := func(tokenIDs []int64, issuers []string, opps []model.Opportunity, relKeys []string, eligible int) *model.Export {
		e := &model.Export{Opportunities: opps}
		e.Summary.EligibleOpportunitiesCount = eligible
		var rels []model.Relation
		for _, k := range relKeys {
			rels = append(rels, model.Relation{Key: k, Depth: 1})
		}
		for _, id := range tokenIDs {
			e.Tokens = append(e.Tokens, model.Token{ID: id, Kind: "tokenized", Relations: rels})
		}
		for _, i := range issuers {
			e.Issuers = append(e.Issuers, model.Issuer{ID: i})
		}
		return e
	}
	prev := mk([]int64{1, 2}, []string{"a"},
		[]model.Opportunity{opp("p1", "base", "uni", "eligible"), opp("p2", "base", "beefy", "eligible"), opp("p3", "ethereum", "aave", "eligible"), opp("p4", "base", "uni", "stale")},
		[]string{"p1", "p2", "p3", "p4"}, 3)
	cur := mk([]int64{2, 3, 5}, []string{"a", "c", "b"},
		[]model.Opportunity{opp("p1", "base", "uni", "eligible"), opp("p4", "base", "uni", "eligible"), opp("p5", "arbitrum", "uni", "eligible"), opp("p6", "base", "uni", "eligible")},
		[]string{"p1", "p4", "p5", "p6"}, 4)
	row := Changes(prev, cur)
	assert.Equal(t, []int32{3, 5}, row.NewTokens)
	assert.Equal(t, []int32{1}, row.RemovedTokens)
	assert.Equal(t, []string{"b", "c"}, row.NewIssuers)
	assert.Equal(t, 2, row.NewPositionsCount, "p5, p6")
	assert.Equal(t, 2, row.GonePositionsCount, "p2, p3")
	assert.Equal(t, 1, row.EligibleDelta)
	assert.Nil(t, row.PreviousBuildID)

	var details ChangeDetails
	require.NoError(t, json.Unmarshal(row.Details, &details))
	// prev eligible: base 2 (p1, p2), ethereum 1 -> cur: base 3 (p1, p4, p6), arbitrum 1
	assert.Equal(t, map[string]int{"base": 1, "ethereum": -1, "arbitrum": 1}, details.EligibleByNetwork)
	// prev: uni 1, beefy 1, aave 1 -> cur: uni 4 (p1, p4, p5, p6)
	assert.Equal(t, map[string]int{"uni": 3, "beefy": -1, "aave": -1}, details.EligibleByPlatform)

	first := Changes(nil, cur)
	assert.Empty(t, first.NewTokens, "the first build has nothing to compare with")
	assert.Empty(t, first.NewIssuers)
	assert.Zero(t, first.NewPositionsCount)
	assert.Zero(t, first.EligibleDelta)
	assert.JSONEq(t, `{"eligible_by_network":{},"eligible_by_platform":{},"reclassified":[],"first_build":true}`, string(first.Details))

	same := Changes(cur, cur)
	assert.Empty(t, same.NewTokens)
	assert.Zero(t, same.EligibleDelta)
	assert.JSONEq(t, `{"eligible_by_network":{},"eligible_by_platform":{},"reclassified":[]}`, string(same.Details))
}

func TestChangesOverIncludedTokensAndReclassified(t *testing.T) {
	tok := func(id int64, kind string) model.Token { return model.Token{ID: id, Kind: kind} }
	prev := &model.Export{Tokens: []model.Token{tok(1, "tokenized"), tok(2, "derivative"), tok(3, "tokenized"), tok(4, "derivative")}}
	cur := &model.Export{Tokens: []model.Token{tok(1, "tokenized"), tok(2, "tokenized"), tok(3, "derivative"), tok(4, "derivative"), tok(5, "derivative"), tok(6, "wrapper")}}
	row := Changes(prev, cur)
	assert.Equal(t, []int32{6}, row.NewTokens, "excluded newcomers are not new tokens; 2 was reclassified")
	assert.Equal(t, []int32{3}, row.RemovedTokens, "3 left the included set")
	var d ChangeDetails
	require.NoError(t, json.Unmarshal(row.Details, &d))
	assert.Equal(t, []int32{2}, d.Reclassified)
	assert.False(t, d.FirstBuild)
}

func TestHistoryAndDimensionsDropDuplicateTokenIDs(t *testing.T) {
	res := dupResult(t, "2026-10-01T00:00:00Z", "2026-10-02T00:00:00Z")
	settings := testSettings()
	require.Len(t, res.Export.Tokens, 2, "the export keeps both copies, as 1.x")
	d, err := DimensionRows(res, 1, settings, nil, nil)
	require.NoError(t, err)
	require.Len(t, d.Tokens, 1)
	assert.Equal(t, "NEW", d.Tokens[0].UnderlyingID, "the newest snapshot wins")
	require.Len(t, d.Warnings, 1)
	assert.Contains(t, d.Warnings[0], "OLD")
	for _, tp := range d.TokenPositions {
		assert.Equal(t, int64(7), tp.TokenID)
	}
	h, err := HistoryRows(&res.Export, &res.Side, 1)
	require.NoError(t, err)
	require.Len(t, h.Tokens, 1)
	assert.Equal(t, int64(7), h.Tokens[0].TokenID)
	assert.Len(t, h.Warnings, 1)

	// equal fetched_at: the first in the export order wins
	res = dupResult(t, "2026-10-01T00:00:00Z", "2026-10-01T00:00:00Z")
	d, err = DimensionRows(res, 1, settings, nil, nil)
	require.NoError(t, err)
	require.Len(t, d.Tokens, 1)
	assert.Equal(t, res.Export.Tokens[0].UnderlyingID, d.Tokens[0].UnderlyingID)
}

func TestDimensionRowsLogoAndNetworks(t *testing.T) {
	res := loadFixture(t)
	settings := testSettings()
	res.Side.PortalsNetworks = []string{"base", "zksync"}
	d, err := DimensionRows(res, 1, settings, nil, map[string]RwaInfo{"2": {Logo: ptr("https://cmc.example/nvda.png")}})
	require.NoError(t, err)
	require.NotNil(t, d.UnderlyingFacts[0].CMCLogoURL)
	assert.Equal(t, "https://cmc.example/nvda.png", *d.UnderlyingFacts[0].CMCLogoURL)
	got := map[string][]string{}
	for _, n := range d.Networks {
		got[n.Slug] = n.CMCNames
	}
	assert.Equal(t, map[string][]string{"base": {"Base"}, "ethereum": {"Ethereum"}, "zksync": {}}, got, "chain_mapping slugs plus the Portals networks")
}
