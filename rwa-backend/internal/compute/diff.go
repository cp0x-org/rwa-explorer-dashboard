package compute

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/shopspring/decimal"
)

// DiffOptions tunes DiffExports.
type DiffOptions struct {
	// IgnorePaths are dotted paths (as printed in the differences, e.g. "generated_at") that are skipped.
	IgnorePaths []string
}

// DiffExports structurally compares two export documents (a = want, b = got) and returns one line per difference,
// e.g. "tokens[3].yield.median_reported_apy_percent: want 10.503 got 10.51". Numbers and numeric strings are compared
// as decimals (trailing zeros do not matter), array order is significant, and so is the key order of the sorted
// breakdown objects (by_network, by_category, by_platform, by_underlying). It is the parity comparator shared by
// the parity test and the import-v1 test.
func DiffExports(a, b []byte, opts DiffOptions) ([]string, error) {
	want, err := parseTree(a)
	if err != nil {
		return nil, fmt.Errorf("compute: diff: parse first document: %w", err)
	}
	got, err := parseTree(b)
	if err != nil {
		return nil, fmt.Errorf("compute: diff: parse second document: %w", err)
	}
	var diffs []string
	cmpTree("", "", want, got, opts.IgnorePaths, func(path string, w, g any) {
		diffs = append(diffs, fmt.Sprintf("%s: want %s got %s", path, show(w), show(g)))
	})
	return diffs, nil
}

// object is a JSON object that remembers its key order.
type object struct {
	keys   []string
	values map[string]any
}

func parseTree(b []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	v, err := readValue(dec)
	if err != nil {
		return nil, err
	}
	return v, nil
}

func readValue(dec *json.Decoder) (any, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	switch d := tok.(type) {
	case json.Delim:
		switch d {
		case '{':
			o := &object{values: map[string]any{}}
			for dec.More() {
				kt, err := dec.Token()
				if err != nil {
					return nil, err
				}
				k := kt.(string)
				v, err := readValue(dec)
				if err != nil {
					return nil, err
				}
				o.keys = append(o.keys, k)
				o.values[k] = v
			}
			_, err := dec.Token()
			return o, err
		case '[':
			arr := []any{}
			for dec.More() {
				v, err := readValue(dec)
				if err != nil {
					return nil, err
				}
				arr = append(arr, v)
			}
			_, err := dec.Token()
			return arr, err
		}
	}
	return tok, nil
}

// orderedKeys are the objects whose key order is part of the result (sorted breakdowns).
var orderedKeys = map[string]bool{"by_network": true, "by_category": true, "by_platform": true, "by_underlying": true}

func show(v any) string {
	switch x := v.(type) {
	case nil:
		return "null"
	case *object:
		return fmt.Sprintf("object(%d keys)", len(x.keys))
	case []any:
		return fmt.Sprintf("array(%d)", len(x))
	case string:
		return strconv.Quote(x)
	case json.Number:
		return x.String()
	}
	return fmt.Sprint(v)
}

func decimalOf(v any) (decimal.Decimal, bool) {
	switch x := v.(type) {
	case json.Number:
		d, err := decimal.NewFromString(x.String())
		return d, err == nil
	case string:
		d, err := decimal.NewFromString(x)
		return d, err == nil
	}
	return decimal.Decimal{}, false
}

// cmpTree walks both trees; ignored paths are skipped, decimals are compared as decimals, arrays in order.
func cmpTree(path, key string, want, got any, ignore []string, report func(path string, w, g any)) {
	if slices.Contains(ignore, path) {
		return
	}
	switch w := want.(type) {
	case *object:
		g, ok := got.(*object)
		if !ok {
			report(path, want, got)
			return
		}
		for _, k := range w.keys {
			sub := path + "." + k
			if path == "" {
				sub = k
			}
			gv, ok := g.values[k]
			if !ok {
				report(sub, w.values[k], "<missing>")
				continue
			}
			cmpTree(sub, k, w.values[k], gv, ignore, report)
		}
		for _, k := range g.keys {
			if _, ok := w.values[k]; !ok {
				sub := path + "." + k
				if path == "" {
					sub = k
				}
				if slices.Contains(ignore, sub) {
					continue
				}
				report(sub, "<missing>", g.values[k])
			}
		}
		if orderedKeys[key] && !slices.Equal(w.keys, g.keys) {
			report(path+" (key order)", strings.Join(w.keys, ","), strings.Join(g.keys, ","))
		}
	case []any:
		g, ok := got.([]any)
		if !ok {
			report(path, want, got)
			return
		}
		if len(w) != len(g) {
			report(path+" (length)", len(w), len(g))
		}
		for i := range min(len(w), len(g)) {
			cmpTree(fmt.Sprintf("%s[%d]", path, i), key, w[i], g[i], ignore, report)
		}
	default:
		if want == nil || got == nil {
			if want != got {
				report(path, want, got)
			}
			return
		}
		if ws, ok := want.(string); ok {
			if gs, ok := got.(string); ok && ws == gs {
				return
			}
		}
		wd, wok := decimalOf(want)
		gd, gok := decimalOf(got)
		if wok && gok && fmt.Sprintf("%T", want) == fmt.Sprintf("%T", got) {
			if !wd.Equal(gd) {
				report(path, want, got)
			}
			return
		}
		if want != got {
			report(path, want, got)
		}
	}
}
