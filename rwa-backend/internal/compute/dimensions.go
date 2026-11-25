package compute

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/model"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/store"
)

// Dimensions are the current-state rows of one build, ready for the store's Upsert* methods
// (order matters for foreign keys: catalog_issuers, _tokens, _deployments, _platforms, _positions, _token_positions).
type Dimensions struct {
	UnderlyingFacts []store.UnderlyingFactsRow
	Issuers         []store.IssuerRow
	Tokens          []store.TokenRow
	Deployments     []store.DeploymentRow
	Platforms       []store.PlatformRow
	Networks        []store.NetworkRow
	Positions       []store.PositionRow
	TokenPositions  []store.TokenPositionRow
	// Warnings lists tokens dropped because the same CMC id appears under several underlyings (see dedupeTokens).
	Warnings []string
}

// dedupeTokens implements the policy for a CMC id listed more than once (under two underlyings, or twice in
// one RWA record): the export keeps every copy as 1.x did, but catalog_tokens and history_tokens are keyed by id,
// so the store rows keep only the occurrence from the NEWEST snapshot (ties: the first in the export, which is the
// sorted snapshot order). keep[i] tells whether exp.Tokens[i] is kept; warnings describe the dropped ones.
func dedupeTokens(exp *model.Export) (keep []bool, warnings []string) {
	keep = make([]bool, len(exp.Tokens))
	best := map[int64]int{}
	at := func(i int) time.Time {
		if exp.Tokens[i].FetchedAt != nil {
			if t, ok := ParseTime(*exp.Tokens[i].FetchedAt); ok {
				return t
			}
		}
		return time.Time{}
	}
	for i := range exp.Tokens {
		id := exp.Tokens[i].ID
		if b, ok := best[id]; !ok || at(i).After(at(b)) {
			best[id] = i
		}
	}
	for i := range exp.Tokens {
		t := exp.Tokens[i]
		b := best[t.ID]
		keep[i] = b == i
		if !keep[i] {
			warnings = append(warnings, fmt.Sprintf("token %d (%s) listed under underlying %s is dropped from store rows: the newest occurrence is under %s",
				t.ID, deref(t.Symbol), t.UnderlyingID, exp.Tokens[b].UnderlyingID))
		}
	}
	return keep, warnings
}

