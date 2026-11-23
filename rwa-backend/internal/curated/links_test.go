package curated

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/model"
)

// The tests mirror rwa-data-pipeline/tests/test_links.py.

var (
	now      = time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	verifd   = m{"at": "2026-11-13", "sample": "robinhood:0xsample"}
	poolID   = "0x" + strings.Repeat("ab", 32)
	vault    = "0x" + strings.Repeat("cd", 20)
	backpack = "issuer-backpack"
)

func linksConfig() m {
	return m{
		"schema_version": 1, "deals_max_age_days": 14,
		"platforms": m{
			"uniswap-v4": m{"home": "https://app.uniswap.org", "hosts": []any{"app.uniswap.org"}, "routes": []any{m{
				"url": "https://app.uniswap.org/explore/pools/{network}/{address}",
				"networks": m{
					"robinhood": m{"slug": "robinhood", "verified": verifd},
					"bsc":       m{"slug": "bnb", "verified": verifd},
					"base":      m{"slug": "base"}, // listed but never opened in a browser: must not be used
				}}}},
			"morpho": m{"home": "https://app.morpho.org", "hosts": []any{"app.morpho.org"}, "routes": []any{
				m{"when": m{"position_kind": "pool_id"}, "url": "https://app.morpho.org/{network}/market/{address}", "networks": m{"base": m{"slug": "base", "verified": verifd}}},
				m{"when": m{"position_kind": "contract"}, "url": "https://app.morpho.org/{network}/vault/{address}", "networks": m{"base": m{"slug": "base", "verified": verifd}}},
			}},
			"beefy-clm": m{"home": "https://app.beefy.com", "hosts": []any{"app.beefy.com"}, "routes": []any{m{
				"url": "https://app.beefy.com/vault/{meta.beefyClmId}", "networks": m{"robinhood": m{"slug": "robinhood", "verified": verifd}}}}},
			"venus": m{"home": "https://app.venus.io", "hosts": []any{"app.venus.io"}, "deal": "venus"},
		},
		"issuers": m{backpack: m{"label": "Backpack", "app": "https://backpack.exchange", "hosts": []any{"backpack.exchange"}, "deal": "backpack"}},
		"swap": m{
			"base": []any{m{"platforms": []any{"uniswap-v4", "uniswap-v3"}, "label": "Uniswap",
				"url": "https://app.uniswap.org/swap?chain=base&outputCurrency={address}", "hosts": []any{"app.uniswap.org"}, "verified": verifd}},
			"arbitrum": []any{m{"platforms": []any{"uniswap-v4", "uniswap-v3"}, "label": "Uniswap", // no verified: must not be used
				"url": "https://app.uniswap.org/swap?chain=arbitrum&outputCurrency={address}", "hosts": []any{"app.uniswap.org"}}},
		},
	}
}

func parseLinks(t *testing.T, cfg m) *Links {
	t.Helper()
	l, err := ParseLinks("links.json", mustJSON(cfg))
	require.NoError(t, err)
	return l
}

func dealsCache(fetchedAt string) *DealsCache {
	return ParseDealsCache(mustJSON(m{"fetched_at": fetchedAt, "deals": m{
		"backpack": m{"title": "Backpack", "link": "https://backpack.exchange/refer/cp0x", "bonus": "+20% fee return", "page_url": "https://cp0x.com/deals/backpack"},
		"venus":    m{"title": "Venus", "link": "https://app.venus.io/?ref=cp0x", "bonus": "boost", "page_url": "https://cp0x.com/deals/venus"},
	}}))
}

func opp(key, platform, kind, category, eligibility string, components ...string) model.Opportunity {
	network, address, _ := strings.Cut(key, ":")
	o := model.Opportunity{Key: key, Network: network, Platform: platform, Address: address, PositionKind: kind, Category: category, Eligibility: eligibility}
	for _, c := range components {
		o.Components = append(o.Components, model.Component{Key: c})
	}
	return o
}

func contractPool(key, platform string, components ...string) model.Opportunity {
	return opp(key, platform, "contract", "dex_lp", "eligible", components...)
}

func attach(t *testing.T, exp *model.Export, links *Links, deals *DealsCache, meta map[string]map[string]string) []string {
	t.Helper()
	return AttachLinks(exp, links, deals, meta, now)
}

