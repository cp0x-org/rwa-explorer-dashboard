package compute

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/model"
)

func ptr[T any](v T) *T { return &v }

func testSettings() *model.Settings {
	return &model.Settings{
		MaxGraphDepth:                  3,
		MinEligibleLiquidityUSD:        model.MustDecimal("10000"),
		MaxPositionAgeHours:            48,
		YieldAPYReviewThresholdPercent: model.MustDecimal("1000"),
		ChainMapping:                   map[string]*string{"Ethereum": ptr("ethereum"), "Base": ptr("base"), "Solana": nil},
		PlatformCategories:             map[string]string{"beefy": "vault", "uniswap-v3": "dex_lp", "xstocks": "wrapper"},
		TagCategories:                  [][2]string{{"#yield", "vault"}, {"#amm", "dex_lp"}},
	}
}

func requireDec(t *testing.T, want string, got model.Decimal, msgAndArgs ...any) {
	t.Helper()
	if want == "" {
		require.True(t, got.IsNull(), msgAndArgs...)
		return
	}
	require.False(t, got.IsNull(), msgAndArgs...)
	require.True(t, model.MustDecimal(want).V.Equal(*got.V), "want %s got %s %v", want, got.String(), msgAndArgs)
}

// ---------- numbers ----------

func TestDec(t *testing.T) {
	assert.True(t, Dec(nil).IsNull())
	assert.True(t, Dec(true).IsNull(), "bool is not a number")
	assert.True(t, Dec("NaN").IsNull())
	assert.True(t, Dec("Infinity").IsNull())
	assert.True(t, Dec("abc").IsNull())
	assert.True(t, Dec("").IsNull())
	assert.Equal(t, "12.5", Dec(" 12.5 ").String())
	assert.Equal(t, "0", Dec(0).String(), "zero is a value, not null")
	assert.Equal(t, "1.5", Dec(1.5).String())
	assert.Equal(t, "100000000000000000000", *Str(Dec("1e20")), "no exponent in output")
	assert.Nil(t, Str(model.Decimal{}))
}

func TestParseTime(t *testing.T) {
	got, ok := ParseTime("2026-10-08T21:11:23Z")
	require.True(t, ok)
	assert.Equal(t, "2026-10-08T21:11:23Z", FormatTime(got))
	got, ok = ParseTime("2026-10-18T12:00:00.123Z")
	require.True(t, ok)
	assert.Equal(t, 123000000, got.Nanosecond())
	_, ok = ParseTime("yesterday")
	assert.False(t, ok)

	// shapes Python 3.11+ fromisoformat accepts
	for in, want := range map[string]string{
		"2026-10-08T21:00Z":          "2026-10-08T21:00:00Z",
		"2026-10-08T21:00:05+0000":   "2026-10-08T21:00:05Z",
		"2026-10-08T21:00:05+00:00":  "2026-10-08T21:00:05Z",
		"2026-10-08T23:00:05+02:00":  "2026-10-08T21:00:05Z",
		"2026-10-08T21:00:05.5Z":     "2026-10-08T21:00:05Z",
		"2026-10-08 21:00:05":        "2026-10-08T21:00:05Z",
		"2026-10-08 21:00:05+00:00":  "2026-10-08T21:00:05Z",
		"20261008T210005Z":           "2026-10-08T21:00:05Z",
		"20261008T2100Z":             "2026-10-08T21:00:00Z",
		"2026-10-08":                 "2026-10-08T00:00:00Z",
		"2026-10-08T21:11:23.123456": "2026-10-08T21:11:23Z",
	} {
		got, ok := ParseTime(in)
		require.True(t, ok, in)
		assert.Equal(t, want, FormatTime(got), in)
	}
	assert.Equal(t, "eligible", Eligibility(func() *Opp {
		o := NormalizeOpportunity(rec("base:0x1", func(r *model.PortalsRecord) { r.UpdatedAt = ptr("2026-10-08T21:00Z") }), testSettings(), nil)
		return &o
	}(), mustTime(t, "2026-10-08T21:30:00Z"), testSettings()), "no-seconds timestamps are fresh, not unknown_freshness")
}

func mustTime(t *testing.T, s string) time.Time {
	t.Helper()
	got, ok := ParseTime(s)
	require.True(t, ok, s)
	return got
}

func TestFormatTimeTruncatesToSeconds(t *testing.T) {
	assert.Equal(t, "2026-10-08T21:11:23Z", FormatTime(time.Date(2026, 10, 8, 21, 11, 23, 987000000, time.UTC)))
}

func TestQuantizeHalfEven(t *testing.T) {
	assert.Equal(t, "0.12", quantizedRatio(1, 8, 2).String(), "0.125 -> 0.12")
	assert.Equal(t, "0.38", quantizedRatio(3, 8, 2).String(), "0.375 -> 0.38")
	assert.Equal(t, "66.6", pyMean([]float64{66.65, 66.65}).String(), "66.65 -> 66.6")
	assert.Equal(t, "12.4", pyMean([]float64{12.25, 12.35, 12.45}).String(), "12.35 -> 12.4")
	assert.Equal(t, "44.4", pyMean([]float64{100.0 / 3, 100, 0}).String(), "statistics.mean gives 44.44444444444444")
	assert.Equal(t, "0.1", pyMean([]float64{0.05, 0.15}).String())
}

func TestIssuerMetaAnyKeyCountsAsPresent(t *testing.T) {
	var cache map[string]IssuerMeta
	require.NoError(t, json.Unmarshal([]byte(`{"6a72fe7072996604d45c6733":{"issuer_id":"x"},"6878977dcbbf471de3366e85":{}}`), &cache))
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "snapshots", "nvda_snapshot.json"))
	require.NoError(t, err)
	snap, err := model.ParseSnapshot(b)
	require.NoError(t, err)
	settings, err := model.LoadSettings(filepath.Join("..", "..", "config", "settings.json"))
	require.NoError(t, err)
	res, err := Build([]*model.Snapshot{snap}, cache, settings, nil)
	require.NoError(t, err)
	status := map[string]string{}
	for _, i := range res.Export.Issuers {
		status[i.ID] = i.MetadataStatus
	}
	assert.Equal(t, "cmc", status["6a72fe7072996604d45c6733"])
	assert.Equal(t, "unknown", status["6878977dcbbf471de3366e85"])
}

func TestBuildValidatesSettings(t *testing.T) {
	for name, mut := range map[string]func(*model.Settings){
		"liquidity": func(s *model.Settings) { s.MinEligibleLiquidityUSD = model.Decimal{} },
		"threshold": func(s *model.Settings) { s.YieldAPYReviewThresholdPercent = model.Decimal{} },
		"age":       func(s *model.Settings) { s.MaxPositionAgeHours = 0 },
		"depth":     func(s *model.Settings) { s.MaxGraphDepth = 0 },
	} {
		s := testSettings()
		mut(s)
		_, err := Build(nil, nil, s, nil)
		assert.Error(t, err, name)
	}
	_, err := Build(nil, nil, nil, nil)
	assert.Error(t, err)
}

func TestNorm(t *testing.T) {
	assert.Equal(t, "BRKB", norm("brk.b"))
	assert.Equal(t, "NVDAX", norm("NVDA-x "))
	assert.Equal(t, "", norm("--"))
}

// ---------- tokens ----------

