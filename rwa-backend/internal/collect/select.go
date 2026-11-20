package collect

import (
	"fmt"
	"slices"
	"strings"

	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/model"
)

// Run modes.
const (
	ModeRefresh = "refresh"
	ModeAdd     = "add"
)

// SelectAssets is collect.py::select_assets: the enabled assets, narrowed to symbols (case-insensitive) when
// given. An unknown or disabled symbol is an error.
func SelectAssets(assets []model.Asset, symbols []string) ([]model.Asset, error) {
	var enabled []model.Asset
	for _, a := range assets {
		if a.Enabled {
			enabled = append(enabled, a)
		}
	}
	wanted := map[string]bool{}
	for _, s := range symbols {
		if s = strings.ToUpper(strings.TrimSpace(s)); s != "" {
			wanted[s] = true
		}
	}
	if len(wanted) == 0 {
		return enabled, nil
	}
	known := map[string]bool{}
	for _, a := range enabled {
		known[a.Ticker] = true
	}
	var unknown []string
	for s := range wanted {
		if !known[s] {
			unknown = append(unknown, s)
		}
	}
	if len(unknown) > 0 {
		slices.Sort(unknown)
		return nil, fmt.Errorf("unknown or disabled symbols: %s", strings.Join(unknown, ", "))
	}
	return slices.DeleteFunc(enabled, func(a model.Asset) bool { return !wanted[a.Ticker] }), nil
}

// Todo narrows the selected assets to the ones a run fetches: all in refresh mode, only those without any
// snapshot (hasSnapshot keyed by underlying id) in add mode.
func Todo(selected []model.Asset, mode string, hasSnapshot map[string]bool) []model.Asset {
	if mode == ModeRefresh {
		return selected
	}
	var out []model.Asset
	for _, a := range selected {
		if !hasSnapshot[a.ID()] {
			out = append(out, a)
		}
	}
	return out
}

// Run statuses.
const (
	RunSucceeded = "succeeded"
	RunPartial   = "partial"
	RunFailed    = "failed"
)

// DecideStatus maps the outcome of a run to its final status: a fatal error or interruption with nothing
// completed is failed (an interruption is failed even after some underlyings completed), some failed
// underlyings make it partial, otherwise succeeded. Like 1.x, a fatal error after some underlyings completed
// (authentication failed mid-run) is partial.
func DecideStatus(completed, failed int, fatal, interrupted bool) string {
	switch {
	case interrupted:
		return RunFailed
	case fatal && completed == 0:
		return RunFailed
	case fatal || failed > 0:
		return RunPartial
	}
	return RunSucceeded
}