func TestPositionRouteForANetworkOpenedInABrowser(t *testing.T) {
	exp := &model.Export{Opportunities: []model.Opportunity{
		opp("robinhood:"+poolID, "uniswap-v4", "pool_id", "dex_lp", "eligible"),
		opp("bsc:"+poolID, "uniswap-v4", "pool_id", "dex_lp", "eligible"),
	}}
	assert.Empty(t, attach(t, exp, parseLinks(t, linksConfig()), dealsCache("2026-11-12T12:00:00Z"), nil))
	assert.Equal(t, &model.PositionLink{URL: "https://app.uniswap.org/explore/pools/robinhood/" + poolID, Kind: "position"}, exp.Opportunities[0].Link)
	// the protocol's own name of the network goes into the URL, not ours
	assert.Equal(t, &model.PositionLink{URL: "https://app.uniswap.org/explore/pools/bnb/" + poolID, Kind: "position"}, exp.Opportunities[1].Link)
	assert.Nil(t, exp.Opportunities[0].Referral)
}

func TestUnverifiedOrUnlistedNetworkGetsTheHomeLinkOnly(t *testing.T) {
	exp := &model.Export{Opportunities: []model.Opportunity{
		opp("base:"+poolID, "uniswap-v4", "pool_id", "dex_lp", "eligible"), opp("arbitrum:"+poolID, "uniswap-v4", "pool_id", "dex_lp", "eligible")}}
	assert.Empty(t, attach(t, exp, parseLinks(t, linksConfig()), nil, nil))
	for _, o := range exp.Opportunities {
		assert.Equal(t, &model.PositionLink{URL: "https://app.uniswap.org", Kind: "home"}, o.Link)
	}
}

func TestRouteIsChosenByPositionKind(t *testing.T) {
	exp := &model.Export{Opportunities: []model.Opportunity{
		opp("base:"+poolID, "morpho", "pool_id", "lending", "eligible"), opp("base:"+vault, "morpho", "contract", "lending", "eligible")}}
	assert.Empty(t, attach(t, exp, parseLinks(t, linksConfig()), nil, nil))
	assert.Equal(t, "https://app.morpho.org/base/market/"+poolID, exp.Opportunities[0].Link.URL)
	assert.Equal(t, "https://app.morpho.org/base/vault/"+vault, exp.Opportunities[1].Link.URL)
}

func TestMetaPlaceholderIsReadFromTheRawPortalsRecord(t *testing.T) {
	key := "robinhood:" + vault
	exp := &model.Export{Opportunities: []model.Opportunity{contractPool(key, "beefy-clm")}}
	attach(t, exp, parseLinks(t, linksConfig()), nil, map[string]map[string]string{key: {"beefyClmId": "uniswap-cow-robinhood-usdg-gld"}})
	assert.Equal(t, &model.PositionLink{URL: "https://app.beefy.com/vault/uniswap-cow-robinhood-usdg-gld", Kind: "position"}, exp.Opportunities[0].Link)

	// no raw record, or a record without the field: never a half-built URL
	for _, meta := range []map[string]map[string]string{nil, {key: {}}, {key: {"other": "x"}}} {
		exp = &model.Export{Opportunities: []model.Opportunity{contractPool(key, "beefy-clm")}}
		attach(t, exp, parseLinks(t, linksConfig()), nil, meta)
		assert.Equal(t, &model.PositionLink{URL: "https://app.beefy.com", Kind: "home"}, exp.Opportunities[0].Link)
	}
}

// The metadata of a snapshot record reaches the links through Snapshot parsing: only string values count.
func TestSnapshotMetadataKeepsStringFields(t *testing.T) {
	var md model.PortalsMetadata
	require.NoError(t, json.Unmarshal([]byte(`{"tags":["#clp"],"beefyClmId":"abc","n":5,"x":null}`), &md))
	assert.Equal(t, []string{"#clp"}, md.Tags)
	assert.Equal(t, map[string]string{"beefyClmId": "abc"}, md.Strings)
}

func TestPlatformWithoutConfigHasNoLinkAndIsNotAWarning(t *testing.T) {
	exp := &model.Export{Opportunities: []model.Opportunity{contractPool("base:"+vault, "superform")}}
	assert.Empty(t, attach(t, exp, parseLinks(t, linksConfig()), nil, nil))
	assert.Nil(t, exp.Opportunities[0].Link)
	assert.Nil(t, exp.Opportunities[0].Referral)
}