func TestClassifyToken(t *testing.T) {
	cases := []struct {
		name         string
		token        model.CMCToken
		ticker       string
		kind, reason string
	}{
		{"missing symbol", model.CMCToken{}, "NVDA", "unclassified", "missing_symbol"},
		{"empty symbol", model.CMCToken{Symbol: ptr("")}, "NVDA", "unclassified", "missing_symbol"},
		{"derivative by name", model.CMCToken{Symbol: ptr("NVDA"), Name: ptr("NVIDIA (Derivatives)")}, "NVDA", "derivative", "derivative"},
		{"derivative by issuer", model.CMCToken{Symbol: ptr("NVDA"), IssuerName: ptr("NA (DERIVATIVES)")}, "NVDA", "derivative", "derivative"},
		{"other share class", model.CMCToken{Symbol: ptr("GOOGL")}, "GOOG", "tokenized", ""},
		{"symbol does not match", model.CMCToken{Symbol: ptr("TSLAX")}, "NVDA", "unclassified", "symbol_does_not_match_ticker"},
		{"wrapper", model.CMCToken{Symbol: ptr("WNVDAX"), Name: ptr("Wrapped NVIDIA Tokenized stock")}, "NVDA", "wrapper", ""},
		{"tokenized", model.CMCToken{Symbol: ptr("NVDAon"), Name: ptr("NVIDIA Tokenized Stock (Ondo)")}, "NVDA", "tokenized", ""},
		{"punctuation ignored", model.CMCToken{Symbol: ptr("BRKBx")}, "BRK.B", "tokenized", ""},
		{"derivative beats mismatch", model.CMCToken{Symbol: ptr("ZZZ"), Name: ptr("X (derivatives)")}, "NVDA", "derivative", "derivative"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			kind, reason := ClassifyToken(c.token, c.ticker)
			assert.Equal(t, c.kind, kind)
			if c.reason == "" {
				assert.Nil(t, reason)
			} else {
				require.NotNil(t, reason)
				assert.Equal(t, c.reason, *reason)
			}
		})
	}
}

func TestNormalizeAddress(t *testing.T) {
	assert.Equal(t, "0xabcdefabcdefabcdefabcdefabcdefabcdefabcd", NormalizeAddress("0xABCDEFabcdefABCDEFabcdefABCDEFabcdefABCD"))
	assert.Equal(t, "Xsc9qvGR1efVDFGLrVsmkzv3qi45LTBjeUKSPmx9qEh", NormalizeAddress("Xsc9qvGR1efVDFGLrVsmkzv3qi45LTBjeUKSPmx9qEh"), "non-EVM keeps its case")
	assert.Equal(t, "0xABC", NormalizeAddress("0xABC"), "short hex is not an EVM address")
}

func TestParseDeployments(t *testing.T) {
	mapping := testSettings().ChainMapping
	addr := "0xB989ad9b91886b1Aaed8DaADb26F028b29b40945"
	lower := "0xb989ad9b91886b1aaed8daadb26f028b29b40945"
	ca := func(a, network string) model.CMCContractAddress {
		c := model.CMCContractAddress{ContractAddress: a}
		c.Platform = &struct {
			Name *string `json:"name"`
		}{Name: ptr(network)}
		return c
	}

	t.Run("nil info", func(t *testing.T) { assert.Empty(t, ParseDeployments(nil, mapping)) })

	t.Run("same address on two networks is two deployments, EVM lower-cased", func(t *testing.T) {
		deps := ParseDeployments(&model.CMCInfo{ContractAddress: []model.CMCContractAddress{ca(addr, "Ethereum"), ca(addr, "Base")}}, mapping)
		require.Len(t, deps, 2)
		assert.Equal(t, "ethereum:"+lower, *deps[0].Key)
		assert.Equal(t, "base:"+lower, *deps[1].Key)
		assert.Equal(t, lower, deps[0].Address)
		assert.Equal(t, addr, deps[0].AddressRaw)
		assert.Empty(t, deps[0].Status, "resolved after the Portals lookup")
	})

	t.Run("duplicates collapse", func(t *testing.T) {
		deps := ParseDeployments(&model.CMCInfo{ContractAddress: []model.CMCContractAddress{ca(addr, "Ethereum"), ca(lower, "Ethereum")}}, mapping)
		assert.Len(t, deps, 1)
	})

	t.Run("fallback token_address is added once, case-insensitively", func(t *testing.T) {
		info := &model.CMCInfo{Platform: &model.CMCInfoPlatform{Name: ptr("Base"), TokenAddress: ptr(addr)}}
		deps := ParseDeployments(info, mapping)
		require.Len(t, deps, 1)
		assert.Equal(t, "base:"+lower, *deps[0].Key)

		info.ContractAddress = []model.CMCContractAddress{ca(lower, "Ethereum")}
		deps = ParseDeployments(info, mapping)
		require.Len(t, deps, 1, "already listed, whatever the network of the fallback")
		assert.Equal(t, "Ethereum", deps[0].CMCNetwork)
	})

	t.Run("unmapped vs unsupported, non-EVM case kept", func(t *testing.T) {
		sol := "Xsc9qvGR1efVDFGLrVsmkzv3qi45LTBjeUKSPmx9qEh"
		deps := ParseDeployments(&model.CMCInfo{ContractAddress: []model.CMCContractAddress{ca(sol, "Solana"), ca("0xdef", "Some New Chain")}}, mapping)
		require.Len(t, deps, 2)
		assert.Equal(t, "unsupported_network", deps[0].Status, "known but not supported by Portals")
		assert.Nil(t, deps[0].Network)
		assert.Nil(t, deps[0].Key)
		assert.Equal(t, sol, deps[0].Address)
		assert.Equal(t, "unmapped_network", deps[1].Status, "missing from chain_mapping")
	})

	t.Run("entries without address or network are skipped, address trimmed", func(t *testing.T) {
		deps := ParseDeployments(&model.CMCInfo{ContractAddress: []model.CMCContractAddress{ca("  ", "Base"), ca("0x1", ""), ca(" 0xAbC ", "Base")}}, mapping)
		require.Len(t, deps, 1)
		assert.Equal(t, "0xAbC", deps[0].AddressRaw)
	})
}

func TestDeploymentStatusAndCoverage(t *testing.T) {
	key := "base:0x1"
	dep := model.Deployment{Network: ptr("base"), Key: &key}
	assert.Equal(t, "data_unavailable", DeploymentStatus(dep, nil))
	assert.Equal(t, "unsupported_network", DeploymentStatus(dep, &model.SnapshotPortals{Networks: []string{"ethereum"}}))
	assert.Equal(t, "not_checked", DeploymentStatus(dep, &model.SnapshotPortals{Networks: []string{"base"}}))
	p := &model.SnapshotPortals{Networks: []string{"base"}, LookedUp: []string{key}}
	assert.Equal(t, "not_indexed", DeploymentStatus(dep, p))
	p.Exact = map[string]model.PortalsRecord{key: {}}
	assert.Equal(t, "indexed", DeploymentStatus(dep, p))
	dep.Status = "unmapped_network"
	assert.Equal(t, "unmapped_network", DeploymentStatus(dep, p), "a status decided earlier wins")

	st := func(s ...string) []model.Deployment {
		var out []model.Deployment
		for _, v := range s {
			out = append(out, model.Deployment{Status: v})
		}
		return out
	}
	assert.Equal(t, "data_unavailable", TokenCoverage(st("indexed"), false))
	assert.Equal(t, "no_contracts", TokenCoverage(nil, true))
	assert.Equal(t, "search", TokenCoverage(st("not_checked", "indexed"), true))
	assert.Equal(t, "partial", TokenCoverage(st("not_indexed", "not_checked"), true))
	assert.Equal(t, "partial", TokenCoverage(st("data_unavailable"), true))
	assert.Equal(t, "not_indexed", TokenCoverage(st("unsupported_network", "not_indexed"), true))
	assert.Equal(t, "unsupported_by_portals", TokenCoverage(st("unsupported_network", "unmapped_network"), true))
}

