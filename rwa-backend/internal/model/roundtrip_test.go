package model

import (
	"encoding/json"
	"math/big"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const repoRoot = "../../.."

var numericString = regexp.MustCompile(`^-?\d+(\.\d+)?$`)

// normalize rewrites decimal-looking strings into a canonical form so that "10.5030" equals "10.503".
func normalize(v any) any {
	switch x := v.(type) {
	case map[string]any:
		for k, e := range x {
			x[k] = normalize(e)
		}
	case []any:
		for i, e := range x {
			x[i] = normalize(e)
		}
	case string:
		if numericString.MatchString(x) {
			d, err := decimal.NewFromString(x)
			if err == nil {
				return "dec:" + d.String()
			}
		}
	case float64:
		return "dec:" + decimal.NewFromBigRat(new(big.Rat).SetFloat64(x), 30).String()
	}
	return v
}

func TestExportRoundTrip(t *testing.T) {
	path := filepath.Join(repoRoot, "rwa-interface/public/data/rwa.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("1.x export not available: %v", err)
	}
	var e Export
	require.NoError(t, json.Unmarshal(raw, &e))
	assert.Equal(t, 125, e.Summary.UnderlyingsCount)
	assert.Len(t, e.Underlyings, 125)
	assert.NotEmpty(t, e.Tokens)
	assert.Nil(t, e.BuildID)

	out, err := json.Marshal(e)
	require.NoError(t, err)

	var want, got map[string]any
	require.NoError(t, json.Unmarshal(raw, &want))
	require.NoError(t, json.Unmarshal(out, &got))
	// 2.0 additions are null for a 1.x document.
	assert.Nil(t, got["build_id"])
	assert.Nil(t, got["run_id"])
	delete(got, "build_id")
	delete(got, "run_id")
	assert.Equal(t, normalize(want), normalize(got))

	// rank_in_underlying is omitted exactly where 1.x omits it.
	var ranked, wantRanked int
	for _, tk := range want["tokens"].([]any) {
		if _, ok := tk.(map[string]any)["rank_in_underlying"]; ok {
			wantRanked++
		}
	}
	for _, tk := range got["tokens"].([]any) {
		if _, ok := tk.(map[string]any)["rank_in_underlying"]; ok {
			ranked++
		}
	}
	assert.Equal(t, wantRanked, ranked)
	assert.Less(t, ranked, len(e.Tokens))

	// breakdown key order survives.
	var rawTok struct {
		Tokens []struct {
			ID        int64
			ByNetwork json.RawMessage `json:"by_network"`
		} `json:"tokens"`
	}
	require.NoError(t, json.Unmarshal(raw, &rawTok))
	for i, tk := range e.Tokens {
		b, err := json.Marshal(tk.ByNetwork)
		require.NoError(t, err)
		if string(rawTok.Tokens[i].ByNetwork) != string(b) {
			// whitespace-insensitive comparison of the key sequences only
			var a, c Breakdown
			require.NoError(t, json.Unmarshal(rawTok.Tokens[i].ByNetwork, &a))
			require.NoError(t, json.Unmarshal(b, &c))
			assert.Equal(t, a.Keys, c.Keys)
		}
	}
}

func readSnapshot(t *testing.T, name string) (*Snapshot, []byte) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(repoRoot, "rwa-data-pipeline/data/snapshots", name)) //nolint:gosec // test fixture path
	if err != nil {
		t.Skipf("1.x snapshots not available: %v", err)
	}
	s, err := ParseSnapshot(b)
	require.NoError(t, err, name)
	return s, b
}

func countKeys(t *testing.T, b []byte, path ...string) int {
	t.Helper()
	var cur any
	require.NoError(t, json.Unmarshal(b, &cur))
	for _, p := range path {
		cur = cur.(map[string]any)[p]
	}
	switch x := cur.(type) {
	case map[string]any:
		return len(x)
	case []any:
		return len(x)
	}
	return -1
}

