package compute

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/model"
)

// Rules of the PR #3 pipeline changes: display names, symbol aliases, search coverage. They mirror
// rwa-data-pipeline/tests/test_analysis_fixes.py and test_refresh_guards.py.

// fixtureMap reads the recorded NVDA snapshot as a mutable document.
func fixtureMap(t *testing.T) map[string]any {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "snapshots", "nvda_snapshot.json"))
	require.NoError(t, err)
	var doc map[string]any
	require.NoError(t, json.Unmarshal(b, &doc))
	return doc
}

func snapshotOf(t *testing.T, doc map[string]any) *model.Snapshot {
	t.Helper()
	b, err := json.Marshal(doc)
	require.NoError(t, err)
	s, err := model.ParseSnapshot(b)
	require.NoError(t, err)
	return s
}

func realSettings(t *testing.T) *model.Settings {
	t.Helper()
	s, err := model.LoadSettings(filepath.Join("..", "..", "config", "settings.json"))
	require.NoError(t, err)
	return s
}

func tokenDoc(doc map[string]any, match func(t map[string]any) bool) map[string]any {
	for _, tk := range doc["cmc"].(map[string]any)["rwa"].(map[string]any)["tokens"].([]any) {
		if match(tk.(map[string]any)) {
			return tk.(map[string]any)
		}
	}
	return nil
}

func TestDisplayNamesCarryNoLineBreaksOrDoubleSpaces(t *testing.T) {
	doc := fixtureMap(t)
	tk := tokenDoc(doc, func(map[string]any) bool { return true })
	tk["name"] = "  NVIDIA  Tokenized\nStock "
	tk["issuer_name"] = "Acme \n Assets"
	id := int64(tk["crypto_id"].(float64))
	var recordKey string
	for k, rec := range doc["portals"].(map[string]any)["records"].(map[string]any) {
		rec.(map[string]any)["name"] = "Uniswap  V3\tNVDA /  USDC "
		recordKey = k
		break
	}
	doc["name"] = " NVIDIA\n Corp "
	doc["cmc"].(map[string]any)["rwa"].(map[string]any)["name"] = "NVIDIA  (Tokenized)\n"

	res, err := Build([]*model.Snapshot{snapshotOf(t, doc)}, nil, realSettings(t), nil)
	require.NoError(t, err)
	var built *model.Token
	for i := range res.Export.Tokens {
		if res.Export.Tokens[i].ID == id {
			built = &res.Export.Tokens[i]
		}
	}
	require.NotNil(t, built)
	assert.Equal(t, "NVIDIA Tokenized Stock", *built.Name)
	assert.Equal(t, "Acme Assets", *built.IssuerName)
	for _, o := range res.Export.Opportunities {
		if o.Key == NormKey(recordKey) {
			assert.Equal(t, "Uniswap V3 NVDA / USDC", *o.Name)
		}
	}
	assert.Equal(t, "NVIDIA Corp", res.Export.Underlyings[0].Name)
	assert.Equal(t, "NVIDIA (Tokenized)", *res.Export.Underlyings[0].CMCName)
	collapsed := func(s *string) {
		if s != nil {
			assert.Equal(t, model.CollapseSpace(*s), *s)
		}
	}
	for _, tk := range res.Export.Tokens {
		collapsed(tk.Name)
		collapsed(tk.IssuerName)
	}
	for _, o := range res.Export.Opportunities {
		collapsed(o.Name)
	}
	for _, is := range res.Export.Issuers {
		collapsed(is.Name)
		collapsed(is.CMCLabel)
	}
}

func TestIssuerNameFromCacheIsNormalizedToo(t *testing.T) {
	doc := fixtureMap(t)
	tk := tokenDoc(doc, func(map[string]any) bool { return true })
	issuerID := tk["issuer_id"].(string)
	cache := map[string]IssuerMeta{issuerID: {Name: ptr("  Cache \n Name ")}}
	res, err := Build([]*model.Snapshot{snapshotOf(t, doc)}, cache, realSettings(t), nil)
	require.NoError(t, err)
	for _, is := range res.Export.Issuers {
		if is.ID == issuerID {
			assert.Equal(t, "Cache Name", *is.Name)
		}
	}
}

