// Package compute is a line-by-line Go port of the 1.x Python pipeline's analyze.py (and the logic of
// export.py that matters; the pipeline itself is removed from the repository): classification, composition graph, eligibility and scores over saved snapshots.
// It is pure: no I/O, no clock. Every function names its Python original; semantics (None != 0, ROUND_HALF_EVEN,
// sort orders, "newest snapshot wins") are preserved exactly, because the tests compare against the 1.x export (testdata/golden).
package compute

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/model"
)

// ScoreVersion is SCORE_VERSION.
const ScoreVersion = "eligible-count-v1"

// SentinelPrice is SENTINEL_PRICE: Portals uses 9007199254740990 as the price of non-ERC-20 pool positions.
var SentinelPrice = decimal.RequireFromString("9e15")

// UseCategories is USE_CATEGORIES. Read-only: the package sets are shared, never mutate them.
var UseCategories = map[string]bool{"dex_lp": true, "lending_supply": true, "vault": true, "staking": true, "yield_market": true}

// LiquiditySemantics is LIQUIDITY_SEMANTICS (read-only).
var LiquiditySemantics = map[string]string{
	"dex_lp":         "pool_tvl",
	"yield_market":   "pool_tvl",
	"lending_supply": "vault_tvl",
	"vault":          "vault_tvl",
	"staking":        "vault_tvl",
}

// ExcludedKinds is EXCLUDED_KINDS (read-only).
var ExcludedKinds = map[string]bool{"derivative": true, "unclassified": true}

// CoveragePriority is COVERAGE_PRIORITY (read-only).
var CoveragePriority = []string{"search", "partial", "not_indexed", "unsupported_by_portals", "no_contracts", "data_unavailable"}

// Dec is analyze.py::dec: anything that is not a finite number (nil, bool, unparsable text, NaN, Infinity)
// becomes the null Decimal. Strings are trimmed like Python's Decimal(str(...)).
func Dec(value any) model.Decimal {
	switch v := value.(type) {
	case nil:
		return model.Decimal{}
	case bool:
		return model.Decimal{}
	case model.Decimal:
		return v
	case model.LenientDecimal:
		return v.Decimal
	case decimal.Decimal:
		return model.NewDecimal(v)
	case *string:
		if v == nil {
			return model.Decimal{}
		}
		return Dec(*v)
	case string:
		d, err := decimal.NewFromString(strings.TrimSpace(v))
		if err != nil {
			return model.Decimal{}
		}
		return model.NewDecimal(d)
	case json.Number:
		return Dec(string(v))
	case int:
		return model.NewDecimal(decimal.NewFromInt(int64(v)))
	case int64:
		return model.NewDecimal(decimal.NewFromInt(v))
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return model.Decimal{}
		}
		return Dec(strconv.FormatFloat(v, 'f', -1, 64))
	}
	return model.Decimal{}
}

// Str is analyze.py::dstr: format(d, "f"), nil for the null Decimal.
func Str(d model.Decimal) *string { return d.StringPtr() }

// timeLayouts are the ISO-8601 shapes Python 3.11+ datetime.fromisoformat accepts that matter here: extended and
// basic dates, "T" or space separator, optional seconds and fraction, and an optional zone (Z, +00:00, +0000, +00).
var timeLayouts = func() []string {
	var out []string
	for _, date := range []string{"2006-01-02", "20060102"} {
		times := []string{"15:04:05.999999999", "15:04", "15"}
		if date == "20060102" {
			times = []string{"150405.999999999", "1504", "15"}
		}
		for _, sep := range []string{"T", " "} {
			for _, tm := range times {
				for _, zone := range []string{"Z07:00", "Z0700", "Z07", ""} {
					out = append(out, date+sep+tm+zone)
				}
			}
		}
		out = append(out, date)
	}
	return out
}()

// ParseTime is analyze.py::parse_time: an ISO-8601 timestamp, ok=false when it cannot be parsed.
// A timestamp without zone is taken as UTC.
func ParseTime(value string) (time.Time, bool) {
	for _, layout := range timeLayouts {
		if t, err := time.Parse(layout, value); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// FormatTime renders a timestamp in the 1.x export format: UTC, whole seconds, "Z".
func FormatTime(t time.Time) string { return t.UTC().Format("2006-01-02T15:04:05Z") }

// norm is analyze.py::_norm: upper-case, keep only [A-Z0-9].
func norm(s string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(s) {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// truthy mirrors Python truthiness of an optional string: None and "" are false.
func truthy(s *string) bool { return s != nil && *s != "" }

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
