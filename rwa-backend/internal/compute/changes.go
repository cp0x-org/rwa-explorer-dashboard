package compute

import (
	"encoding/json"
	"maps"
	"slices"

	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/model"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/store"
)

// ChangeDetails is the JSON written to build_changes.details: eligible-position deltas per network and per
// platform slug (cur - prev, zero deltas omitted), tokens that moved from excluded to included, and a marker
// for the first build.
type ChangeDetails struct {
	EligibleByNetwork  map[string]int `json:"eligible_by_network"`
	EligibleByPlatform map[string]int `json:"eligible_by_platform"`
	// Reclassified are token ids that were excluded (derivative/unclassified) in prev and are included in cur.
	Reclassified []int32 `json:"reclassified"`
	FirstBuild   bool    `json:"first_build,omitempty"`
}

// Changes diffs two exports for the "updates" page. BuildID and PreviousBuildID are left for the caller.
//
// With prev == nil (the first build) there is nothing to compare with: the row has zero counts and
// details.first_build = true, rather than marking everything new.
//
// Tokens are compared over INCLUDED tokens only (kind not derivative/unclassified), the set the rankings show:
// new_tokens are included tokens of cur that prev did not list at all, removed_tokens are included tokens of prev that
// are no longer included in cur (gone or turned into a derivative), and a token that was excluded in prev and is
// included in cur is reported in details.reclassified, not as new. Positions are the export's opportunities
// (confirmed and unconfirmed). eligible_delta is the difference of the summary counters; the per-network and
// per-platform deltas count eligible confirmed positions of the non-excluded tokens, the same set the summary counts.
func Changes(prev, cur *model.Export) store.BuildChangesRow {
	if prev == nil {
		details, _ := json.Marshal(ChangeDetails{EligibleByNetwork: map[string]int{}, EligibleByPlatform: map[string]int{}, Reclassified: []int32{}, FirstBuild: true})
		return store.BuildChangesRow{NewTokens: []int32{}, RemovedTokens: []int32{}, NewIssuers: []string{}, Details: details}
	}
	row := store.BuildChangesRow{
		NewTokens:     []int32{},
		RemovedTokens: []int32{},
		NewIssuers:    []string{},
		EligibleDelta: cur.Summary.EligibleOpportunitiesCount - prev.Summary.EligibleOpportunitiesCount,
	}
	prevAll, prevIncluded, curIncluded := map[int64]bool{}, map[int64]bool{}, map[int64]bool{}
	for _, t := range prev.Tokens {
		prevAll[t.ID] = true
		if !ExcludedKinds[t.Kind] {
			prevIncluded[t.ID] = true
		}
	}
	for _, t := range cur.Tokens {
		if !ExcludedKinds[t.Kind] {
			curIncluded[t.ID] = true
		}
	}
	reclassified := []int32{}
	for _, id := range slices.Sorted(maps.Keys(curIncluded)) {
		switch {
		case prevIncluded[id]:
		case prevAll[id]:
			reclassified = append(reclassified, int32(id)) //nolint:gosec // CMC crypto ids fit int32 (the column is INT)
		default:
			row.NewTokens = append(row.NewTokens, int32(id)) //nolint:gosec // see above
		}
	}
	for _, id := range slices.Sorted(maps.Keys(prevIncluded)) {
		if !curIncluded[id] {
			row.RemovedTokens = append(row.RemovedTokens, int32(id)) //nolint:gosec // see above
		}
	}
	prevIssuers := map[string]bool{}
	for _, i := range prev.Issuers {
		prevIssuers[i.ID] = true
	}
	for _, i := range cur.Issuers {
		if !prevIssuers[i.ID] {
			row.NewIssuers = append(row.NewIssuers, i.ID)
		}
	}
	slices.Sort(row.NewIssuers)

	prevKeys, curKeys := map[string]bool{}, map[string]bool{}
	for _, o := range prev.Opportunities {
		prevKeys[o.Key] = true
	}
	for _, o := range cur.Opportunities {
		curKeys[o.Key] = true
		if !prevKeys[o.Key] {
			row.NewPositionsCount++
		}
	}
	for k := range prevKeys {
		if !curKeys[k] {
			row.GonePositionsCount++
		}
	}

	prevNet, prevPlat := eligibleCounts(prev)
	curNet, curPlat := eligibleCounts(cur)
	details := ChangeDetails{EligibleByNetwork: deltas(prevNet, curNet), EligibleByPlatform: deltas(prevPlat, curPlat), Reclassified: reclassified}
	row.Details, _ = json.Marshal(details) // maps of ints cannot fail to marshal
	return row
}

// eligibleCounts counts the eligible confirmed positions of the non-excluded tokens per network and per platform.
func eligibleCounts(e *model.Export) (byNetwork, byPlatform map[string]int) {
	byNetwork, byPlatform = map[string]int{}, map[string]int{}
	byKey := make(map[string]*model.Opportunity, len(e.Opportunities))
	for i := range e.Opportunities {
		byKey[e.Opportunities[i].Key] = &e.Opportunities[i]
	}
	seen := map[string]bool{}
	for _, t := range e.Tokens {
		if ExcludedKinds[t.Kind] {
			continue
		}
		for _, r := range t.Relations {
			o := byKey[r.Key]
			if o == nil || seen[r.Key] || o.Eligibility != "eligible" {
				continue
			}
			seen[r.Key] = true
			byNetwork[o.Network]++
			byPlatform[o.Platform]++
		}
	}
	return byNetwork, byPlatform
}

// deltas is cur - prev per key, zero deltas omitted.
func deltas(prev, cur map[string]int) map[string]int {
	out := map[string]int{}
	for k, n := range cur {
		if d := n - prev[k]; d != 0 {
			out[k] = d
		}
	}
	for k, n := range prev {
		if _, ok := cur[k]; !ok && n != 0 {
			out[k] = -n
		}
	}
	return out
}