func TestAnAliasKeepsARealVersionWhoseSymbolDiffersFromTheTicker(t *testing.T) {
	for _, tk := range []model.CMCToken{
		{Symbol: ptr("tSpaceX"), Name: ptr("T-SpaceX tokenized stock (Tessera)"), IssuerName: ptr("Tessera")},
		{Symbol: ptr("SPACEX"), Name: ptr("SpaceX tokenized stock (PreStocks)"), IssuerName: ptr("PreStocks")},
	} {
		kind, reason := ClassifyToken(tk, "SPCX")
		assert.Equal(t, "unclassified", kind)
		assert.Equal(t, "symbol_does_not_match_ticker", *reason)
		kind, reason = ClassifyToken(tk, "SPCX", "SPACEX")
		assert.Equal(t, "tokenized", kind)
		assert.Nil(t, reason)
	}
	// an alias changes nothing for derivatives and wrappers
	kind, _ := ClassifyToken(model.CMCToken{Symbol: ptr("SPACEX"), Name: ptr("SpaceX (Derivatives)")}, "SPCX", "SPACEX")
	assert.Equal(t, "derivative", kind)
	kind, _ = ClassifyToken(model.CMCToken{Symbol: ptr("wSPACEX"), Name: ptr("Wrapped SpaceX")}, "SPCX", "SPACEX")
	assert.Equal(t, "wrapper", kind)
	// a symbol that matches neither stays out
	kind, reason := ClassifyToken(model.CMCToken{Symbol: ptr("GOOGon"), Name: ptr("Alphabet Class C")}, "SPCX", "SPACEX")
	assert.Equal(t, "unclassified", kind)
	assert.Equal(t, "symbol_does_not_match_ticker", *reason)
	// other share classes stay out
	kind, _ = ClassifyToken(model.CMCToken{Symbol: ptr("BRK.A.D"), Name: ptr("Berkshire Hathaway Class A")}, "BRK.B")
	assert.Equal(t, "unclassified", kind)
}

// aliasedSnapshot renames the symbol of NVDAc (indexed contracts) to one without the ticker, so that only an alias
// keeps it as a version.
func aliasedSnapshot(t *testing.T, searchedAlias bool) (*model.Snapshot, int64) {
	t.Helper()
	doc := fixtureMap(t)
	tk := tokenDoc(doc, func(tk map[string]any) bool { return tk["symbol"] == "NVDAc" })
	require.NotNil(t, tk)
	tk["symbol"] = "NVIDIAC"
	if searchedAlias {
		doc["portals"].(map[string]any)["searches"].(map[string]any)["NVIDIA"] = map[string]any{"complete": true, "pages": 1, "received": 0, "total_items": 0}
	}
	return snapshotOf(t, doc), int64(tk["crypto_id"].(float64))
}

func tokenByIDRes(res *Result, id int64) model.Token {
	for _, tk := range res.Export.Tokens {
		if tk.ID == id {
			return tk
		}
	}
	return model.Token{}
}

func TestAVersionFoundThroughAnAliasIsNotSearchCoverageUntilItsSymbolIsSearched(t *testing.T) {
	snap, id := aliasedSnapshot(t, false)
	res, err := Build([]*model.Snapshot{snap}, nil, realSettings(t), nil, WithSymbolAliases(map[string][]string{"NVDA": {"NVIDIA"}}))
	require.NoError(t, err)
	tk := tokenByIDRes(res, id)
	assert.Nil(t, tk.ExcludedReason, "the alias keeps it as a version")
	assert.Equal(t, "partial", tk.Coverage, "no search could have found its positions")
	assert.False(t, res.Export.Underlyings[0].SearchComplete)
}

func TestOnceItsSymbolWasSearchedTheAliasedVersionCountsAsSearched(t *testing.T) {
	snap, id := aliasedSnapshot(t, true)
	res, err := Build([]*model.Snapshot{snap}, nil, realSettings(t), nil, WithSymbolAliases(map[string][]string{"NVDA": {"NVIDIA"}}))
	require.NoError(t, err)
	assert.Equal(t, "search", tokenByIDRes(res, id).Coverage)
	assert.True(t, res.Export.Underlyings[0].SearchComplete)
}

func TestVersionsWhoseSymbolHoldsASearchedTermKeepTheirCoverage(t *testing.T) {
	res, err := Build([]*model.Snapshot{snapshotOf(t, fixtureMap(t))}, nil, realSettings(t), nil)
	require.NoError(t, err)
	coverage := map[string]string{}
	for _, tk := range res.Export.Tokens {
		if tk.ExcludedReason == nil {
			coverage[*tk.Symbol] = tk.Coverage
		}
	}
	assert.Equal(t, map[string]string{"NVDAX": "search", "NVDAon": "search", "WNVDAX": "partial", "NVDAc": "search"}, coverage)
	assert.True(t, res.Export.Underlyings[0].SearchComplete)
}

func TestASearchedTermCoversASymbolInAnyLetterCase(t *testing.T) {
	snap, id := aliasedSnapshot(t, false)
	snap.Portals.Searches["Nvidia"] = model.PortalsSearch{Complete: true}
	res, err := Build([]*model.Snapshot{snap}, nil, realSettings(t), nil, WithSymbolAliases(map[string][]string{"NVDA": {"NVIDIA"}}))
	require.NoError(t, err)
	assert.Equal(t, "search", tokenByIDRes(res, id).Coverage)
	assert.True(t, res.Export.Underlyings[0].SearchComplete)
}

