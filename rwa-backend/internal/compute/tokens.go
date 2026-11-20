package compute

import (
	"strings"

	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/model"
)

// NormalizeAddress is cmc.py::normalize_address; the single implementation lives in model.NormalizeAddress.
func NormalizeAddress(address string) string { return model.NormalizeAddress(address) }

// ClassifyToken is analyze.py::classify_token. It returns (kind, excluded_reason); derivatives and tokens of
// another share class stay out of rankings.
func ClassifyToken(token model.CMCToken, ticker string) (kind string, excludedReason *string) {
	reason := func(s string) *string { return &s }
	name := strings.ToLower(deref(token.Name))
	issuer := strings.ToLower(deref(token.IssuerName))
	if !truthy(token.Symbol) {
		return "unclassified", reason("missing_symbol")
	}
	if strings.Contains(name, "(derivatives)") || strings.Contains(issuer, "(derivatives)") {
		return "derivative", reason("derivative")
	}
	if !strings.Contains(norm(*token.Symbol), norm(ticker)) {
		return "unclassified", reason("symbol_does_not_match_ticker")
	}
	if strings.HasPrefix(name, "wrapped") {
		return "wrapper", nil
	}
	return "tokenized", nil
}

// Dep is a parsed deployment with its raw (un-normalized) address, which the export does not carry but the
// deployments table does.
type Dep struct {
	model.Deployment
	AddressRaw string
}

// ParseDeployments is cmc.py::parse_deployments: one deployment per (CMC network, address); the same address
// on two networks is two deployments. Status is "unmapped_network" / "unsupported_network" when decided here and ""
// (Python None) when it is resolved after the Portals lookup (DeploymentStatus). A nil info yields no deployments.
func ParseDeployments(info *model.CMCInfo, chainMapping map[string]*string) []Dep {
	if info == nil {
		return nil
	}
	type entry struct {
		address string
		network *string
	}
	var entries []entry
	for _, e := range info.ContractAddress {
		var n *string
		if e.Platform != nil {
			n = e.Platform.Name
		}
		entries = append(entries, entry{e.ContractAddress, n})
	}
	if info.Platform != nil && info.Platform.TokenAddress != nil && *info.Platform.TokenAddress != "" {
		fallback := *info.Platform.TokenAddress
		found := false
		for _, e := range entries {
			if NormalizeAddress(e.address) == NormalizeAddress(fallback) {
				found = true
				break
			}
		}
		if !found {
			entries = append(entries, entry{fallback, info.Platform.Name})
		}
	}

	var out []Dep
	type seenKey struct{ network, address string }
	seen := map[seenKey]bool{}
	for _, e := range entries {
		rawAddress := strings.TrimSpace(e.address)
		cmcNetwork := deref(e.network)
		if rawAddress == "" || cmcNetwork == "" {
			continue
		}
		address := NormalizeAddress(rawAddress)
		if seen[seenKey{cmcNetwork, address}] {
			continue
		}
		seen[seenKey{cmcNetwork, address}] = true
		network, mapped := chainMapping[cmcNetwork]
		d := Dep{AddressRaw: rawAddress}
		d.CMCNetwork = cmcNetwork
		d.Network = network
		d.Address = address
		switch {
		case !mapped:
			d.Status = "unmapped_network"
		case network == nil:
			d.Status = "unsupported_network"
		default:
			k := *network + ":" + address
			d.Key = &k
		}
		out = append(out, d)
	}
	return out
}

// DeploymentStatus is analyze.py::deployment_status: the status decided by ParseDeployments wins, then the
// Portals lookup result.
func DeploymentStatus(dep model.Deployment, portals *model.SnapshotPortals) string {
	if dep.Status != "" {
		return dep.Status
	}
	if portals == nil {
		return "data_unavailable"
	}
	if dep.Network == nil || !contains(portals.Networks, *dep.Network) {
		return "unsupported_network"
	}
	if dep.Key == nil || !contains(portals.LookedUp, *dep.Key) {
		return "not_checked"
	}
	if _, ok := portals.Exact[*dep.Key]; ok {
		return "indexed"
	}
	return "not_indexed"
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// TokenCoverage is analyze.py::token_coverage.
func TokenCoverage(deployments []model.Deployment, infoOK bool) string {
	statuses := map[string]bool{}
	for _, d := range deployments {
		statuses[d.Status] = true
	}
	switch {
	case !infoOK:
		return "data_unavailable"
	case len(deployments) == 0:
		return "no_contracts"
	case statuses["indexed"]:
		return "search"
	case statuses["not_checked"] || statuses["data_unavailable"]:
		return "partial"
	case statuses["not_indexed"]:
		return "not_indexed"
	}
	return "unsupported_by_portals"
}