func TestSnapshotNVDA(t *testing.T) {
	s, raw := readSnapshot(t, "US67066G1040.json")
	assert.Equal(t, "NVDA", s.Ticker)
	assert.Equal(t, "ok", s.Status)
	assert.Equal(t, "2026-10-18T20:16:06Z", s.FetchedAt.UTC().Format("2006-01-02T15:04:05Z"))
	require.NotNil(t, s.CMC)
	require.NotNil(t, s.CMC.RWA)
	require.NotNil(t, s.CMC.RWA.RwaID)
	assert.Equal(t, 2, *s.CMC.RWA.RwaID)
	assert.Len(t, s.CMC.RWA.Tokens, countKeys(t, raw, "cmc", "rwa", "tokens"))
	assert.Len(t, s.CMC.Info, countKeys(t, raw, "cmc", "info"))
	require.NotNil(t, s.Portals)
	assert.Len(t, s.Portals.Records, 183)
	assert.Len(t, s.Portals.Exact, 10)
	assert.Len(t, s.Portals.Searches, 1)
	assert.True(t, s.Portals.Searches["NVDA"].Complete)
	assert.Equal(t, "148973893.90937608", s.CMC.RWA.TokenizedMarketCap.String())

	info := s.CMC.Info["36992"]
	require.NotNil(t, info.Platform)
	assert.Len(t, info.ContractAddress, 6)
	var tok36992 *CMCToken
	for i := range s.CMC.RWA.Tokens {
		if s.CMC.RWA.Tokens[i].CryptoID != nil && *s.CMC.RWA.Tokens[i].CryptoID == 36992 {
			tok36992 = &s.CMC.RWA.Tokens[i]
		}
	}
	require.NotNil(t, tok36992)
	assert.Equal(t, "6878977dcbbf471de3366e85", *tok36992.IssuerID)
	assert.Equal(t, "43702741.31", tok36992.MarketCap.String())

	var withAPY, withTokens int
	for _, r := range s.Portals.Records {
		require.NotEmpty(t, r.Key)
		if r.Metrics != nil && !r.Metrics.APY.IsNull() {
			withAPY++
		}
		if len(r.Tokens) > 0 {
			withTokens++
		}
	}
	assert.Positive(t, withAPY)
	assert.Positive(t, withTokens)
}

func TestSnapshotETFAndStatuses(t *testing.T) {
	entries, err := os.ReadDir(filepath.Join(repoRoot, "rwa-data-pipeline/data/snapshots"))
	if err != nil {
		t.Skipf("1.x snapshots not available: %v", err)
	}
	statuses := map[string]int{}
	var sawETF bool
	for _, e := range entries {
		s, raw := readSnapshot(t, e.Name())
		statuses[s.Status]++
		if s.AssetType == "etf" && s.Portals != nil {
			sawETF = true
			assert.Len(t, s.Portals.Records, countKeys(t, raw, "portals", "records"), e.Name())
			assert.Len(t, s.Portals.Exact, countKeys(t, raw, "portals", "exact"), e.Name())
			assert.Len(t, s.CMC.RWA.Tokens, countKeys(t, raw, "cmc", "rwa", "tokens"), e.Name())
		}
		if s.Portals != nil {
			assert.Len(t, s.Portals.Records, countKeys(t, raw, "portals", "records"), e.Name())
		}
		if s.CMC != nil && s.CMC.RWA != nil {
			assert.Len(t, s.CMC.RWA.Tokens, countKeys(t, raw, "cmc", "rwa", "tokens"), e.Name())
		}
	}
	assert.True(t, sawETF, "at least one ETF snapshot with Portals data")
	t.Logf("snapshot statuses: %v", statuses)
}

func TestSnapshotNonOKDocument(t *testing.T) {
	// Every local 1.x snapshot is status ok, so the non-ok shapes are covered synthetically.
	for _, doc := range []string{
		`{"schema_version":1,"underlying_id":"US1","ticker":"X","name":"X","isin":null,"asset_type":"etf","run_id":"r","fetched_at":"2026-10-18T20:16:06Z","status":"not_in_cmc","cmc":null,"portals":null}`,
		`{"schema_version":1,"underlying_id":"US1","ticker":"X","name":"X","isin":"US1","asset_type":"etf","run_id":"r","fetched_at":"2026-10-18T20:16:06Z","status":"no_tracked_tokens","cmc":{"rwa":{"tokens":[]},"info":{},"invalid_ids":[]},"portals":null}`,
	} {
		s, err := ParseSnapshot([]byte(doc))
		require.NoError(t, err)
		assert.NotEqual(t, "ok", s.Status)
		assert.Nil(t, s.Portals)
	}
}