// ---------- positions ----------

func rec(key string, mut func(*model.PortalsRecord)) model.PortalsRecord {
	r := model.PortalsRecord{
		Key: key, Address: "0xa1" + strings.Repeat("0", 38), Platform: ptr("beefy"), Network: ptr("base"),
		Symbol: ptr("X"), Liquidity: lenient("50000"), UpdatedAt: ptr("2026-10-08T20:00:00Z"),
		Metrics: &model.PortalsMetrics{APY: lenient("5")}, IsPortable: ptr(true),
	}
	if mut != nil {
		mut(&r)
	}
	return r
}

func lenient(s string) model.LenientDecimal {
	return model.LenientDecimal{Decimal: model.MustDecimal(s)}
}

func TestClassifyPosition(t *testing.T) {
	s := testSettings()
	assert.Equal(t, "vault", ClassifyPosition(model.PortalsRecord{Platform: ptr("beefy")}, s), "platform wins")
	assert.Equal(t, "vault", ClassifyPosition(model.PortalsRecord{Platform: ptr("beefy"), Metadata: &model.PortalsMetadata{Tags: []string{"#amm"}}}, s))
	assert.Equal(t, "vault", ClassifyPosition(model.PortalsRecord{Platform: ptr("zzz"), Metadata: &model.PortalsMetadata{Tags: []string{"#amm", "#yield"}}}, s), "first tag_categories entry wins, not first tag")
	assert.Equal(t, "dex_lp", ClassifyPosition(model.PortalsRecord{Metadata: &model.PortalsMetadata{Tags: []string{"#amm"}}}, s))
	assert.Equal(t, "unknown", ClassifyPosition(model.PortalsRecord{Platform: ptr("zzz")}, s))
}

func TestNormKeyAndChildKeys(t *testing.T) {
	assert.Equal(t, "base:0xabcdefabcdefabcdefabcdefabcdefabcdefabcd", NormKey("base:0xABCDEFabcdefABCDEFabcdefABCDEFabcdefABCD"))
	assert.Equal(t, "solana:AbC", NormKey("solana:AbC"))
	r := model.PortalsRecord{Network: ptr("base"), Tokens: []string{"0xABCDEFabcdefABCDEFabcdefABCDEFabcdefABCD", "ethereum:0x1", "", "plain"}}
	assert.Equal(t, []string{"base:0xabcdefabcdefabcdefabcdefabcdefabcdefabcd", "ethereum:0x1", "base:plain"}, ChildKeys(r))
}

func TestNormalizeOpportunityFlags(t *testing.T) {
	s := testSettings()
	flags := func(mut func(*model.PortalsRecord)) []string {
		o := NormalizeOpportunity(rec("base:0x1", mut), s, nil)
		return o.Flags
	}
	assert.Empty(t, flags(nil))
	assert.Equal(t, []string{"sentinel_price"}, flags(func(r *model.PortalsRecord) { r.Price = lenient("9007199254740990") }))
	assert.Empty(t, flags(func(r *model.PortalsRecord) { r.Price = lenient("9e14") }), "below the sentinel")
	assert.Equal(t, []string{"apy_missing"}, flags(func(r *model.PortalsRecord) { r.Metrics = nil }))
	assert.Equal(t, []string{"apy_missing"}, flags(func(r *model.PortalsRecord) { r.Metrics = &model.PortalsMetrics{} }))
	assert.Equal(t, []string{"apy_anomaly"}, flags(func(r *model.PortalsRecord) { r.Metrics.APY = lenient("1000.01") }))
	assert.Empty(t, flags(func(r *model.PortalsRecord) { r.Metrics.APY = lenient("1000") }), "the threshold itself is fine")
	assert.Equal(t, []string{"apy_negative"}, flags(func(r *model.PortalsRecord) { r.Metrics.APY = lenient("-0.1") }))
	assert.Empty(t, flags(func(r *model.PortalsRecord) { r.Metrics.APY = lenient("0") }), "zero APY is reported, not missing")
	assert.Equal(t, []string{"negative_volume_net_flow"}, flags(func(r *model.PortalsRecord) { r.Metrics.VolumeUSD7d = lenient("-1") }))
	assert.Equal(t, []string{"liquidity_missing"}, flags(func(r *model.PortalsRecord) { r.Liquidity = model.LenientDecimal{} }))
	assert.Equal(t, []string{"range_details_unknown"}, flags(func(r *model.PortalsRecord) {
		r.Platform = ptr("uniswap-v3")
		r.Metadata = &model.PortalsMetadata{Tags: []string{"#clp"}}
	}))
	assert.Empty(t, flags(func(r *model.PortalsRecord) { r.Metadata = &model.PortalsMetadata{Tags: []string{"#clp"}} }), "#clp matters for dex_lp only")
	// flag order: sentinel, apy, volume, range, liquidity
	assert.Equal(t, []string{"sentinel_price", "apy_missing", "negative_volume_net_flow", "range_details_unknown", "liquidity_missing"},
		flags(func(r *model.PortalsRecord) {
			r.Price = lenient("9e15")
			r.Metrics = &model.PortalsMetrics{VolumeUSD1d: lenient("-5")}
			r.Platform = ptr("uniswap-v3")
			r.Metadata = &model.PortalsMetadata{Tags: []string{"#clp"}}
			r.Liquidity = model.LenientDecimal{}
		}))
}

func TestNormalizeOpportunityFields(t *testing.T) {
	s := testSettings()
	names := map[string]string{"beefy": "Beefy"}
	o := NormalizeOpportunity(rec("Base:0xABCDEFabcdefABCDEFabcdefABCDEFabcdefABCD", func(r *model.PortalsRecord) {
		r.Address = "0xABCDEFabcdefABCDEFabcdefABCDEFabcdefABCD"
	}), s, names)
	assert.Equal(t, "Base:0xabcdefabcdefabcdefabcdefabcdefabcdefabcd", o.Key, "only the address part is normalized")
	assert.Equal(t, "Beefy", o.PlatformName)
	assert.Equal(t, "vault", o.Category)
	assert.Equal(t, "vault_tvl", o.LiquiditySemantic)
	assert.Equal(t, "contract", o.PositionKind)
	require.NotNil(t, o.ReceiptTokenKey, "portable and 42 chars")
	assert.Equal(t, o.Key, *o.ReceiptTokenKey)

	pool := NormalizeOpportunity(rec("base:0xp", func(r *model.PortalsRecord) { r.Address = "0x" + strings.Repeat("0", 64) }), s, nil)
	assert.Equal(t, "pool_id", pool.PositionKind, "66-char address")
	assert.Nil(t, pool.ReceiptTokenKey)
	assert.Equal(t, "beefy", pool.PlatformName, "unknown platform name falls back to the slug")

	unk := NormalizeOpportunity(rec("base:0xu", func(r *model.PortalsRecord) { r.Platform = ptr("zzz") }), s, nil)
	assert.Equal(t, "unknown", unk.Category)
	assert.Equal(t, "unknown", unk.LiquiditySemantic)

	assert.Nil(t, NormalizeOpportunity(rec("base:0xn", func(r *model.PortalsRecord) { r.IsPortable = nil }), s, nil).ReceiptTokenKey)
	assert.Nil(t, NormalizeOpportunity(rec("base:0xn", func(r *model.PortalsRecord) { r.IsPortable = ptr(false) }), s, nil).ReceiptTokenKey)
	// None != 0: a missing metric stays null, a reported 0 stays 0.
	z := NormalizeOpportunity(rec("base:0xz", func(r *model.PortalsRecord) { r.Metrics = &model.PortalsMetrics{APY: lenient("0")} }), s, nil)
	requireDec(t, "0", z.APYPercent)
	requireDec(t, "", z.BaseAPYPercent)
}