func TestRouteOutsideDeclaredHostsIsDroppedWithAWarning(t *testing.T) {
	cfg := linksConfig()
	route := cfg["platforms"].(m)["uniswap-v4"].(m)["routes"].([]any)[0].(m)
	route["url"] = "https://uniswap.example.org/pools/{network}/{address}"
	exp := &model.Export{Opportunities: []model.Opportunity{opp("robinhood:"+poolID, "uniswap-v4", "pool_id", "dex_lp", "eligible")}}
	warnings := attach(t, exp, parseLinks(t, cfg), nil, nil)
	assert.Equal(t, &model.PositionLink{URL: "https://app.uniswap.org", Kind: "home"}, exp.Opportunities[0].Link)
	assert.Equal(t, []string{"platform uniswap-v4: invalid route URL"}, warnings)
}

func TestHomeThatIsNotACleanHTTPSURLOnADeclaredHostGivesNoLink(t *testing.T) {
	for _, home := range []string{"http://app.uniswap.org", "javascript:alert(1)", "https://app.uniswap.org/ x", "https://evil.example.org", "//app.uniswap.org", ""} {
		cfg := linksConfig()
		entry := cfg["platforms"].(m)["uniswap-v4"].(m)
		entry["home"] = home
		entry["routes"] = []any{}
		exp := &model.Export{Opportunities: []model.Opportunity{opp("robinhood:"+poolID, "uniswap-v4", "pool_id", "dex_lp", "eligible")}}
		warnings := attach(t, exp, parseLinks(t, cfg), nil, nil)
		assert.Nil(t, exp.Opportunities[0].Link, home)
		assert.Equal(t, []string{"platform uniswap-v4: invalid home URL"}, warnings, home)
	}
}

func TestSubstitutedValuesCannotReshapeTheURL(t *testing.T) {
	for _, address := range []string{`0xabc" onclick="x`, "0xabc def", "0xabc\n", "0xabc<script>", "0xabc/../../evil", "0xabc?next=https://evil.example.org", "0xabc#frag"} {
		o := opp("robinhood:"+address, "uniswap-v4", "pool_id", "dex_lp", "eligible")
		exp := &model.Export{Opportunities: []model.Opportunity{o}}
		attach(t, exp, parseLinks(t, linksConfig()), nil, nil)
		assert.Equal(t, &model.PositionLink{URL: "https://app.uniswap.org", Kind: "home"}, exp.Opportunities[0].Link, address)
	}
}

func TestReferralIsASeparateObjectAndTheDirectLinkStays(t *testing.T) {
	exp := &model.Export{Opportunities: []model.Opportunity{contractPool("bsc:"+vault, "venus")}}
	assert.Empty(t, attach(t, exp, parseLinks(t, linksConfig()), dealsCache("2026-11-12T12:00:00Z"), nil))
	o := exp.Opportunities[0]
	assert.Equal(t, &model.PositionLink{URL: "https://app.venus.io", Kind: "home"}, o.Link)
	page := "https://cp0x.com/deals/venus"
	assert.Equal(t, &model.Referral{URL: "https://app.venus.io/?ref=cp0x", Deal: "venus", Bonus: "boost", PageURL: &page, AsOf: "2026-11-12T12:00:00Z"}, o.Referral)
}

func venusReferral(t *testing.T, cache *DealsCache) (model.Opportunity, []string) {
	t.Helper()
	exp := &model.Export{Opportunities: []model.Opportunity{contractPool("bsc:"+vault, "venus")}}
	warnings := attach(t, exp, parseLinks(t, linksConfig()), cache, nil)
	return exp.Opportunities[0], warnings
}

func TestReferralIsNotPublishedWithoutAFreshMatchingDeal(t *testing.T) {
	deal := func(mutate func(deals m)) *DealsCache {
		d := m{"venus": m{"title": "Venus", "link": "https://app.venus.io/?ref=cp0x", "bonus": "boost", "page_url": "https://cp0x.com/deals/venus"}}
		if mutate != nil {
			mutate(d)
		}
		return ParseDealsCache(mustJSON(m{"fetched_at": "2026-11-12T12:00:00Z", "deals": d}))
	}
	at := func(fetched string) *DealsCache {
		c := deal(nil)
		c.FetchedAt = fetched
		return c
	}
	// 14 days exactly at `now` is still valid; one second more is not.
	o, warnings := venusReferral(t, at("2026-10-30T12:00:00Z"))
	assert.NotNil(t, o.Referral)
	assert.Empty(t, warnings)

	for name, cache := range map[string]*DealsCache{
		"stale by a second":   at("2026-10-30T11:59:59Z"),
		"missing deal":        deal(func(d m) { delete(d, "venus") }),
		"foreign host":        deal(func(d m) { d["venus"].(m)["link"] = "https://venus.example.org/?ref=cp0x" }),
		"plain http":          deal(func(d m) { d["venus"].(m)["link"] = "http://app.venus.io/?ref=cp0x" }),
		"no scheme":           deal(func(d m) { d["venus"].(m)["link"] = "app.venus.io/?ref=cp0x" }),
		"no cache":            nil,
		"not a date":          ParseDealsCache([]byte(`{"fetched_at":"not a date","deals":{}}`)),
		"naive timestamp":     at("2026-11-12T12:00:00"),
		"deal not an object":  ParseDealsCache([]byte(`{"fetched_at":"2026-11-12T12:00:00Z","deals":{"venus":"x"}}`)),
		"deals not an object": ParseDealsCache([]byte(`{"fetched_at":"2026-11-12T12:00:00Z","deals":[]}`)),
	} {
		o, warnings := venusReferral(t, cache)
		assert.Nil(t, o.Referral, name)
		assert.Equal(t, &model.PositionLink{URL: "https://app.venus.io", Kind: "home"}, o.Link, "%s: the direct link does not depend on the deal", name)
		assert.Equal(t, []string{"platform venus: deals referral venus unavailable, stale or invalid"}, warnings, name)
	}
}

