package compute

import (
	"maps"
	"slices"
	"strings"
	"unicode"

	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/model"
)

// Relations maps a position key to how a token reaches it (analyze.py: dict[str, dict]).
type Relations = map[string]model.Relation

// ParentIndex is analyze.py::parent_index: child key -> set of parent position keys.
func ParentIndex(records map[string]model.PortalsRecord) map[string]map[string]bool {
	index := map[string]map[string]bool{}
	for key, record := range records {
		for _, child := range ChildKeys(record) {
			if index[child] == nil {
				index[child] = map[string]bool{}
			}
			index[child][NormKey(key)] = true
		}
	}
	return index
}

// FindRelations is analyze.py::find_relations: BFS up the "contains" graph (root -> LP -> vault). The minimum
// depth wins, cycles are cut by visited. Tracked tokens (stopKeys) are neither counted nor traversed: a wrapper
// or another version is a token, not a use. Swaps never appear because only composition edges exist.
func FindRelations(roots []string, index map[string]map[string]bool, stopKeys map[string]bool, maxDepth int) Relations {
	found := Relations{}
	sortedRoots := slices.Sorted(slices.Values(roots))
	type node struct {
		key  string
		path []string
	}
	for _, root := range sortedRoots {
		visited := map[string]bool{root: true}
		frontier := []node{{root, []string{root}}}
		for depth := 1; depth <= maxDepth; depth++ {
			var next []node
			for _, n := range frontier {
				for _, parent := range slices.Sorted(maps.Keys(index[n.key])) {
					if visited[parent] || stopKeys[parent] {
						continue
					}
					visited[parent] = true
					newPath := append(slices.Clone(n.path), parent)
					if cur, ok := found[parent]; !ok || depth < cur.Depth {
						found[parent] = model.Relation{Key: parent, Depth: depth, Path: newPath}
					}
					next = append(next, node{parent, newPath})
				}
			}
			frontier = next
		}
	}
	return found
}

// SymbolCandidates is analyze.py::symbol_candidates: positions whose symbol names the token on the same network
// but whose composition does not contain its address (e.g. Morpho markets expose only the loan token). They are
// kept as unconfirmed, never counted. The result is sorted.
func SymbolCandidates(symbol string, networks map[string]bool, opps map[string]*Opp, exclude map[string]bool) []string {
	target := norm(symbol)
	var out []string
	for key, opp := range opps {
		if exclude[key] || !networks[opp.Network] || !UseCategories[opp.Category] {
			continue
		}
		parts := strings.FieldsFuncSeq(deref(opp.Symbol), func(r rune) bool {
			return r == '/' || r == '-' || r == '_' || unicode.IsSpace(r)
		})
		hit := false
		empty := true
		for p := range parts {
			empty = false
			if norm(p) == target {
				hit = true
			}
		}
		// re.split of a string without separators (or of "") still yields one (possibly empty) part.
		if empty && target == "" {
			hit = true
		}
		if hit {
			out = append(out, key)
		}
	}
	slices.Sort(out)
	return out
}
