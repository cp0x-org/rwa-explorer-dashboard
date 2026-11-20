//go:build parity

package compute_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/compute"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/model"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/v1files"
)

// TestParity runs Build+Finalize over the 1.x data and compares the result with the 1.x export
// (testdata/golden/rwa.json). Run it with `make parity`; it needs the 1.x files next to the repo
// (rwa-data-pipeline/data, or RWA_V1_DATA_DIR).
func TestParity(t *testing.T) {
	dataDir := os.Getenv("RWA_V1_DATA_DIR")
	if dataDir == "" {
		dataDir = filepath.Join("..", "..", "..", "rwa-data-pipeline", "data")
	}
	if _, err := os.Stat(filepath.Join(dataDir, "snapshots")); err != nil {
		t.Skipf("1.x data not found at %s (set RWA_V1_DATA_DIR): %v", dataDir, err)
	}
	settings, err := model.LoadSettings(filepath.Join("..", "..", "config", "settings.json"))
	require.NoError(t, err)
	assets, err := model.LoadAssets(filepath.Join("..", "..", "config", "assets.json"))
	require.NoError(t, err)

	var snaps []*model.Snapshot
	for _, a := range assets {
		if !a.Enabled {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dataDir, "snapshots", a.StorageID()+".json"))
		if os.IsNotExist(err) {
			continue // like 1.x: an enabled asset without a snapshot is skipped
		}
		require.NoError(t, err)
		s, err := model.ParseSnapshot(b)
		require.NoError(t, err, a.Ticker)
		snaps = append(snaps, s)
	}
	t.Logf("snapshots loaded: %d", len(snaps))
	issuers, err := v1files.LoadIssuers(filepath.Join(dataDir, "issuers.json"))
	require.NoError(t, err)
	platforms, err := v1files.LoadPlatformNames(filepath.Join(dataDir, "portals_platforms.json"))
	require.NoError(t, err)
	rwaInfo, err := v1files.LoadRwaInfo(filepath.Join(dataDir, "rwa_info.json"))
	require.NoError(t, err)

	res, err := compute.Build(snaps, issuers, settings, platforms)
	require.NoError(t, err)
	exp := compute.Finalize(res, time.Now(), rwaInfo, fsLogos{filepath.Join(dataDir, "logos")})
	got, err := json.Marshal(exp)
	require.NoError(t, err)
	want, err := os.ReadFile(filepath.Join("..", "..", "testdata", "golden", "rwa.json"))
	require.NoError(t, err)

	wantTree, err := parseTree(want)
	require.NoError(t, err)
	gotTree, err := parseTree(got)
	require.NoError(t, err)
	// build_id and run_id are 2.0-only fields (absent from the 1.x export); compute leaves them null.
	gotObj := gotTree.(*object)
	for _, k := range []string{"build_id", "run_id"} {
		require.Nil(t, gotObj.values[k], k)
		delete(gotObj.values, k)
		gotObj.keys = slices.DeleteFunc(gotObj.keys, func(s string) bool { return s == k })
	}
	var diffs []string
	perSection := map[string]int{}
	cmpTree("", "", wantTree, gotTree, func(path string, w, g any) {
		section := strings.SplitN(strings.SplitN(path, ".", 2)[0], "[", 2)[0]
		perSection[section]++
		diffs = append(diffs, fmt.Sprintf("%s: want %s got %s", path, show(w), show(g)))
	})
	if len(diffs) == 0 {
		t.Logf("PARITY OK: 0 differences (underlyings=%d tokens=%d issuers=%d opportunities=%d)",
			len(exp.Underlyings), len(exp.Tokens), len(exp.Issuers), len(exp.Opportunities))
		return
	}
	for _, d := range diffs[:min(50, len(diffs))] {
		t.Log(d)
	}
	var sections []string
	for s, n := range perSection {
		sections = append(sections, fmt.Sprintf("%s=%d", s, n))
	}
	sort.Strings(sections)
	t.Errorf("PARITY FAILED: %d differences (%s)", len(diffs), strings.Join(sections, " "))
}

// fsLogos resolves logos from <dir>/<kind>/<id>.* like 1.x logos.py::find_logo (first match by name).
type fsLogos struct{ dir string }

func (f fsLogos) Find(kind, entityID string) (string, bool) {
	entries, err := os.ReadDir(filepath.Join(f.dir, kind))
	if err != nil {
		return "", false
	}
	var names []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), entityID+".") {
			names = append(names, e.Name())
		}
	}
	if len(names) == 0 {
		return "", false
	}
	slices.Sort(names)
	return strings.TrimPrefix(names[0], entityID+"."), true
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

// cmpTree walks both trees; generated_at is ignored, decimals are compared as decimals, arrays in order.
func cmpTree(path, key string, want, got any, report func(path string, w, g any)) {
	if path == "generated_at" {
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
			cmpTree(sub, k, w.values[k], gv, report)
		}
		for _, k := range g.keys {
			if _, ok := w.values[k]; !ok {
				report(path+"."+k, "<missing>", g.values[k])
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
			cmpTree(fmt.Sprintf("%s[%d]", path, i), key, w[i], g[i], report)
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