func TestEligibility(t *testing.T) {
	s := testSettings()
	fetched, _ := ParseTime("2026-10-08T21:00:00Z")
	mk := func(mut func(*Opp)) *Opp {
		o := NormalizeOpportunity(rec("base:0x1", nil), s, nil)
		if mut != nil {
			mut(&o)
		}
		return &o
	}
	assert.Equal(t, "eligible", Eligibility(mk(nil), fetched, s))
	assert.Equal(t, "category_excluded", Eligibility(mk(func(o *Opp) { o.Category = "wrapper" }), fetched, s))
	assert.Equal(t, "unknown_freshness", Eligibility(mk(func(o *Opp) { o.UpdatedAt = nil }), fetched, s))
	assert.Equal(t, "unknown_freshness", Eligibility(mk(func(o *Opp) { o.UpdatedAt = ptr("garbage") }), fetched, s))
	assert.Equal(t, "eligible", Eligibility(mk(func(o *Opp) { o.UpdatedAt = ptr("2026-10-06T21:00:00Z") }), fetched, s), "exactly 48h is not stale")
	assert.Equal(t, "stale", Eligibility(mk(func(o *Opp) { o.UpdatedAt = ptr("2026-10-06T20:59:59Z") }), fetched, s))
	assert.Equal(t, "unknown_liquidity", Eligibility(mk(func(o *Opp) { o.LiquidityUSD = model.Decimal{} }), fetched, s))
	assert.Equal(t, "unknown_liquidity", Eligibility(mk(func(o *Opp) { o.LiquiditySemantic = "unknown" }), fetched, s))
	assert.Equal(t, "low_liquidity", Eligibility(mk(func(o *Opp) { o.LiquidityUSD = model.MustDecimal("9999.99") }), fetched, s))
	assert.Equal(t, "eligible", Eligibility(mk(func(o *Opp) { o.LiquidityUSD = model.MustDecimal("10000") }), fetched, s), "the minimum itself passes")
	assert.Equal(t, "low_liquidity", Eligibility(mk(func(o *Opp) { o.LiquidityUSD = model.MustDecimal("0") }), fetched, s), "zero is a number, not unknown")
	// freshness is measured against fetched_at, not against now: an old snapshot does not decay
	old, _ := ParseTime("2020-01-01T00:00:00Z")
	assert.Equal(t, "eligible", Eligibility(mk(func(o *Opp) { o.UpdatedAt = ptr("2020-01-01T00:00:00Z") }), old.Add(1e9), s))
}

// ---------- graph ----------

func graphRecords(spec map[string][]string) map[string]model.PortalsRecord {
	out := map[string]model.PortalsRecord{}
	for k, children := range spec {
		out[k] = model.PortalsRecord{Key: k, Network: ptr("base"), Tokens: children}
	}
	return out
}

func TestFindRelations(t *testing.T) {
	// R <- lp1 <- v1 <- v2 <- v3 ; lp2 contains R and lp1 (so v1 is also reachable from a shorter path)
	records := graphRecords(map[string][]string{
		"base:lp1": {"base:R"}, "base:v1": {"base:lp1"}, "base:v2": {"base:v1"}, "base:v3": {"base:v2"},
	})
	idx := ParentIndex(records)
	rel := FindRelations([]string{"base:R"}, idx, nil, 3)
	require.Len(t, rel, 3, "depth is capped at max_depth")
	assert.Equal(t, model.Relation{Key: "base:lp1", Depth: 1, Path: []string{"base:R", "base:lp1"}}, rel["base:lp1"])
	assert.Equal(t, 3, rel["base:v2"].Depth)
	assert.Equal(t, []string{"base:R", "base:lp1", "base:v1", "base:v2"}, rel["base:v2"].Path)
	assert.NotContains(t, rel, "base:v3")
	assert.Len(t, FindRelations([]string{"base:R"}, idx, nil, 1), 1)
	assert.Empty(t, FindRelations(nil, idx, nil, 3))

	t.Run("stop keys are neither counted nor traversed", func(t *testing.T) {
		rel := FindRelations([]string{"base:R"}, idx, map[string]bool{"base:v1": true}, 5)
		assert.ElementsMatch(t, []string{"base:lp1"}, keys(rel))
	})

	t.Run("cycles are cut by visited and the root is never its own relation", func(t *testing.T) {
		records := graphRecords(map[string][]string{"base:a": {"base:R", "base:b"}, "base:b": {"base:a"}})
		rel := FindRelations([]string{"base:R"}, ParentIndex(records), nil, 5)
		assert.ElementsMatch(t, []string{"base:a", "base:b"}, keys(rel))
		assert.Equal(t, 1, rel["base:a"].Depth)
		assert.Equal(t, 2, rel["base:b"].Depth)

		self := graphRecords(map[string][]string{"base:R": {"base:R"}})
		assert.Empty(t, FindRelations([]string{"base:R"}, ParentIndex(self), nil, 5))
	})

	t.Run("min depth wins across roots and paths", func(t *testing.T) {
		// v contains lp (depth 2 from R1) and R2 directly (depth 1)
		records := graphRecords(map[string][]string{"base:lp": {"base:R1"}, "base:v": {"base:lp", "base:R2"}})
		rel := FindRelations([]string{"base:R2", "base:R1"}, ParentIndex(records), nil, 3)
		assert.Equal(t, 1, rel["base:v"].Depth)
		assert.Equal(t, []string{"base:R2", "base:v"}, rel["base:v"].Path)
		assert.Equal(t, 1, rel["base:lp"].Depth)
	})

	t.Run("equal depth: the first sorted root wins", func(t *testing.T) {
		records := graphRecords(map[string][]string{"base:v": {"base:B", "base:A"}})
		rel := FindRelations([]string{"base:B", "base:A"}, ParentIndex(records), nil, 3)
		assert.Equal(t, []string{"base:A", "base:v"}, rel["base:v"].Path)
	})
}

func keys(r Relations) []string {
	var out []string
	for k := range r {
		out = append(out, k)
	}
	return out
}

func TestSymbolCandidates(t *testing.T) {
	mk := func(network, category, symbol string) *Opp {
		return &Opp{Opportunity: model.Opportunity{Network: network, Category: category, Symbol: &symbol}}
	}
	opps := map[string]*Opp{
		"base:slash":   mk("base", "dex_lp", "NVDAc/USDC"),
		"base:dash":    mk("base", "vault", "usdc-nvdac"),
		"base:space":   mk("base", "lending_supply", "Morpho NVDAc"),
		"base:under":   mk("base", "staking", "NVDAc_ETH"),
		"base:exact":   mk("base", "yield_market", "NVDA.c"),
		"base:longer":  mk("base", "dex_lp", "NVDAcX/USDC"),
		"base:other":   mk("ethereum", "dex_lp", "NVDAc/USDC"),
		"base:wrapper": mk("base", "wrapper", "NVDAc"),
		"base:known":   mk("base", "dex_lp", "NVDAc/USDC"),
		"base:nosym":   {Opportunity: model.Opportunity{Network: "base", Category: "vault"}},
	}
	got := SymbolCandidates("NVDAc", map[string]bool{"base": true}, opps, map[string]bool{"base:known": true})
	assert.Equal(t, []string{"base:dash", "base:exact", "base:slash", "base:space", "base:under"}, got, "sorted; whole parts only; same network; use categories only")
	assert.Empty(t, SymbolCandidates("NVDAc", map[string]bool{}, opps, nil))
}