// The -1h tolerance: a cache from the future is fresh only within an hour of clock skew.
func TestADealsCacheFromTheFutureIsNotFresh(t *testing.T) {
	for fetched, wantReferral := range map[string]bool{
		"2026-11-13T12:59:59Z": true, "2026-11-13T13:00:00Z": true, "2026-11-13T13:00:01Z": false, "2026-11-14T12:00:00Z": false,
	} {
		cache := dealsCache("2026-11-12T12:00:00Z")
		cache.FetchedAt = fetched
		o, _ := venusReferral(t, cache)
		assert.Equal(t, wantReferral, o.Referral != nil, fetched)
	}
}

func TestDealsMaxAgeFromConfig(t *testing.T) {
	cfg := linksConfig()
	cfg["deals_max_age_days"] = 1
	exp := &model.Export{Opportunities: []model.Opportunity{contractPool("bsc:"+vault, "venus")}}
	attach(t, exp, parseLinks(t, cfg), dealsCache("2026-11-12T12:00:00Z"), nil)
	assert.NotNil(t, exp.Opportunities[0].Referral, "exactly one day old")
	assert.Equal(t, 1.0, *exp.Summary.LinksDealsMaxAgeDays)

	delete(cfg, "deals_max_age_days")
	exp = &model.Export{}
	attach(t, exp, parseLinks(t, cfg), nil, nil)
	assert.Equal(t, 14.0, *exp.Summary.LinksDealsMaxAgeDays)
}

func TestDealPageMustBeTheCP0XPageOfThatDeal(t *testing.T) {
	for page, want := range map[string]bool{
		"https://cp0x.com/deals/venus": true, "https://cp0x.com/deals/backpack": false, "https://evil.example.org/deals/venus": false,
		"http://cp0x.com/deals/venus": false, "https://cp0x.com/deals/venus?x=1": false, "": false,
	} {
		cache := ParseDealsCache(mustJSON(m{"fetched_at": "2026-11-12T12:00:00Z", "deals": m{
			"venus": m{"link": "https://app.venus.io/?ref=cp0x", "bonus": "boost", "page_url": page}}}))
		o, _ := venusReferral(t, cache)
		require.NotNil(t, o.Referral, page)
		assert.Equal(t, want, o.Referral.PageURL != nil, page)
	}
}

func TestOneWarningPerPlatformNotPerPosition(t *testing.T) {
	var opps []model.Opportunity
	for n := 1; n <= 5; n++ {
		opps = append(opps, contractPool(fmt.Sprintf("bsc:0x%040x", n), "venus"))
	}
	assert.Len(t, attach(t, &model.Export{Opportunities: opps}, parseLinks(t, linksConfig()), nil, nil), 1)
}

func TestIssuerAppAndReferral(t *testing.T) {
	exp := &model.Export{Issuers: []model.Issuer{{ID: backpack, Name: str("Backpack")}, {ID: "issuer-other", Name: str("Other")}}}
	assert.Empty(t, attach(t, exp, parseLinks(t, linksConfig()), dealsCache("2026-11-12T12:00:00Z"), nil))
	bp, other := exp.Issuers[0], exp.Issuers[1]
	assert.Equal(t, &model.IssuerApp{URL: "https://backpack.exchange", Label: str("Backpack")}, bp.App)
	require.NotNil(t, bp.Referral)
	assert.Equal(t, "https://backpack.exchange/refer/cp0x", bp.Referral.URL)
	assert.Equal(t, "backpack", bp.Referral.Deal)
	assert.Nil(t, other.App)
	assert.Nil(t, other.Referral)

	// the app URL must sit on a declared host
	cfg := linksConfig()
	cfg["issuers"].(m)[backpack].(m)["app"] = "https://evil.example.org"
	exp = &model.Export{Issuers: []model.Issuer{{ID: backpack}}}
	assert.Equal(t, []string{"issuer " + backpack + ": invalid app URL"}, attach(t, exp, parseLinks(t, cfg), dealsCache("2026-11-12T12:00:00Z"), nil))
	assert.Nil(t, exp.Issuers[0].App)
}

