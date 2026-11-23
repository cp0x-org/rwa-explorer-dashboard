package model

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func assetsFile(t *testing.T, aliases string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "assets.json")
	item := `{"ticker":"SPCX","name":"SpaceX","asset_type":"stock"`
	if aliases != "" {
		item += `,"symbol_aliases":` + aliases
	}
	require.NoError(t, os.WriteFile(path, []byte(`{"schema_version":1,"assets":[`+item+`}]}`), 0o600))
	return path
}

func TestSymbolAliasesAreReadAndChecked(t *testing.T) {
	assets, err := LoadAssets(assetsFile(t, ""))
	require.NoError(t, err)
	assert.Empty(t, assets[0].SymbolAliases)

	assets, err = LoadAssets(assetsFile(t, `["spacex", " SpaceX ", "TSPACEX"]`))
	require.NoError(t, err)
	assert.Equal(t, []string{"SPACEX", "TSPACEX"}, assets[0].SymbolAliases, "trimmed, upper-cased, de-duplicated in order")

	for _, bad := range []string{`"SPACEX"`, `[""]`, `["  "]`, `[1]`, `[null]`, `{"a":1}`, `null`} {
		_, err := LoadAssets(assetsFile(t, bad))
		require.Error(t, err, bad)
		assert.Contains(t, err.Error(), "SPCX: symbol_aliases must be a list of nonempty strings", bad)
	}

	shipped, err := LoadAssets(filepath.Join(configDir, "assets.json"))
	require.NoError(t, err)
	for _, a := range shipped {
		if a.Ticker == "SPCX" {
			assert.Equal(t, []string{"SPACEX"}, a.SymbolAliases)
		}
	}
}

func TestNormalizeNameMatchesPythonSplit(t *testing.T) {
	assert.Nil(t, NormalizeName(nil))
	for in, want := range map[string]string{
		"  NVIDIA  Tokenized\nStock ": "NVIDIA Tokenized Stock",
		"a b c　d":                     "a b c d",
		"a\x1fb":                      "a b", // str.split() treats the unit separator as whitespace
		"":                            "",
		" \t\n ":                      "",
		"plain":                       "plain",
	} {
		got := NormalizeName(&in)
		require.NotNil(t, got)
		assert.Equal(t, want, *got, in)
	}
	assert.Equal(t, "x", PyStrip(" \x1f x "))
}