// ---------- aggregation ----------

func TestBreakdownOrdering(t *testing.T) {
	opps := map[string]*Opp{}
	rels := Relations{}
	add := func(key, network, eligibility string) {
		opps[key] = &Opp{Opportunity: model.Opportunity{Key: key, Network: network, Eligibility: eligibility}}
		rels[key] = model.Relation{Key: key, Depth: 1}
	}
	add("1", "ethereum", "eligible")
	add("2", "base", "eligible")
	add("3", "base", "eligible")
	add("4", "arbitrum", "stale")
	add("5", "arbitrum", "stale")
	add("6", "arbitrum", "stale")
	add("7", "zksync", "stale")
	add("8", "avalanche", "stale")
	add("9", "", "stale")
	b := breakdown(rels, opps, byNetwork)
	// -eligible, then -raw, then key
	assert.Equal(t, []string{"base", "ethereum", "arbitrum", "avalanche", "unknown", "zksync"}, b.Keys)
	assert.Equal(t, model.BreakdownCount{Raw: 2, Eligible: 2}, b.Values["base"])
	assert.Equal(t, model.BreakdownCount{Raw: 3, Eligible: 0}, b.Values["arbitrum"])
	assert.Empty(t, breakdown(Relations{}, opps, byNetwork).Keys)
	assert.NotNil(t, breakdown(Relations{}, opps, byNetwork).Values)
}

func TestMedianAndMean(t *testing.T) {
	d := func(ss ...string) []decimal.Decimal {
		var out []decimal.Decimal
		for _, s := range ss {
			out = append(out, decimal.RequireFromString(s))
		}
		return out
	}
	assert.Equal(t, "2", median(d("3", "1", "2")).String(), "odd: the middle value")
	assert.Equal(t, "2.5", median(d("4", "1", "3", "2")).String(), "even: mean of the two middles")
	assert.Equal(t, "7", median(d("7")).String())
	assert.Equal(t, "1.6667", mean(d("1", "2", "2")).String())
	assert.Equal(t, "0.0002", mean(d("0.0001", "0.0002")).String(), "0.00015: ties round to even (2)")
	assert.Equal(t, "0.0002", mean(d("0.0002", "0.0003")).String(), "0.00025: ties round to even (2)")
	assert.Equal(t, "44.2418", mean(d("80.158", "48.746", "25.439", "22.967", "43.899")).String())
}

func TestYieldProfile(t *testing.T) {
	opps := map[string]*Opp{}
	rels := Relations{}
	add := func(key, category, apy, eligibility string, flags ...string) {
		o := &Opp{Opportunity: model.Opportunity{Key: key, Category: category, Eligibility: eligibility, Flags: flags}}
		if apy != "" {
			o.APYPercent = model.MustDecimal(apy)
		}
		opps[key], rels[key] = o, model.Relation{Key: key, Depth: 1}
	}
	empty := YieldProfile(rels, opps)
	assert.Equal(t, "unknown", empty.Status)
	assert.Empty(t, empty.TopKeys)
	assert.Nil(t, empty.MaxReportedAPYKey)

	add("stale", "vault", "99", "stale")
	assert.Equal(t, "unknown", YieldProfile(rels, opps).Status, "stale positions are not eligible")
	add("zero", "vault", "0", "eligible")
	add("none", "vault", "", "eligible")
	add("anomaly", "vault", "5000", "eligible", "apy_anomaly")
	p := YieldProfile(rels, opps)
	assert.Equal(t, "no_reported_yield_in_coverage", p.Status)
	assert.Equal(t, 1, p.MissingAPYCount, "None is counted apart from 0")
	assert.Equal(t, 1, p.AnomalyCount)
	assert.Equal(t, 0, p.YieldOpportunitiesCount)

	add("a", "dex_lp", "10", "eligible")
	add("b", "dex_lp", "30", "eligible")
	add("c", "vault", "20", "eligible")
	add("d", "vault", "30", "eligible") // ties with b: key order breaks it
	p = YieldProfile(rels, opps)
	assert.Equal(t, "reported", p.Status)
	assert.Equal(t, 4, p.YieldOpportunitiesCount)
	requireDec(t, "30", p.MaxReportedAPYPercent)
	assert.Equal(t, "b", *p.MaxReportedAPYKey)
	assert.Equal(t, []string{"b", "d", "c", "a"}, p.TopKeys)
	requireDec(t, "22.5", p.AverageReportedAPYPercent)
	requireDec(t, "25", p.MedianReportedAPYPercent)
	assert.Equal(t, []string{"dex_lp", "vault"}, p.ByCategory.Keys, "categories in order of first appearance among the sorted yields")
	assert.Equal(t, 2, p.ByCategory.Values["vault"].Count)
	requireDec(t, "25", p.ByCategory.Values["vault"].Average)
	requireDec(t, "30", p.ByCategory.Values["vault"].Max)

	for _, k := range []string{"e", "f", "g"} {
		add(k, "vault", "1", "eligible")
	}
	assert.Len(t, YieldProfile(rels, opps).TopKeys, 5, "top 5 only")
}

func TestOpportunityCountsAndPlatforms(t *testing.T) {
	opps := map[string]*Opp{
		"a": {Opportunity: model.Opportunity{Platform: "uniswap-v3", Network: "base", Eligibility: "eligible"}},
		"b": {Opportunity: model.Opportunity{Platform: "uniswap-v3", Network: "ethereum", Eligibility: "eligible"}},
		"c": {Opportunity: model.Opportunity{Platform: "beefy", Network: "base", Eligibility: "eligible"}},
		"d": {Opportunity: model.Opportunity{Platform: "aave", Network: "base", Eligibility: "unknown_liquidity"}},
		"e": {Opportunity: model.Opportunity{Platform: "aave", Network: "arbitrum", Eligibility: "stale"}},
	}
	rels := Relations{"a": {Depth: 1}, "b": {Depth: 2}, "c": {Depth: 3}, "d": {Depth: 1}, "e": {Depth: 1}}
	c := OpportunityCounts(rels, opps)
	assert.Equal(t, model.OpportunityCounts{
		RawConfirmedOpportunitiesCount: 5, EligibleOpportunitiesCount: 3, UnknownLiquidityCount: 1,
		DirectEligibleCount: 1, IndirectEligibleCount: 2, EligibleProtocolsCount: 2, EligibleNetworksCount: 2,
	}, c)
	assert.Equal(t, []string{"Beefy", "uniswap-v3"}, EligiblePlatforms(rels, opps, map[string]string{"beefy": "Beefy"}), "names where known, slugs otherwise, sorted")
	assert.Equal(t, []string{}, append([]string{}, EligiblePlatforms(Relations{}, opps, nil)...))
}

func TestRankKey(t *testing.T) {
	tok := func(id int64, eligible, protocols, networks, direct int) *model.Token {
		return &model.Token{ID: id, Counts: model.TokenCounts{OpportunityCounts: model.OpportunityCounts{
			EligibleOpportunitiesCount: eligible, EligibleProtocolsCount: protocols, EligibleNetworksCount: networks, DirectEligibleCount: direct}}}
	}
	assert.Negative(t, RankKey(tok(2, 5, 0, 0, 0), tok(1, 4, 9, 9, 9)), "more eligible first")
	assert.Negative(t, RankKey(tok(2, 5, 3, 0, 0), tok(1, 5, 2, 9, 9)), "then protocols")
	assert.Negative(t, RankKey(tok(2, 5, 3, 2, 0), tok(1, 5, 3, 1, 9)), "then networks")
	assert.Negative(t, RankKey(tok(2, 5, 3, 2, 4), tok(1, 5, 3, 2, 3)), "then direct")
	assert.Negative(t, RankKey(tok(1, 5, 3, 2, 4), tok(2, 5, 3, 2, 4)), "finally the lower id")
}

