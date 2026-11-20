package model

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecimalMarshal(t *testing.T) {
	cases := []struct{ in, want string }{
		{`"10.503"`, `"10.503"`},
		{`12`, `"12"`},
		{`null`, `null`},
		{`1e-7`, `"0.0000001"`},
		{`"1E-7"`, `"0.0000001"`},
		{`9007199254740990`, `"9007199254740990"`},
		{`211.65546361136143`, `"211.65546361136143"`},
		{`"148973893.90937608"`, `"148973893.90937608"`},
	}
	for _, c := range cases {
		var d Decimal
		require.NoError(t, json.Unmarshal([]byte(c.in), &d), c.in)
		out, err := json.Marshal(d)
		require.NoError(t, err)
		assert.Equal(t, c.want, string(out), c.in)
	}
}

func TestDecimalInStructAndZeroValue(t *testing.T) {
	type row struct {
		A Decimal `json:"a"`
		B Decimal `json:"b"`
	}
	out, err := json.Marshal(row{A: MustDecimal("1.50")})
	require.NoError(t, err)
	assert.JSONEq(t, `{"a":"1.5","b":null}`, string(out))
	var r row
	require.NoError(t, json.Unmarshal([]byte(`{"a":3}`), &r))
	assert.False(t, r.A.IsNull())
	assert.True(t, r.B.IsNull())
}

func TestDecimalInvalid(t *testing.T) {
	var d Decimal
	assert.Error(t, json.Unmarshal([]byte(`"abc"`), &d))
	assert.Error(t, json.Unmarshal([]byte(`true`), &d))
}

func TestDecimalCmp(t *testing.T) {
	a, b := MustDecimal("10.5"), MustDecimal("10.50")
	assert.Equal(t, 0, a.Cmp(b))
	assert.Equal(t, -1, MustDecimal("2").Cmp(MustDecimal("10")))
	assert.Equal(t, -1, Decimal{}.Cmp(a))
	assert.Equal(t, 1, a.Cmp(Decimal{}))
	assert.Equal(t, 0, Decimal{}.Cmp(Decimal{}))
}

func TestOrderedMapKeepsOrder(t *testing.T) {
	in := `{"z":{"raw":1,"eligible":0},"a":{"raw":2,"eligible":1}}`
	var b Breakdown
	require.NoError(t, json.Unmarshal([]byte(in), &b))
	assert.Equal(t, []string{"z", "a"}, b.Keys)
	out, err := json.Marshal(b)
	require.NoError(t, err)
	assert.Equal(t, in, string(out))
	empty, _ := json.Marshal(Breakdown{})
	assert.Equal(t, `{}`, string(empty))
}

func TestLenientDecimal(t *testing.T) {
	cases := []struct {
		in   string
		want string // "" = null
	}{
		{`"NaN"`, ""}, {`"Infinity"`, ""}, {`"-Infinity"`, ""}, {`""`, ""}, {`" 1"`, "1"}, {`true`, ""}, {`false`, ""},
		{`824.28`, "824.28"}, {`2230344`, "2230344"}, {`"10.5"`, "10.5"}, {`null`, ""}, {`"abc"`, ""}, {`{"a":1}`, ""},
		{`[1]`, ""}, {`-3.5`, "-3.5"}, {`1e-7`, "0.0000001"},
	}
	for _, c := range cases {
		var d LenientDecimal
		require.NoError(t, json.Unmarshal([]byte(c.in), &d), c.in)
		if c.want == "" {
			assert.True(t, d.IsNull(), c.in)
		} else {
			assert.Equal(t, c.want, d.String(), c.in)
		}
	}
	// a record with an odd value still parses as a whole
	var rec PortalsMetrics
	require.NoError(t, json.Unmarshal([]byte(`{"apy":"NaN","baseApy":"12.7","volumeUsd1d":true}`), &rec))
	assert.True(t, rec.APY.IsNull())
	assert.Equal(t, "12.7", rec.BaseAPY.String())
	assert.True(t, rec.VolumeUSD1d.IsNull())
}