func token(id int64, deployments [][2]string, depth int, relations ...string) model.Token {
	tk := model.Token{ID: id}
	for _, d := range deployments {
		dep := model.Deployment{Address: d[1], Status: "indexed"}
		if d[0] != "" {
			network, key := d[0], d[0]+":"+d[1]
			dep.Network, dep.Key = &network, &key
		}
		tk.Deployments = append(tk.Deployments, dep)
	}
	for _, r := range relations {
		tk.Relations = append(tk.Relations, model.Relation{Key: r, Depth: depth})
	}
	return tk
}

func TestSwapLinkOnlyWhereTheTokenHasAnEligibleDirectPoolOfThatDEX(t *testing.T) {
	basePool, thinPool, vaultKey, cakePool := fmt.Sprintf("base:0x%040x", 1), fmt.Sprintf("base:0x%040x", 2), fmt.Sprintf("base:0x%040x", 4), fmt.Sprintf("base:0x%040x", 5)
	arbPool := fmt.Sprintf("arbitrum:0x%040x", 3)
	here, there := "base:"+vault, "arbitrum:"+vault
	thin := contractPool(thinPool, "uniswap-v3", here)
	thin.Eligibility = "low_liquidity"
	vaultOpp := opp(vaultKey, "beefy", "contract", "vault", "eligible", here)
	exp := &model.Export{
		Opportunities: []model.Opportunity{
			contractPool(basePool, "uniswap-v3", here), thin, contractPool(arbPool, "uniswap-v3", there), vaultOpp, contractPool(cakePool, "pancakeswap-v3", here)},
		Tokens: []model.Token{
			token(1, [][2]string{{"base", vault}, {"arbitrum", vault}, {"bsc", vault}, {"", "SoLanaAddr111"}}, 1, basePool, arbPool),
			token(2, [][2]string{{"base", vault}}, 1, thinPool), // the only pool is below the liquidity bar
			token(3, [][2]string{{"base", vault}}, 1, vaultKey), // a vault is not a place to swap
			token(4, [][2]string{{"base", vault}}, 1, cakePool), // pool of another DEX
			token(5, [][2]string{{"base", vault}}, 2, basePool), // reached only through another position
			token(6, nil, 1),
		},
	}
	assert.Empty(t, attach(t, exp, parseLinks(t, linksConfig()), nil, nil))
	assert.Equal(t, []model.SwapLink{{Kind: "swap", Label: str("Uniswap"), URL: "https://app.uniswap.org/swap?chain=base&outputCurrency=" + vault, Network: "base"}}, exp.Tokens[0].Links)
	for _, tk := range exp.Tokens[1:] {
		assert.Equal(t, []model.SwapLink{}, tk.Links, tk.ID)
	}
}

func TestSwapLinkBelongsToTheContractThatIsActuallyInThePool(t *testing.T) {
	pool := fmt.Sprintf("base:0x%040x", 9)
	first, second := "0x"+strings.Repeat("11", 20), "0x"+strings.Repeat("22", 20)
	exp := &model.Export{
		Opportunities: []model.Opportunity{contractPool(pool, "uniswap-v3", "base:"+first, "base:0x"+strings.Repeat("ee", 20))},
		Tokens:        []model.Token{token(1, [][2]string{{"base", first}, {"base", second}}, 1, pool)},
	}
	assert.Empty(t, attach(t, exp, parseLinks(t, linksConfig()), nil, nil))
	require.Len(t, exp.Tokens[0].Links, 1)
	assert.Equal(t, "https://app.uniswap.org/swap?chain=base&outputCurrency="+first, exp.Tokens[0].Links[0].URL)
}