// ---------- fixture (expected values taken from the Python implementation) ----------

func loadFixture(t *testing.T) *Result {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "snapshots", "nvda_snapshot.json"))
	require.NoError(t, err)
	snap, err := model.ParseSnapshot(b)
	require.NoError(t, err)
	settings, err := model.LoadSettings(filepath.Join("..", "..", "config", "settings.json"))
	require.NoError(t, err)
	res, err := Build([]*model.Snapshot{snap}, nil, settings, map[string]string{"beefy": "Beefy"})
	require.NoError(t, err)
	return res
}

func tokenByID(t *testing.T, e *model.Export, id int64) *model.Token {
	t.Helper()
	for i := range e.Tokens {
		if e.Tokens[i].ID == id {
			return &e.Tokens[i]
		}
	}
	t.Fatalf("token %d not found", id)
	return nil
}

func TestFixtureSummaryAndUnderlying(t *testing.T) {
	e := &loadFixture(t).Export
	s := e.Summary
	assert.Equal(t, ScoreVersion, e.ScoreVersion)
	assert.Equal(t, 1, s.UnderlyingsCount)
	assert.Equal(t, 4, s.TokensCount)
	assert.Equal(t, 1, s.ExcludedTokens)
	assert.Equal(t, 3, s.IssuersCount)
	assert.Equal(t, 13, s.DeploymentsCount)
	assert.Equal(t, 3, s.IndexedDeploys)
	assert.Equal(t, 1, s.TokensInDefi)
	assert.Equal(t, model.OpportunityCounts{
		RawConfirmedOpportunitiesCount: 16, EligibleOpportunitiesCount: 7, UnknownLiquidityCount: 0,
		DirectEligibleCount: 5, IndirectEligibleCount: 2, EligibleProtocolsCount: 5, EligibleNetworksCount: 1,
	}, s.OpportunityCounts)
	assert.Equal(t, 5, s.YieldOpportunitiesCount)
	requireDec(t, "80.158", s.MaxReportedAPYPercent)
	requireDec(t, "44.2418", s.AverageReportedAPYPercent)
	requireDec(t, "43.899", s.MedianReportedAPYPercent)
	assert.Equal(t, "search", s.Coverage)
	assert.Equal(t, "2026-10-08T21:11:23Z", *s.OldestFetchedAt)
	assert.Equal(t, "2026-10-08T21:11:23Z", *s.NewestFetchedAt)
	assert.Len(t, e.Opportunities, 17, "16 confirmed positions plus 1 unconfirmed candidate")

	require.Len(t, e.Underlyings, 1)
	u := e.Underlyings[0]
	assert.Equal(t, "US67066G1040", u.ID)
	assert.Equal(t, []int64{42461, 36992, 37270, 38093}, u.TokenIDs, "ranked")
	assert.Equal(t, []int64{38153}, u.ExcludedTokenIDs)
	assert.Equal(t, 4, u.VersionsCount)
	assert.Equal(t, 3, u.IssuersCount)
	assert.Equal(t, 1, u.TokensInDefi)
	assert.Equal(t, []string{"Beefy", "aerodrome-slipstream", "beefy-clm", "uniswap-v3", "uniswap-v4"}, u.EligiblePlatform)
	assert.Equal(t, "search", u.Coverage)
	assert.Equal(t, []string{"NVDA"}, u.SearchTerms)
	assert.True(t, u.SearchComplete)
	assert.Equal(t, 2, *u.CMCRwaID)
	requireDec(t, "151803344.10364377", u.TokenizedMarketCapUSD)
	requireDec(t, "90664811.44390228", u.TokenizedVolume24hUSD)
	requireDec(t, "231.21263591548592", u.AverageTokenizedPriceUSD)
	assert.Equal(t, 7, u.EligibleOpportunitiesCount)
	assert.Equal(t, 16, u.RawConfirmedOpportunitiesCount)
}

