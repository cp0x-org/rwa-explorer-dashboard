package model

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const configDir = "../../config"

func TestLoadAssets(t *testing.T) {
	assets, err := LoadAssets(filepath.Join(configDir, "assets.json"))
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(assets), 100)
	var nvda *Asset
	for i := range assets {
		if assets[i].Ticker == "NVDA" {
			nvda = &assets[i]
		}
	}
	require.NotNil(t, nvda)
	assert.Equal(t, "US67066G1040", nvda.ID())
	assert.Equal(t, "stock", nvda.AssetType)
	assert.True(t, nvda.Enabled)
}

func TestLoadSettings(t *testing.T) {
	s, err := LoadSettings(filepath.Join(configDir, "settings.json"))
	require.NoError(t, err)
	assert.Equal(t, 60, s.RequestsPerMinute.Portals)
	assert.Equal(t, 20, s.RequestsPerMinute.CMC)
	assert.Equal(t, "10000", s.MinEligibleLiquidityUSD.String())
	assert.Equal(t, 48, s.MaxPositionAgeHours)

	v, ok := s.ChainMapping["Solana"]
	assert.True(t, ok, "Solana is a known network")
	assert.Nil(t, v, "Solana is known but unsupported")
	v, ok = s.ChainMapping["Ethereum"]
	require.True(t, ok)
	require.NotNil(t, v)
	assert.Equal(t, "ethereum", *v)
	_, ok = s.ChainMapping["Nowhere"]
	assert.False(t, ok, "missing key means unmapped")

	assert.Equal(t, "wrapper", s.PlatformCategories["xstocks"])
	require.NotEmpty(t, s.TagCategories)
	assert.Equal(t, [2]string{"#lending", "lending_supply"}, s.TagCategories[0])
	assert.Equal(t, "1000", s.ExportSettings().YieldAPYReviewThresholdPercent.String())
}

func TestISINIsValid(t *testing.T) {
	for _, ok := range []string{"US67066G1040", "US0378331005", "US5949181045", "US0231351067", "US02079K3059", "IE000S9YS762"} {
		assert.True(t, ISINIsValid(ok), ok)
	}
	for _, bad := range []string{"US67066G1041", "US0378331006", "US037833100", "US03783310055", "us0378331005", "1S0378331005", "US037833100A", ""} {
		assert.False(t, ISINIsValid(bad), bad)
	}
}

func TestAssetIDAndStorageID(t *testing.T) {
	isin := "US67066G1040"
	assert.Equal(t, isin, Asset{Ticker: "NVDA", AssetType: "stock", ISIN: &isin}.ID())
	a := Asset{Ticker: "ABC.X", AssetType: "etf"}
	assert.Equal(t, "etf:ticker:ABC.X", a.ID())
	assert.Equal(t, "etf_ticker_ABC.X", a.StorageID())
	assert.Equal(t, "US67066G1040", StorageID(isin))
}

func writeAssets(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "assets.json")
	require.NoError(t, os.WriteFile(p, []byte(body), 0o600))
	return p
}

func TestLoadAssetsValidation(t *testing.T) {
	cases := map[string]string{
		"no ticker":      `{"assets":[{"asset_type":"stock"}]}`,
		"bad type":       `{"assets":[{"ticker":"A","asset_type":"bond"}]}`,
		"bad isin":       `{"assets":[{"ticker":"A","asset_type":"stock","isin":"US67066G1041"}]}`,
		"duplicate tick": `{"assets":[{"ticker":"a","asset_type":"stock"},{"ticker":"A","asset_type":"etf"}]}`,
		"duplicate isin": `{"assets":[{"ticker":"A","asset_type":"stock","isin":"US67066G1040"},{"ticker":"B","asset_type":"stock","isin":"US67066G1040"}]}`,
	}
	for name, body := range cases {
		_, err := LoadAssets(writeAssets(t, body))
		assert.Error(t, err, name)
	}
	got, err := LoadAssets(writeAssets(t, `{"assets":[{"ticker":" nvda ","asset_type":"stock","enabled":false}]}`))
	require.NoError(t, err)
	assert.Equal(t, Asset{Ticker: "NVDA", Name: "NVDA", AssetType: "stock", Enabled: false}, got[0])
}