func TestSummaryCarriesTheDealsTimestampAndTheWarningsCount(t *testing.T) {
	exp := &model.Export{Opportunities: []model.Opportunity{contractPool("bsc:"+vault, "venus")}}
	attach(t, exp, parseLinks(t, linksConfig()), dealsCache("2026-11-12T12:00:00Z"), nil)
	assert.Equal(t, "2026-11-12T12:00:00Z", *exp.Summary.LinksDealsFetchedAt)
	assert.Zero(t, exp.Summary.LinksWarningsCount)

	// only a UTC-offset timestamp is published
	for _, fetched := range []string{"not a date", "2026-11-12T12:00:00", ""} {
		exp = &model.Export{}
		cache := dealsCache("2026-11-12T12:00:00Z")
		cache.FetchedAt = fetched
		attach(t, exp, parseLinks(t, linksConfig()), cache, nil)
		assert.Nil(t, exp.Summary.LinksDealsFetchedAt, fetched)
	}
	exp = &model.Export{Opportunities: []model.Opportunity{contractPool("bsc:"+vault, "venus")}}
	attach(t, exp, parseLinks(t, linksConfig()), nil, nil)
	assert.Equal(t, 1, exp.Summary.LinksWarningsCount)
	assert.Nil(t, exp.Summary.LinksDealsFetchedAt)
}

func TestWithoutALinksConfigEveryFieldIsPresentAndEmpty(t *testing.T) {
	exp := &model.Export{
		Opportunities: []model.Opportunity{contractPool("bsc:"+vault, "venus")},
		Issuers:       []model.Issuer{{ID: backpack}},
		Tokens:        []model.Token{token(1, nil, 1)},
	}
	assert.Empty(t, AttachLinks(exp, nil, dealsCache("2026-11-12T12:00:00Z"), nil, now))
	assert.Nil(t, exp.Opportunities[0].Link)
	assert.Nil(t, exp.Opportunities[0].Referral)
	assert.Nil(t, exp.Issuers[0].App)
	assert.Equal(t, []model.SwapLink{}, exp.Tokens[0].Links)
	assert.Nil(t, exp.Summary.LinksDealsMaxAgeDays)
	assert.Equal(t, "2026-11-12T12:00:00Z", *exp.Summary.LinksDealsFetchedAt)
}

func TestFirstApplicableRouteWins(t *testing.T) {
	cfg := linksConfig()
	entry := cfg["platforms"].(m)["morpho"].(m)
	entry["routes"] = []any{
		m{"when": m{"position_kind": "pool_id"}, "url": "https://app.morpho.org/{network}/first/{address}", "networks": m{"base": m{"slug": "base", "verified": verifd}}},
		m{"url": "https://app.morpho.org/{network}/second/{address}", "networks": m{"base": m{"slug": "base", "verified": verifd}}},
	}
	exp := &model.Export{Opportunities: []model.Opportunity{opp("base:"+poolID, "morpho", "pool_id", "lending", "eligible"), opp("base:"+vault, "morpho", "contract", "lending", "eligible")}}
	attach(t, exp, parseLinks(t, cfg), nil, nil)
	assert.Equal(t, "https://app.morpho.org/base/first/"+poolID, exp.Opportunities[0].Link.URL)
	assert.Equal(t, "https://app.morpho.org/base/second/"+vault, exp.Opportunities[1].Link.URL)
}

func TestHostsThatAreNotAListOfHostNamesAllowNothing(t *testing.T) {
	for _, hosts := range []any{"app.venus.io", nil, 5, m{"app.venus.io": true}} {
		entry := m{"home": "https://app.venus.io", "hosts": hosts}
		assert.False(t, hostAllowed("https://app.venus.io", mustObj(t, entry)), hosts)
	}
	assert.True(t, hostAllowed("https://APP.venus.io/x", mustObj(t, m{"hosts": []any{"app.venus.io"}})), "the host of the URL is compared lower-case")
	assert.False(t, hostAllowed("https://app.venus.io", mustObj(t, m{"hosts": []any{"APP.venus.io"}})), "config hosts are compared as written")
}

func mustObj(t *testing.T, v m) *Obj {
	t.Helper()
	parsed, err := parseJSON(mustJSON(v))
	require.NoError(t, err)
	return parsed.(*Obj)
}

