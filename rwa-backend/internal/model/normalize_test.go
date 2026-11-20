package model

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// arrayFields lists the array-typed fields of rwa.ts per entity (dotted paths for nested ones).
var arrayFields = map[string][]string{
	"underlyings":   {"token_ids", "excluded_token_ids", "eligible_platforms", "search_terms"},
	"tokens":        {"deployments", "relations", "unconfirmed_keys", "yield.top_keys"},
	"issuers":       {"underlying_ids", "token_ids", "eligible_platforms", "best_reported_yield_keys"},
	"opportunities": {"tags", "components", "flags"},
}

func lookup(m map[string]any, path string) (any, bool) {
	var cur any = m
	for _, p := range splitDot(path) {
		mm, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		if cur, ok = mm[p]; !ok {
			return nil, false
		}
	}
	return cur, true
}

func splitDot(s string) []string {
	var out []string
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == '.' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	return out
}

func TestExportMarshalNeverEmitsNullArrays(t *testing.T) {
	e := Export{
		Underlyings:   []Underlying{{}},
		Tokens:        []Token{{Relations: []Relation{{}}}},
		Issuers:       []Issuer{{}},
		Opportunities: []Opportunity{{}},
	}
	out, err := json.Marshal(e)
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(out, &got))
	for entity, fields := range arrayFields {
		items, ok := got[entity].([]any)
		require.True(t, ok, entity)
		require.Len(t, items, 1)
		for _, f := range fields {
			v, ok := lookup(items[0].(map[string]any), f)
			require.True(t, ok, "%s.%s missing", entity, f)
			assert.IsType(t, []any{}, v, "%s.%s must be an array, not null", entity, f)
		}
	}
	rel := got["tokens"].([]any)[0].(map[string]any)["relations"].([]any)[0].(map[string]any)
	assert.IsType(t, []any{}, rel["path"])

	empty, err := json.Marshal(Export{})
	require.NoError(t, err)
	assert.Contains(t, string(empty), `"underlyings":[]`)
	assert.Contains(t, string(empty), `"opportunities":[]`)
}
