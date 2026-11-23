package curated

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/model"
)

// More of rwa-data-pipeline/tests/test_links.py: review round tests on the attach logic itself.

// unvalidated wraps a config without the load-time checks, as Python's attach_links reads a plain dict: the attach
// logic must hold on its own, not only through ParseLinks.
func unvalidated(t *testing.T, cfg m) *Links {
	t.Helper()
	return &Links{root: mustObj(t, cfg), DealsMaxAgeDays: 14}
}

func TestSummaryTimestampIsPublishedOnlyWhenItIsAUTCTimestamp(t *testing.T) {
	for _, fetched := range []any{"not a date", "2026-11-12", 5.0, nil, "2026-11-12T12:00:00"} {
		cache := dealsCache("2026-11-12T12:00:00Z")
		cache.FetchedAt = fetched
		exp := &model.Export{Opportunities: []model.Opportunity{contractPool("bsc:"+vault, "venus")}}
		attach(t, exp, parseLinks(t, linksConfig()), cache, nil)
		assert.Nil(t, exp.Summary.LinksDealsFetchedAt, "%v", fetched)
		assert.Nil(t, exp.Opportunities[0].Referral, "%v", fetched)
	}
}

func TestSwapAddressThatIsNotAnIdentifierIsSkippedWithoutAWarning(t *testing.T) {
	pool := fmt.Sprintf("base:0x%040x", 1)
	tk := token(1, [][2]string{{"base", "0xabc def"}}, 1, pool)
	exp := &model.Export{
		Opportunities: []model.Opportunity{contractPool(pool, "uniswap-v3", "base:0xabc def")},
		Tokens:        []model.Token{tk},
	}
	assert.Empty(t, attach(t, exp, parseLinks(t, linksConfig()), nil, nil))
	assert.Equal(t, []model.SwapLink{}, exp.Tokens[0].Links)
}

func TestBrokenDealRecordsInTheCacheDoNotCrashTheBuild(t *testing.T) {
	cache := ParseDealsCache(mustJSON(m{"fetched_at": "2026-11-12T12:00:00Z", "deals": m{"venus": "https://app.venus.io/?ref=cp0x", "backpack": nil}}))
	exp := &model.Export{
		Opportunities: []model.Opportunity{contractPool("bsc:"+vault, "venus")},
		Issuers:       []model.Issuer{{ID: backpack, Name: str("Backpack")}},
	}
	warnings := attach(t, exp, parseLinks(t, linksConfig()), cache, nil)
	assert.Nil(t, exp.Opportunities[0].Referral)
	assert.Nil(t, exp.Issuers[0].Referral)
	assert.Len(t, warnings, 2)

	list := ParseDealsCache(mustJSON(m{"fetched_at": "2026-11-12T12:00:00Z", "deals": []any{"venus"}}))
	exp = &model.Export{Opportunities: []model.Opportunity{contractPool("bsc:"+vault, "venus")}}
	attach(t, exp, parseLinks(t, linksConfig()), list, nil)
	assert.Nil(t, exp.Opportunities[0].Referral)
}

// A verification record that is not complete never enables a swap link, whatever the file validation says.
func TestOnlyACompleteVerificationRecordEnablesASwapLink(t *testing.T) {
	pool := fmt.Sprintf("base:0x%040x", 1)
	for name, v := range map[string]any{
		"complete": verifd, "string": "false", "bool": true, "number": 1, "empty": m{}, "list": []any{},
		"no sample": m{"at": "2026-11-13"}, "no date": m{"sample": "x"}, "bad date": m{"at": "yesterday", "sample": "x"}, "blank sample": m{"at": "2026-11-13", "sample": ""},
	} {
		cfg := linksConfig()
		cfg["swap"].(m)["base"].([]any)[0].(m)["verified"] = v
		exp := &model.Export{
			Opportunities: []model.Opportunity{contractPool(pool, "uniswap-v3", "base:"+vault)},
			Tokens:        []model.Token{token(1, [][2]string{{"base", vault}}, 1, pool)},
		}
		attach(t, exp, unvalidated(t, cfg), nil, nil)
		if name == "complete" {
			assert.Len(t, exp.Tokens[0].Links, 1, name)
		} else {
			assert.Empty(t, exp.Tokens[0].Links, name)
		}
	}
}

func TestOnlyACompleteVerificationRecordEnablesARouteInTheLogic(t *testing.T) {
	for name, v := range map[string]any{"string": "false", "bool": true, "number": 1, "empty": m{}, "no sample": m{"at": "2026-11-13"}, "bad date": m{"at": "yesterday", "sample": "x"}} {
		cfg := linksConfig()
		cfg["platforms"].(m)["uniswap-v4"].(m)["routes"].([]any)[0].(m)["networks"].(m)["robinhood"].(m)["verified"] = v
		exp := &model.Export{Opportunities: []model.Opportunity{opp("robinhood:"+poolID, "uniswap-v4", "pool_id", "dex_lp", "eligible")}}
		attach(t, exp, unvalidated(t, cfg), nil, nil)
		assert.Equal(t, &model.PositionLink{URL: "https://app.uniswap.org", Kind: "home"}, exp.Opportunities[0].Link, name)
	}
}