func TestFixtureTokens(t *testing.T) {
	e := &loadFixture(t).Export
	type want struct {
		kind, coverage        string
		rank                  int // 0: excluded, omitted
		raw, eligible, direct int
		unconfirmed           int
		yield                 string
	}
	for id, w := range map[int64]want{
		42461: {"tokenized", "search", 1, 13, 7, 5, 1, "reported"},
		36992: {"tokenized", "search", 2, 2, 0, 0, 0, "unknown"},
		37270: {"wrapper", "partial", 3, 0, 0, 0, 0, "unknown"},
		38093: {"tokenized", "search", 4, 1, 0, 0, 0, "unknown"},
		38153: {"derivative", "no_contracts", 0, 0, 0, 0, 0, "unknown"},
	} {
		tok := tokenByID(t, e, id)
		assert.Equal(t, w.kind, tok.Kind, id)
		assert.Equal(t, w.coverage, tok.Coverage, id)
		if w.rank == 0 {
			assert.Nil(t, tok.RankInUnderlying, id)
		} else {
			require.NotNil(t, tok.RankInUnderlying, id)
			assert.Equal(t, w.rank, *tok.RankInUnderlying, id)
		}
		assert.Equal(t, w.raw, tok.Counts.RawConfirmedOpportunitiesCount, id)
		assert.Equal(t, w.eligible, tok.Counts.EligibleOpportunitiesCount, id)
		assert.Equal(t, w.eligible, tok.UsefulnessScore, id)
		assert.Equal(t, w.direct, tok.Counts.DirectEligibleCount, id)
		assert.Equal(t, w.unconfirmed, tok.Counts.UnconfirmedCandidatesCount, id)
		assert.Equal(t, w.unconfirmed, len(tok.UnconfirmedKeys), id)
		assert.Equal(t, w.yield, tok.Yield.Status, id)
	}
	assert.Equal(t, "derivative", *tokenByID(t, e, 38153).ExcludedReason)

	tok := tokenByID(t, e, 42461) // NVDAc
	assert.Equal(t, 2, tok.Counts.IndirectEligibleCount)
	assert.Equal(t, 5, tok.Counts.EligibleProtocolsCount)
	assert.Equal(t, 1, tok.Counts.EligibleNetworksCount)
	assert.Equal(t, []string{"base:0xb4b42dd66cef25614b94510a910d54b2c148e7621d22b4271566724beda63d13"}, tok.UnconfirmedKeys, "morpho market names only the loan token")
	assert.Len(t, tok.Relations, 13)
	assert.Equal(t, 9, countDepth(tok.Relations, 1))
	assert.Equal(t, 4, countDepth(tok.Relations, 2))
	assert.Equal(t, model.Relation{Key: "base:0x3d6c5caafa8b9ba427bb0c9b21d52dd15fafcea3c8111dd8142681eb781d0980", Depth: 1,
		Path: []string{"base:0xb20000000000000000000078ee7ce2fe4908108c", "base:0x3d6c5caafa8b9ba427bb0c9b21d52dd15fafcea3c8111dd8142681eb781d0980"}}, tok.Relations[0], "sorted by (depth, key)")
	assert.Equal(t, 2, tok.Relations[len(tok.Relations)-1].Depth)

	assert.Equal(t, []string{"base"}, tok.ByNetwork.Keys)
	assert.Equal(t, model.BreakdownCount{Raw: 13, Eligible: 7}, tok.ByNetwork.Values["base"])
	assert.Equal(t, []string{"dex_lp", "vault", "yield_market"}, tok.ByCategory.Keys)
	assert.Equal(t, model.BreakdownCount{Raw: 7, Eligible: 4}, tok.ByCategory.Values["dex_lp"])
	assert.Equal(t, model.BreakdownCount{Raw: 4, Eligible: 3}, tok.ByCategory.Values["vault"])
	assert.Equal(t, model.BreakdownCount{Raw: 2, Eligible: 0}, tok.ByCategory.Values["yield_market"])
	assert.Equal(t, []string{"uniswap-v4", "beefy-clm", "aerodrome-slipstream", "beefy", "uniswap-v3", "harvest-finance", "hydrex", "spectra", "spectra-pt"}, tok.ByPlatform.Keys)
	assert.Equal(t, model.BreakdownCount{Raw: 4, Eligible: 2}, tok.ByPlatform.Values["uniswap-v4"])

	y := tok.Yield
	assert.Equal(t, 5, y.YieldOpportunitiesCount)
	requireDec(t, "80.158", y.MaxReportedAPYPercent)
	assert.Equal(t, "base:0x853f5f1b92b16714fe6cda67caad0856b83c7ab9", *y.MaxReportedAPYKey)
	requireDec(t, "44.2418", y.AverageReportedAPYPercent)
	requireDec(t, "43.899", y.MedianReportedAPYPercent)
	assert.Equal(t, []string{
		"base:0x853f5f1b92b16714fe6cda67caad0856b83c7ab9", "base:0x0cbe68943519ebbeb024f20e85db39f6b51a29ed",
		"base:0x32bfa96c4cf3322cd50ca0c980f2378082da681f", "base:0xb21769526e3a0814752628ec6c6e08efc883a025a18c64ee3ad376aa67728a18",
		"base:0x60661b315553eb81872deea9a66d567cf0ccd33b"}, y.TopKeys)
	assert.Equal(t, []string{"dex_lp", "vault"}, y.ByCategory.Keys)
	assert.Equal(t, 3, y.ByCategory.Values["dex_lp"].Count)
	requireDec(t, "80.158", y.ByCategory.Values["dex_lp"].Max)
	requireDec(t, "42.8547", y.ByCategory.Values["dex_lp"].Average)
	requireDec(t, "25.439", y.ByCategory.Values["dex_lp"].Median)
	assert.Equal(t, 2, y.ByCategory.Values["vault"].Count)
	requireDec(t, "48.746", y.ByCategory.Values["vault"].Max)
	requireDec(t, "46.3225", y.ByCategory.Values["vault"].Average)
	requireDec(t, "46.3225", y.ByCategory.Values["vault"].Median, "even count: mean of the two middles")
	assert.Equal(t, 0, y.MissingAPYCount)
	assert.Equal(t, 0, y.AnomalyCount)

	// deployments of NVDAX: status per network and the Portals facts of the indexed one
	nvdax := tokenByID(t, e, 36992)
	statuses := map[string]string{}
	for _, d := range nvdax.Deployments {
		statuses[d.CMCNetwork] = d.Status
	}
	assert.Equal(t, map[string]string{"Solana": "unsupported_network", "BNB Smart Chain (BEP20)": "not_checked", "Ethereum": "indexed",
		"Ink": "unsupported_network", "TON": "unsupported_network", "X Layer": "unsupported_network"}, statuses)
	for _, d := range nvdax.Deployments {
		if d.Status == "indexed" {
			assert.Equal(t, "NVDAX", *d.PortalsSymbol)
			assert.Equal(t, 18, *d.Decimals)
			assert.Equal(t, "xstocks", *d.PortalsPlatform)
		}
	}
	for _, d := range nvdax.Deployments {
		if d.CMCNetwork == "Solana" {
			assert.Equal(t, "Xsc9qvGR1efVDFGLrVsmkzv3qi45LTBjeUKSPmx9qEh", d.Address, "non-EVM case preserved")
		}
	}
}

func countDepth(rels []model.Relation, depth int) int {
	n := 0
	for _, r := range rels {
		if r.Depth == depth {
			n++
		}
	}
	return n
}

func TestFixtureIssuers(t *testing.T) {
	e := &loadFixture(t).Export
	require.Len(t, e.Issuers, 3)
	// (-score, -protocols, name): Backed and Ondo tie at 0/0 and are ordered by name
	assert.Equal(t, []string{"Coinbase", "Backed Assets", "Ondo Assets"}, []string{*e.Issuers[0].Name, *e.Issuers[1].Name, *e.Issuers[2].Name})

	c := e.Issuers[0]
	assert.Equal(t, "6a72fe7072996604d45c6733", c.ID)
	assert.Equal(t, "unknown", c.MetadataStatus)
	assert.Nil(t, c.RosterSize)
	assert.Equal(t, 7, c.IssuerUsefulnessScore)
	assert.Equal(t, 13, c.RawConfirmedOpportunitiesCount)
	assert.Equal(t, []int64{42461}, c.TokenIDs)
	assert.Equal(t, []string{"US67066G1040"}, c.UnderlyingIDs)
	assert.Equal(t, 1, c.TokensInDefi)
	assert.Equal(t, 1, c.DeploymentsCount)
	assert.Equal(t, 1, c.SupportedDeploymentsCount)
	assert.Equal(t, 0, c.UnsupportedDeploymentCount)
	assert.Equal(t, []string{"base:0x853f5f1b92b16714fe6cda67caad0856b83c7ab9", "base:0x0cbe68943519ebbeb024f20e85db39f6b51a29ed", "base:0x32bfa96c4cf3322cd50ca0c980f2378082da681f"}, c.BestReportedYieldKeys, "top 3")
	assert.Equal(t, []string{"base"}, c.ByNetwork.Keys)
	assert.Equal(t, []string{"dex_lp", "vault", "yield_market"}, c.ByCategory.Keys)
	assert.Equal(t, []string{"NVDA"}, c.ByUnderlying.Keys)
	assert.Equal(t, 7, c.ByUnderlying.Values["NVDA"])
	requireDec(t, "7", c.MeanOpportunitiesPerUnderlying)
	requireDec(t, "7", c.MeanOpportunitiesPerToken)
	requireDec(t, "100", c.BasketRelativeScore)
	assert.Equal(t, 1, c.ComparedUnderlyingsCount)
	requireDec(t, "2099106.17", c.DirectPoolsLiquidityUSD)
	requireDec(t, "5956483.22", c.BasketMarketCapUSD)
	assert.Equal(t, "2026-10-08T21:11:23Z", *c.OldestFetchedAt)

	b := e.Issuers[1]
	assert.Equal(t, 2, b.BasketTokensCount)
	assert.Equal(t, 8, b.DeploymentsCount)
	assert.Equal(t, 1, b.SupportedDeploymentsCount)
	assert.Equal(t, 5, b.UnsupportedDeploymentCount)
	assert.Equal(t, 2, b.RawConfirmedOpportunitiesCount)
	assert.Equal(t, 0, b.IssuerUsefulnessScore)
	assert.True(t, b.DirectPoolsLiquidityUSD.IsNull(), "no eligible direct pool: null, not 0")
	assert.True(t, b.MaxReportedAPYPercent.IsNull())
	requireDec(t, "0", b.BasketRelativeScore, "compared and scored 0")
	requireDec(t, "45367616", b.BasketMarketCapUSD)
	assert.Equal(t, []string{"ethereum"}, b.ByNetwork.Keys)
	assert.Equal(t, []string{"wrapper"}, b.ByCategory.Keys)
	assert.Equal(t, []int64{36992, 37270}, b.TokenIDs)
}

