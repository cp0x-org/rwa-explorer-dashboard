//go:build parity

package compute_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

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

	// build_id and run_id are 2.0-only fields (absent from the 1.x export); compute leaves them null.
	require.Nil(t, exp.BuildID)
	require.Nil(t, exp.RunID)
	diffs, err := compute.DiffExports(want, got, compute.DiffOptions{IgnorePaths: []string{"generated_at", "build_id", "run_id"}})
	require.NoError(t, err)
	perSection := map[string]int{}
	for _, d := range diffs {
		path, _, _ := strings.Cut(d, ": want ")
		section := strings.SplitN(strings.SplitN(strings.SplitN(path, ".", 2)[0], "[", 2)[0], " ", 2)[0]
		perSection[section]++
	}
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