// DimensionRows maps a finalized build to the store's upsert rows. res.Side supplies what the export does not
// carry (raw addresses, CMC logo URLs, date_added, position created_at, issuer rosters); rwaInfo (may be nil)
// supplies the industry. platformNames and settings.ChainMapping give the platforms and networks tables.
func DimensionRows(res *Result, runID int64, settings *model.Settings, platformNames map[string]string, rwaInfo map[string]RwaInfo) (*Dimensions, error) {
	exp, side := &res.Export, &res.Side
	d := &Dimensions{}
	keep, warnings := dedupeTokens(exp)
	d.Warnings = warnings

	for _, u := range exp.Underlyings {
		status := u.Status
		f := store.UnderlyingFactsRow{ID: u.ID, CMCRwaID: u.CMCRwaID, CMCName: u.CMCName, Website: u.Website, Status: &status, LastRunID: runID}
		if u.CMCRwaID != nil {
			if info, ok := rwaInfo[fmt.Sprint(*u.CMCRwaID)]; ok {
				f.Industry = info.Industry
				f.CMCLogoURL = info.Logo
			}
		}
		d.UnderlyingFacts = append(d.UnderlyingFacts, f)
	}

	issuerRow := func(id string, name, label, website *string, rosterSize *int, status string) store.IssuerRow {
		meta := side.Issuers[id]
		row := store.IssuerRow{ID: id, Name: name, CMCLabel: label, Website: website, CMCLogoURL: meta.Logo,
			RosterSize: rosterSize, Roster: meta.Roster, MetadataStatus: status}
		row.FetchedAt = parseTimePtr(meta.FetchedAt)
		return row
	}
	known := map[string]bool{}
	for _, is := range exp.Issuers {
		known[is.ID] = true
		// The website stored is the CMC value of the cache, never the exported one: curated.Apply may have replaced
		// the exported website with the config/issuers.json override, which must affect the export only.
		d.Issuers = append(d.Issuers, issuerRow(is.ID, is.Name, is.CMCLabel, side.Issuers[is.ID].Website, is.RosterSize, is.MetadataStatus))
	}
	// tokens.issuer_id is a foreign key: issuers whose tokens are all excluded (derivatives) are not in the export
	// but must have a row. Stubs mirror what build_issuers would have written.
	for i := range exp.Tokens {
		t := &exp.Tokens[i]
		if !keep[i] || !truthy(t.IssuerID) || known[*t.IssuerID] {
			continue
		}
		id := *t.IssuerID
		known[id] = true
		meta := side.Issuers[id]
		name := t.IssuerName
		if truthy(meta.Name) {
			name = meta.Name
		}
		status := "unknown"
		if meta.present() {
			status = "cmc"
		}
		d.Issuers = append(d.Issuers, issuerRow(id, name, t.IssuerName, meta.Website, meta.RosterSize, status))
	}

	for i := range exp.Tokens {
		t := &exp.Tokens[i]
		if !keep[i] {
			continue
		}
		tk := TokenKey{t.ID, t.UnderlyingID}
		issuerID := t.IssuerID
		if !truthy(issuerID) {
			issuerID = nil
		}
		info := side.TokenInfo[tk]
		d.Tokens = append(d.Tokens, store.TokenRow{
			ID: t.ID, UnderlyingID: t.UnderlyingID, IssuerID: issuerID, Symbol: t.Symbol, Name: t.Name, CMCSlug: t.CMCSlug,
			Kind: t.Kind, ExcludedReason: t.ExcludedReason, CMCLogoURL: info.LogoURL, DateAdded: parseTimePtr(info.DateAdded),
			Coverage: t.Coverage,
		})
		for _, dep := range t.Deployments {
			d.Deployments = append(d.Deployments, store.DeploymentRow{
				TokenID: t.ID, CMCNetwork: dep.CMCNetwork, Address: dep.Address,
				AddressRaw: side.DeploymentRaw[DeploymentID{tk, dep.CMCNetwork, dep.Address}],
				Network:    dep.Network, PositionKey: dep.Key, Status: dep.Status,
				PortalsSymbol: dep.PortalsSymbol, Decimals: dep.Decimals, PortalsPlatform: dep.PortalsPlatform,
			})
		}
		for _, r := range t.Relations {
			link, depth := "direct", r.Depth
			if r.Depth > 1 {
				link = "indirect"
			}
			d.TokenPositions = append(d.TokenPositions, store.TokenPositionRow{TokenID: t.ID, PositionKey: r.Key, Link: link, Depth: &depth, Path: r.Path})
		}
		for _, k := range t.UnconfirmedKeys {
			d.TokenPositions = append(d.TokenPositions, store.TokenPositionRow{TokenID: t.ID, PositionKey: k, Link: "unconfirmed", Path: []string{}})
		}
	}

	for _, o := range exp.Opportunities {
		components, err := json.Marshal(o.Components)
		if err != nil {
			return nil, fmt.Errorf("compute: dimensions: position %s components: %w", o.Key, err)
		}
		var created *string
		if c, ok := side.PositionCreatedAt[o.Key]; ok {
			created = &c
		}
		d.Positions = append(d.Positions, store.PositionRow{
			Key: o.Key, Network: o.Network, Platform: o.Platform, Symbol: o.Symbol, Name: o.Name, Address: o.Address,
			PositionKind: o.PositionKind, ReceiptTokenKey: o.ReceiptTokenKey, Category: o.Category, Tags: o.Tags,
			Components: components, LiquiditySemantics: o.LiquiditySemantic, IsPortable: o.IsPortable,
			PortalsCreatedAt: parseTimePtr(created), LiquidityUSD: o.LiquidityUSD, APYPercent: o.APYPercent,
			BaseAPYPercent: o.BaseAPYPercent, RewardAPYPercent: o.RewardAPYPercent, BorrowAPYPercent: o.BorrowAPYPercent,
			VolumeUSD1d: o.VolumeUSD1d, VolumeUSD7d: o.VolumeUSD7d, SourceUpdatedAt: parseTimePtr(o.UpdatedAt),
			Flags: o.Flags, Eligibility: o.Eligibility,
		})
	}

	for _, slug := range slices.Sorted(maps.Keys(platformNames)) {
		d.Platforms = append(d.Platforms, store.PlatformRow{Slug: slug, Name: platformNames[slug]})
	}
	// networks: every chain_mapping slug plus the Portals networks of the newest snapshot; cmc_names from chain_mapping.
	cmcNames := map[string][]string{}
	for cmc, network := range settings.ChainMapping {
		if network != nil {
			cmcNames[*network] = append(cmcNames[*network], cmc)
		}
	}
	for _, slug := range side.PortalsNetworks {
		if _, ok := cmcNames[slug]; !ok {
			cmcNames[slug] = []string{}
		}
	}
	for _, slug := range slices.Sorted(maps.Keys(cmcNames)) {
		names := cmcNames[slug]
		slices.Sort(names)
		d.Networks = append(d.Networks, store.NetworkRow{Slug: slug, CMCNames: names})
	}
	return d, nil
}