func TestIssuerMetaFromCache(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "snapshots", "nvda_snapshot.json"))
	require.NoError(t, err)
	snap, err := model.ParseSnapshot(b)
	require.NoError(t, err)
	settings, err := model.LoadSettings(filepath.Join("..", "..", "config", "settings.json"))
	require.NoError(t, err)
	cache := map[string]IssuerMeta{
		"6a72fe7072996604d45c6733": {Name: ptr("Coinbase Tokenized"), Website: ptr("https://c.example"), RosterSize: ptr(12)},
		"6878977dcbbf471de3366e85": {},
	}
	res, err := Build([]*model.Snapshot{snap}, cache, settings, nil)
	require.NoError(t, err)
	byID := map[string]model.Issuer{}
	for _, i := range res.Export.Issuers {
		byID[i.ID] = i
	}
	c := byID["6a72fe7072996604d45c6733"]
	assert.Equal(t, "cmc", c.MetadataStatus)
	assert.Equal(t, "Coinbase Tokenized", *c.Name)
	assert.Equal(t, "Coinbase", *c.CMCLabel, "the CMC label stays")
	assert.Equal(t, 12, *c.RosterSize)
	assert.Equal(t, "unknown", byID["6878977dcbbf471de3366e85"].MetadataStatus, "an empty cache entry counts as absent")
	assert.Contains(t, res.Side.Issuers, "6a72fe7072996604d45c6733")
}

// linkedSnapshot builds a snapshot whose token (crypto id cid) has one indexed deployment and one position (parent)
// containing it, with the given liquidity.
func linkedSnapshot(t *testing.T, id, ticker string, cid int64, fetched, liquidity string) *model.Snapshot {
	t.Helper()
	root := "base:0x" + strings.Repeat("a", 40)
	parent := "base:0x" + strings.Repeat("b", 40)
	ca := model.CMCContractAddress{ContractAddress: "0x" + strings.Repeat("a", 40)}
	ca.Platform = &struct {
		Name *string `json:"name"`
	}{Name: ptr("Base")}
	return &model.Snapshot{
		UnderlyingID: id, Ticker: ticker, Name: ticker, AssetType: "stock", Status: "ok", FetchedAt: mustTime(t, fetched),
		CMC: &model.SnapshotCMC{
			RWA:  &model.CMCRwa{Tokens: []model.CMCToken{{CryptoID: ptr(cid), Symbol: ptr(ticker + "x")}}},
			Info: map[string]model.CMCInfo{strconv.FormatInt(cid, 10): {ContractAddress: []model.CMCContractAddress{ca}}},
		},
		Portals: &model.SnapshotPortals{
			Networks: []string{"base"}, LookedUp: []string{root},
			Exact: map[string]model.PortalsRecord{root: {Key: root, Network: ptr("base"), Address: root[5:]}},
			Records: map[string]model.PortalsRecord{parent: {
				Key: parent, Network: ptr("base"), Platform: ptr("beefy"), Address: parent[5:], Symbol: ptr("P"), Tokens: []string{root},
				Liquidity: lenient(liquidity), UpdatedAt: ptr(fetched), Metrics: &model.PortalsMetrics{APY: lenient("5")},
			}},
		},
	}
}

func dupResult(t *testing.T, oldAt, newAt string) *Result {
	t.Helper()
	res, err := Build([]*model.Snapshot{
		linkedSnapshot(t, "OLD", "AAA", 7, oldAt, "11000"),
		linkedSnapshot(t, "NEW", "AAA", 7, newAt, "12000"),
	}, nil, testSettings(), nil)
	require.NoError(t, err)
	return res
}

func TestBuildSortsSnapshotsAndNewestWins(t *testing.T) {
	s := testSettings()
	parent := "base:0x" + strings.Repeat("b", 40)
	old := linkedSnapshot(t, "A", "AAA", 1, "2026-10-01T00:00:00Z", "11000") // eligible on its own
	newer := linkedSnapshot(t, "BB", "BBB", 2, "2026-10-02T00:00:00Z", "5000")
	res, err := Build([]*model.Snapshot{newer, old}, nil, s, nil)
	require.NoError(t, err)
	assert.Equal(t, "A", res.Export.Underlyings[0].ID, "sorted by fetched_at")
	assert.Equal(t, "2026-10-01T00:00:00Z", *res.Export.Summary.OldestFetchedAt)
	assert.Equal(t, "2026-10-02T00:00:00Z", *res.Export.Summary.NewestFetchedAt)
	// the position shared by both snapshots is supplied by the newest one: its liquidity, eligibility and timestamp
	require.Len(t, res.Export.Opportunities, 1)
	o := res.Export.Opportunities[0]
	assert.Equal(t, parent, o.Key)
	requireDec(t, "5000", o.LiquidityUSD)
	assert.Equal(t, "low_liquidity", o.Eligibility, "the older snapshot would have said eligible")
	assert.Equal(t, "2026-10-02T00:00:00Z", *o.UpdatedAt)
	assert.Equal(t, newer.FetchedAt, res.Side.PositionFetchedAt[parent])
	// and the older underlying's token sees the newest record too
	assert.Equal(t, 0, tokenByID(t, &res.Export, 1).Counts.EligibleOpportunitiesCount)
	assert.Equal(t, 1, tokenByID(t, &res.Export, 1).Counts.RawConfirmedOpportunitiesCount)

	// ties keep the input order
	tie1 := linkedSnapshot(t, "A", "AAA", 1, "2026-10-01T00:00:00Z", "1")
	tie2 := linkedSnapshot(t, "BB", "BBB", 2, "2026-10-01T00:00:00Z", "1")
	res, err = Build([]*model.Snapshot{tie2, tie1}, nil, s, nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"BB", "A"}, []string{res.Export.Underlyings[0].ID, res.Export.Underlyings[1].ID})
	assert.Equal(t, tie2.FetchedAt, res.Side.PositionFetchedAt[parent], "tie: the later in sorted order (input order) overwrites")
}

func TestFinalize(t *testing.T) {
	res := loadFixture(t)
	now, _ := ParseTime("2026-11-01T10:00:00Z")
	info := map[string]RwaInfo{"2": {Website: ptr("https://www.nvidia.com")}}
	logos := MapLogos{"tokens/42461": "png", "issuers/6a72fe7072996604d45c6733": "jpg", "underlyings/US67066G1040": "svg"}
	e := Finalize(res, now, info, logos)
	assert.Equal(t, 1, e.SchemaVersion)
	assert.Equal(t, "2026-11-01T10:00:00Z", e.GeneratedAt)
	assert.Nil(t, e.BuildID)
	assert.Nil(t, e.RunID)
	assert.Equal(t, "https://www.nvidia.com", *e.Underlyings[0].Website)
	assert.Equal(t, "logos/underlyings/US67066G1040.svg", *e.Underlyings[0].Logo)
	assert.Equal(t, "logos/tokens/42461.png", *tokenByID(t, e, 42461).Logo)
	assert.Nil(t, tokenByID(t, e, 36992).Logo, "no file, no logo (CMC URLs are never used)")
	assert.Equal(t, "logos/issuers/6a72fe7072996604d45c6733.jpg", *e.Issuers[0].Logo)
	assert.Nil(t, e.Issuers[1].Logo)

	// no rwa_info entry: null website; nil resolver: no logos at all
	e = Finalize(res, now, nil, nil)
	assert.Nil(t, e.Underlyings[0].Website)
	assert.Nil(t, e.Underlyings[0].Logo)
}