// Only "search" coverage is downgraded: an unsearched symbol that Portals cannot index keeps its own coverage.
func TestAVersionPortalsCannotIndexKeepsItsCoverage(t *testing.T) {
	doc := fixtureMap(t)
	tk := tokenDoc(doc, func(tk map[string]any) bool { return tk["symbol"] == "NVDAc" })
	tk["symbol"] = "NVIDIAC"
	// drop every deployment of the token: nothing to look up, coverage no_contracts, and the symbol is unsearched
	info := doc["cmc"].(map[string]any)["info"].(map[string]any)[strconvID(tk)].(map[string]any)
	info["contract_address"] = []any{}
	info["platform"] = nil
	res, err := Build([]*model.Snapshot{snapshotOf(t, doc)}, nil, realSettings(t), nil, WithSymbolAliases(map[string][]string{"NVDA": {"NVIDIA"}}))
	require.NoError(t, err)
	assert.Equal(t, "no_contracts", tokenByIDRes(res, int64(tk["crypto_id"].(float64))).Coverage)
	assert.False(t, res.Export.Underlyings[0].SearchComplete, "its symbol was never searched")
}

func strconvID(tk map[string]any) string {
	return json.Number(string(mustMarshal(tk["crypto_id"]))).String()
}

func mustMarshal(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

func TestSymbolAliasesLoaded(t *testing.T) {
	assets, err := model.LoadAssets(filepath.Join("..", "..", "config", "assets.json"))
	require.NoError(t, err)
	aliases := SymbolAliases(assets)
	assert.Equal(t, []string{"SPACEX"}, aliases["SPCX"])
	assert.Empty(t, aliases["NVDA"])
}

func TestMantleAndMonadAreKnownNetworksThatPortalsDoesNotIndex(t *testing.T) {
	mapping := realSettings(t).ChainMapping
	for _, name := range []string{"Mantle", "Monad"} {
		v, ok := mapping[name]
		assert.True(t, ok, name)
		assert.Nil(t, v, name)
	}
	address := "0x" + "ab"
	_ = address
	deps := ParseDeployments(&model.CMCInfo{ContractAddress: []model.CMCContractAddress{
		{ContractAddress: "0x" + "ab" + "cdef0123456789abcdef0123456789abcdef01", Platform: &struct {
			Name *string `json:"name"`
		}{Name: ptr("Mantle")}},
	}}, mapping)
	require.Len(t, deps, 1)
	assert.Equal(t, "unsupported_network", deps[0].Status, "known but not indexed, not 'unmapped'")
}

func TestSymbolSearchedUsesCasefold(t *testing.T) {
	sym := func(s string) *string { return &s }
	term := func(t string) map[string]model.PortalsSearch { return map[string]model.PortalsSearch{t: {}} }
	assert.True(t, symbolSearched(sym("NVDAx"), term("nvda")))
	assert.True(t, symbolSearched(sym("Straße"), term("STRASSE")), "ß folds to ss like str.casefold")
	assert.True(t, symbolSearched(sym("STRASSE"), term("straße")))
	assert.False(t, symbolSearched(sym("TSLA"), term("nvda")))
	assert.False(t, symbolSearched(nil, term("nvda")))
}

// test_links.py::test_raw_records_come_from_the_newest_snapshot_whatever_the_asset_order
func TestPositionMetadataComesFromTheNewestSnapshotWhateverTheOrder(t *testing.T) {
	mk := func(id, fetchedAt, tag string) *model.Snapshot {
		doc := fixtureMap(t)
		doc["underlying_id"], doc["fetched_at"] = id, fetchedAt
		for _, section := range []string{"records", "exact"} {
			recs, _ := doc["portals"].(map[string]any)[section].(map[string]any)
			for _, rec := range recs {
				rec.(map[string]any)["metadata"] = map[string]any{"tag": tag}
			}
		}
		return snapshotOf(t, doc)
	}
	older, newer := mk("AAA", "2026-10-01T00:00:00Z", "old"), mk("ZZZ", "2026-10-18T00:00:00Z", "new")
	for name, order := range map[string][]*model.Snapshot{"old first": {older, newer}, "new first": {newer, older}} {
		res, err := Build(order, nil, realSettings(t), nil)
		require.NoError(t, err, name)
		require.NotEmpty(t, res.Side.PositionMetadata, name)
		for key, md := range res.Side.PositionMetadata {
			assert.Equal(t, "new", md["tag"], "%s: %s", name, key)
		}
	}
}