func TestLoadLinksRejectsEntriesOfTheWrongShape(t *testing.T) {
	good := linksConfig()
	require.NotNil(t, parseLinks(t, good))
	for name, mutate := range map[string]func(c m){
		"schema":              func(c m) { c["schema_version"] = 2 },
		"platforms":           func(c m) { c["platforms"] = []any{} },
		"issuers section":     func(c m) { c["issuers"] = []any{} },
		"swap section":        func(c m) { c["swap"] = "x" },
		"negative age":        func(c m) { c["deals_max_age_days"] = -1 },
		"bool age":            func(c m) { c["deals_max_age_days"] = true },
		"string age":          func(c m) { c["deals_max_age_days"] = "14" },
		"entry not an object": func(c m) { c["platforms"].(m)["venus"] = "x" },
		"deal not a string":   func(c m) { c["platforms"].(m)["venus"].(m)["deal"] = 5 },
		"blank deal":          func(c m) { c["platforms"].(m)["venus"].(m)["deal"] = " " },
		"hosts not a list":    func(c m) { c["platforms"].(m)["venus"].(m)["hosts"] = "x" },
		"empty host":          func(c m) { c["platforms"].(m)["venus"].(m)["hosts"] = []any{""} },
		"routes not a list":   func(c m) { c["platforms"].(m)["venus"].(m)["routes"] = m{} },
		"route not an object": func(c m) { c["platforms"].(m)["venus"].(m)["routes"] = []any{"x"} },
		"when not an object":  func(c m) { c["platforms"].(m)["venus"].(m)["routes"] = []any{m{"when": []any{}}} },
		"networks not object": func(c m) { c["platforms"].(m)["venus"].(m)["routes"] = []any{m{"networks": []any{}}} },
		"verified without sample": func(c m) {
			c["platforms"].(m)["uniswap-v4"].(m)["routes"].([]any)[0].(m)["networks"].(m)["bsc"].(m)["verified"] = m{"at": "2026-11-13"}
		},
		"verified bad date": func(c m) {
			c["platforms"].(m)["uniswap-v4"].(m)["routes"].([]any)[0].(m)["networks"].(m)["bsc"].(m)["verified"] = m{"at": "2026-13-13", "sample": "x"}
		},
		"swap not a list":       func(c m) { c["swap"].(m)["base"] = m{} },
		"swap platforms":        func(c m) { c["swap"].(m)["base"].([]any)[0].(m)["platforms"] = "uniswap" },
		"swap platform nonstr":  func(c m) { c["swap"].(m)["base"].([]any)[0].(m)["platforms"] = []any{1} },
		"swap unverified shape": func(c m) { c["swap"].(m)["base"].([]any)[0].(m)["verified"] = "yes" },
	} {
		cfg := linksConfig()
		mutate(cfg)
		_, err := ParseLinks("links.json", mustJSON(cfg))
		var ce *ConfigError
		require.ErrorAs(t, err, &ce, name)
		assert.True(t, strings.HasPrefix(ce.Msg, "links.json: "), "%s: %s", name, ce.Msg)
	}
	_, err := ParseLinks("links.json", []byte("{nope"))
	assert.ErrorContains(t, err, "links.json: invalid links config")

	cfg := linksConfig()
	cfg["platforms"].(m)["venus"].(m)["deal"] = 5
	_, err = ParseLinks("links.json", mustJSON(cfg))
	assert.EqualError(t, err, "links.json: platforms.venus.deal: expected a nonempty string")

	// A missing file disables links.
	l, err := LoadLinks(t.TempDir() + "/links.json")
	require.NoError(t, err)
	assert.Nil(t, l)
}

func TestOnlyACompleteVerificationRecordEnablesARoute(t *testing.T) {
	for name, v := range map[string]any{
		"complete": verifd, "no sample": m{"at": "2026-11-13"}, "blank sample": m{"at": "2026-11-13", "sample": " "},
		"bad date": m{"at": "13/11/2026", "sample": "x"}, "no date": m{"sample": "x"},
	} {
		cfg := linksConfig()
		cfg["platforms"].(m)["uniswap-v4"].(m)["routes"].([]any)[0].(m)["networks"].(m)["robinhood"].(m)["verified"] = v
		_, err := ParseLinks("links.json", mustJSON(cfg))
		if name == "complete" {
			assert.NoError(t, err, name)
		} else {
			assert.Error(t, err, name)
		}
	}
}

func TestADealThatIsNotASlugIsRefusedAndNeverCrashesTheBuild(t *testing.T) {
	// Config validation rejects non-string deals; AttachLinks stays safe for a config built without validation.
	cfg := linksConfig()
	cfg["platforms"].(m)["venus"].(m)["deal"] = " "
	_, err := ParseLinks("links.json", mustJSON(cfg))
	require.Error(t, err)
}

func TestReferencedDeals(t *testing.T) {
	assert.Equal(t, map[string]bool{"venus": true, "backpack": true}, parseLinks(t, linksConfig()).ReferencedDeals())
	var none *Links
	assert.Empty(t, none.ReferencedDeals())
}