func TestOneWarningPerBrokenPlatformEntryHoweverManyPositionsUseIt(t *testing.T) {
	five := func() *model.Export {
		var opps []model.Opportunity
		for n := 1; n <= 5; n++ {
			opps = append(opps, opp(fmt.Sprintf("robinhood:0x%064x", n), "uniswap-v4", "pool_id", "dex_lp", "eligible"))
		}
		return &model.Export{Opportunities: opps}
	}
	badHome := linksConfig()
	badHome["platforms"].(m)["uniswap-v4"].(m)["home"] = "http://app.uniswap.org"
	badHome["platforms"].(m)["uniswap-v4"].(m)["routes"] = []any{}
	assert.Len(t, attach(t, five(), parseLinks(t, badHome), nil, nil), 1)

	badRoute := linksConfig()
	badRoute["platforms"].(m)["uniswap-v4"].(m)["routes"].([]any)[0].(m)["url"] = "https://uniswap.example.org/pools/{network}/{address}"
	assert.Len(t, attach(t, five(), parseLinks(t, badRoute), nil, nil), 1)
}

// An unreadable cache payload is an unavailable cache: nothing referral-shaped, one warning per deal platform.
func TestAnUnreadableDealsCacheIsAnUnavailableCache(t *testing.T) {
	for name, payload := range map[string][]byte{"not json": []byte("{nope"), "a string": []byte(`"x"`), "a list": []byte(`[]`), "empty": nil} {
		cache := ParseDealsCache(payload)
		exp := &model.Export{Opportunities: []model.Opportunity{contractPool("bsc:"+vault, "venus")}}
		warnings := attach(t, exp, parseLinks(t, linksConfig()), cache, nil)
		assert.Nil(t, exp.Opportunities[0].Referral, name)
		assert.Nil(t, exp.Summary.LinksDealsFetchedAt, name)
		assert.Equal(t, []string{"platform venus: deals referral venus unavailable, stale or invalid"}, warnings, name)
	}
}

// The collector loads the curated config once and reuses it for every publish: applying it must not change it.
func TestApplyDoesNotModifyTheConfig(t *testing.T) {
	load := func() *Config {
		cfg, err := Load("../../config")
		require.NoError(t, err)
		return cfg
	}
	cfg, pristine := load(), load()
	require.NotNil(t, cfg.Links)
	require.NotNil(t, cfg.Rights)
	for range 2 {
		exp := exportOf()
		exp.Opportunities = []model.Opportunity{contractPool("bsc:"+vault, "venus")}
		Apply(exp, cfg, dealsCache("2026-11-12T12:00:00Z"), nil, now)
	}
	assert.Equal(t, pristine, cfg)
}

func TestOverflowEdgesAreNotFresh(t *testing.T) {
	cache := dealsCache("2026-11-12T12:00:00Z")
	assert.True(t, cache.fresh(1, now))
	for _, days := range []float64{1e9, 1e300, -1e9} {
		assert.False(t, cache.fresh(days, now), "%g", days)
	}
	assert.False(t, cache.fresh(math.NaN(), now))
	// A cache from year 1 is old, not a wrapped duration.
	ancient := dealsCache("0001-01-01T00:00:00Z")
	assert.False(t, ancient.fresh(14, now))
	assert.True(t, ancient.fresh(999999999, now), "2025 years is within Python's largest timedelta")
}

func TestWholeDaysFloors(t *testing.T) {
	day := func(y int, mo time.Month, d int) time.Time { return time.Date(y, mo, d, 0, 0, 0, 0, time.UTC) }
	assert.Equal(t, int64(0), wholeDays(day(2026, 11, 14), day(2026, 11, 14)))
	assert.Equal(t, int64(180), wholeDays(day(2026, 11, 14), day(2026, 5, 18)))
	assert.Equal(t, int64(-1), wholeDays(day(2026, 11, 13), day(2026, 11, 14)))
	assert.Greater(t, wholeDays(day(2026, 11, 14), day(1, 1, 1)), int64(700000), "centuries apart do not saturate")
	assert.Equal(t, int64(-1), wholeDays(day(2026, 11, 14), day(2026, 11, 14).Add(time.Hour)), "Python floors")
}

func TestHugeReviewValidDaysNeverMakesAReviewStale(t *testing.T) {
	cfg := config(m{"review_valid_days": 9007199254740993}, profile("acme-line", []int{1}, m{"reviewed_at": "2001-01-01"}))
	exp, warnings := attachRights(t, cfg, nil, today)
	require.Len(t, exp.RightsProfiles, 1, "%v", warnings)
	assert.NotContains(t, string(exp.RightsProfiles[0]), `"stale":true`)
}

// Python compares `schema_version == 1` for links and issuers, so true passes like 1 and 1.0; rights is strict.
func TestSchemaVersionTrueIsAcceptedLikePython(t *testing.T) {
	for _, v := range []any{true, 1, 1.0} {
		cfg := linksConfig()
		cfg["schema_version"] = v
		_, err := ParseLinks("links.json", mustJSON(cfg))
		assert.NoError(t, err, "links %v", v)
		_, err = ParseIssuerWebsites("issuers.json", mustJSON(m{"schema_version": v, "issuers": m{}}))
		assert.NoError(t, err, "issuers %v", v)
	}
	for _, v := range []any{false, 2, "1", nil} {
		cfg := linksConfig()
		cfg["schema_version"] = v
		_, err := ParseLinks("links.json", mustJSON(cfg))
		assert.Error(t, err, "links %v", v)
		_, err = ParseIssuerWebsites("issuers.json", mustJSON(m{"schema_version": v, "issuers": m{}}))
		assert.Error(t, err, "issuers %v", v)
	}
	_, err := ParseRights("r.json", mustJSON(m{"schema_version": true, "profiles": []any{}}))
	assert.Error(t, err, "rights stays strict (type is int)")
}