func TestShippedLinksConfigLoads(t *testing.T) {
	l, err := LoadLinks("../../config/links.json")
	require.NoError(t, err)
	require.NotNil(t, l)
	assert.Equal(t, map[string]bool{"backpack": true, "hyperliquid": true}, l.ReferencedDeals())
	assert.Equal(t, 14.0, l.DealsMaxAgeDays)
}

func TestIssuerWebsites(t *testing.T) {
	good := []byte(`{"schema_version":1,"issuers":{"a":{"website":"https://example.org/x"}}}`)
	got, err := ParseIssuerWebsites("issuers.json", good)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"a": "https://example.org/x"}, got)
	for _, bad := range []string{
		`[]`, `{"schema_version":2,"issuers":{}}`, `{"schema_version":1}`, `{"schema_version":1,"issuers":[]}`,
		`{"schema_version":1,"issuers":{"a":{"website":"http://example.org"}}}`,
		`{"schema_version":1,"issuers":{"a":{"website":"https://example.org","logo":"x"}}}`,
		`{"schema_version":1,"issuers":{"a":"https://example.org"}}`,
		`{"schema_version":1,"issuers":{"a":{}}}`,
		`{"schema_version":1,"issuers":{"a":{"website":"https://user:pw@example.org"}}}`,
	} {
		_, err := ParseIssuerWebsites("issuers.json", []byte(bad))
		var ce *ConfigError
		require.ErrorAs(t, err, &ce, bad)
	}
	_, err = ParseIssuerWebsites("issuers.json", []byte(`{"schema_version":1,"issuers":{"a":{}}}`))
	assert.EqualError(t, err, "issuers.json: issuers.a: expected an object containing only a clean HTTPS website")
	_, err = ParseIssuerWebsites("issuers.json", []byte(`[]`))
	assert.EqualError(t, err, "issuers.json: expected an object with schema_version 1 and an issuers object")

	// The override fills only a gap: a CMC website wins.
	issuers := []model.Issuer{{ID: "a"}, {ID: "b", Website: str("https://cmc.example.org")}, {ID: "c", Website: str("")}}
	ApplyIssuerWebsites(issuers, map[string]string{"a": "https://o.example.org", "b": "https://o.example.org"})
	assert.Equal(t, "https://o.example.org", *issuers[0].Website)
	assert.Equal(t, "https://cmc.example.org", *issuers[1].Website)
	assert.Nil(t, issuers[2].Website)

	sites, err := LoadIssuerWebsites("../../config/issuers.json")
	require.NoError(t, err)
	assert.Contains(t, sites, "6a2aed5097c45356b1a5f710")
}

func TestCleanHTTPSURL(t *testing.T) {
	for u, want := range map[string]bool{
		"https://example.org": true, "https://example.org:8443/x?y=1": true, "http://example.org": false, "https://": false,
		"https://user@example.org": false, "https://@example.org": false, "https://example.org:99999": false, "https://example.org:abc": false,
		"https://exa mple.org": false, "https://example.org/\x00": false, "https://example.org/`": false, "https://example.org/{x}": false,
		"https://example.org/\\x": false, "https://example.org/<": false, "": false,
	} {
		assert.Equal(t, want, CleanHTTPSURL(u), u)
	}
	assert.False(t, CleanHTTPSURL(5))
	assert.False(t, CleanHTTPSURL(nil))
}

func TestApplyOrderAndWarnings(t *testing.T) {
	exp := exportOf()
	exp.Opportunities = []model.Opportunity{contractPool("bsc:"+vault, "venus")}
	rights, err := ParseRights("r", config(nil, profile("acme-line", []int{1}, m{"reviewed_at": "2026-01-01"})))
	require.NoError(t, err)
	cfg := &Config{Links: parseLinks(t, linksConfig()), Rights: rights, IssuerWebsites: map[string]string{issuerA: "https://acme.example.org"}}
	warnings := Apply(exp, cfg, nil, nil, now)
	assert.Equal(t, []string{
		"platform venus: deals referral venus unavailable, stale or invalid",
		"rights profile acme-line: review is stale (older than 180 days)",
	}, warnings, "links warnings first, then rights")
	assert.Equal(t, "https://acme.example.org", *exp.Issuers[0].Website)
	assert.Equal(t, 1, exp.Summary.LinksWarningsCount)
	assert.Equal(t, 1, exp.Summary.RightsWarningsCount)

	// nil config: the fields exist and are empty
	exp = exportOf()
	assert.Empty(t, Apply(exp, nil, nil, nil, now))
	assert.NotNil(t, exp.RightsProfiles)
}
