package compute

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math"
	"math/big"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/model"
)

// IssuerMeta is one entry of issuers.json (the CMC issuer cache). Python treats the entry as present when the
// dict is non-empty, whatever its keys: UnmarshalJSON records that.
type IssuerMeta struct {
	Name       *string         `json:"name"`
	Website    *string         `json:"website"`
	Logo       *string         `json:"logo"`
	RosterSize *int            `json:"roster_size"`
	Roster     json.RawMessage `json:"roster"`
	FetchedAt  *string         `json:"fetched_at"`

	nonEmpty bool // the JSON object had at least one key
}

type issuerMetaAlias IssuerMeta

// UnmarshalJSON decodes the modelled fields and remembers whether the object was non-empty.
func (m *IssuerMeta) UnmarshalJSON(b []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	var a issuerMetaAlias
	if err := json.Unmarshal(b, &a); err != nil {
		return err
	}
	*m = IssuerMeta(a)
	m.nonEmpty = len(raw) > 0
	return nil
}

func (m IssuerMeta) present() bool {
	return m.nonEmpty || m.Name != nil || m.Website != nil || m.Logo != nil || m.RosterSize != nil || len(m.Roster) > 0 || m.FetchedAt != nil
}

// TokenKey identifies a token within a build: the same CMC id may be listed under two underlyings.
type TokenKey struct {
	ID           int64
	UnderlyingID string
}

// DeploymentID identifies a deployment within a build.
type DeploymentID struct {
	Token      TokenKey
	CMCNetwork string
	Address    string
}

// TokenInfo is what the tokens table keeps beyond the export.
type TokenInfo struct {
	LogoURL   *string
	DateAdded *string
}

// Side is data gathered by Build that the export does not carry but the database does.
type Side struct {
	// PositionFetchedAt is the fetched_at of the snapshot that supplied the position record (newest wins).
	PositionFetchedAt map[string]time.Time
	// PositionCreatedAt is Portals' createdAt of the position record.
	PositionCreatedAt map[string]string
	// DeploymentRaw is the un-normalized contract address.
	DeploymentRaw map[DeploymentID]string
	TokenInfo     map[TokenKey]TokenInfo
	// Issuers holds the cache entry of every issuer referenced by a token (also by excluded-only issuers).
	Issuers map[string]IssuerMeta
	// PortalsNetworks is portals.networks of the newest snapshot that has a Portals part.
	PortalsNetworks []string
	// PositionMetadata holds the string-valued metadata of the raw Portals record of every position (the newest
	// snapshot wins, like the raw records of export.py::raw_portals_records); link templates read it.
	PositionMetadata map[string]map[string]string
}

// Result is the output of Build: the export without the parts Finalize adds (schema_version, generated_at,
// websites and logos), plus the side data.
type Result struct {
	Export model.Export
	Side   Side
}

// BuildOption tunes Build.
type BuildOption func(*buildOptions)

type buildOptions struct{ symbolAliases map[string][]string }

// WithSymbolAliases passes the symbol_aliases of config/assets.json, keyed by ticker (analyze.build symbol_aliases).
func WithSymbolAliases(aliases map[string][]string) BuildOption {
	return func(o *buildOptions) { o.symbolAliases = aliases }
}

// SymbolAliases collects the aliases of the assets by ticker, the argument of WithSymbolAliases.
func SymbolAliases(assets []model.Asset) map[string][]string {
	out := make(map[string][]string, len(assets))
	for _, a := range assets {
		out[a.Ticker] = a.SymbolAliases
	}
	return out
}

// symbolSearched is analyze.py::_symbol_searched: Portals searches substrings, so a searched term must occur in the
// token symbol (case-insensitive, by Python's str.casefold).
func symbolSearched(symbol *string, searches map[string]model.PortalsSearch) bool {
	s := casefold(deref(symbol))
	for term := range searches {
		if strings.Contains(s, casefold(term)) {
			return true
		}
	}
	return false
}

// casefold approximates Python's str.casefold. Lower-casing the upper-cased text folds the long s and the final
// sigma; Go has only simple case mappings, so ß (full folding: ss) is replaced explicitly. The remaining differences
// (a few exotic ligatures) do not occur in token symbols.
func casefold(s string) string {
	return strings.ReplaceAll(strings.ToLower(strings.ToUpper(s)), "ß", "ss")
}

// Build is analyze.py::build. snapshots are the latest snapshot of every enabled underlying (input order breaks
// fetched_at ties, so pass them in assets.json order); issuers is the issuers.json cache; platformNames maps a
// Portals platform slug to a protocol name.
func Build(snapshots []*model.Snapshot, issuers map[string]IssuerMeta, settings *model.Settings, platformNames map[string]string, opts ...BuildOption) (*Result, error) {
	var o buildOptions
	for _, opt := range opts {
		opt(&o)
	}
	if err := validateSettings(settings); err != nil {
		return nil, err
	}
	snaps := slices.Clone(snapshots)
	slices.SortStableFunc(snaps, func(a, b *model.Snapshot) int { return a.FetchedAt.Compare(b.FetchedAt) })
	side := Side{
		PositionFetchedAt: map[string]time.Time{},
		PositionCreatedAt: map[string]string{},
		PositionMetadata:  map[string]map[string]string{},
		DeploymentRaw:     map[DeploymentID]string{},
		TokenInfo:         map[TokenKey]TokenInfo{},
		Issuers:           map[string]IssuerMeta{},
	}

	for _, snap := range snaps {
		if snap.Portals != nil {
			side.PortalsNetworks = snap.Portals.Networks
		}
	}

	// 1. tokens and the basket-wide set of tracked deployment keys
	var tokens []model.Token
	for _, snap := range snaps {
		var info map[string]model.CMCInfo
		var rwa *model.CMCRwa
		var searches map[string]model.PortalsSearch
		if snap.Portals != nil {
			searches = snap.Portals.Searches
		}
		if snap.CMC != nil {
			info, rwa = snap.CMC.Info, snap.CMC.RWA
		}
		if rwa == nil {
			continue
		}
		for _, t := range rwa.Tokens {
			if t.CryptoID == nil {
				continue
			}
			cid := *t.CryptoID
			var tokenInfo *model.CMCInfo
			if v, ok := info[strconv.FormatInt(cid, 10)]; ok {
				tokenInfo = &v
			}
			kind, excludedReason := ClassifyToken(t, snap.Ticker, o.symbolAliases[snap.Ticker]...)
			tk := TokenKey{cid, snap.UnderlyingID}
			parsed := ParseDeployments(tokenInfo, settings.ChainMapping)
			deployments := make([]model.Deployment, len(parsed))
			for i, d := range parsed {
				d.Status = DeploymentStatus(d.Deployment, snap.Portals)
				if d.Key != nil && snap.Portals != nil {
					if exact, ok := snap.Portals.Exact[*d.Key]; ok {
						d.PortalsSymbol = exact.Symbol
						d.Decimals = exact.Decimals
						d.PortalsPlatform = exact.Platform
					}
				}
				deployments[i] = d.Deployment
				side.DeploymentRaw[DeploymentID{tk, d.CMCNetwork, d.Address}] = d.AddressRaw
			}
			coverage := TokenCoverage(deployments, tokenInfo != nil)
			if coverage == "search" && !symbolSearched(t.Symbol, searches) {
				coverage = "partial"
			}
			token := model.Token{
				ID:             cid,
				Symbol:         t.Symbol,
				Name:           model.NormalizeName(t.Name),
				Kind:           kind,
				ExcludedReason: excludedReason,
				UnderlyingID:   snap.UnderlyingID,
				Ticker:         snap.Ticker,
				IssuerID:       t.IssuerID,
				IssuerName:     model.NormalizeName(t.IssuerName),
				PriceUSD:       t.Price.Decimal,
				MarketCapUSD:   t.MarketCap.Decimal,
				Volume24hUSD:   t.Volume24h.Decimal,
				Deployments:    deployments,
				Coverage:       coverage,
				FetchedAt:      timePtr(snap.FetchedAt),
			}
			if tokenInfo != nil {
				token.Logo = tokenInfo.Logo
				token.CMCSlug = tokenInfo.Slug
				side.TokenInfo[tk] = TokenInfo{LogoURL: tokenInfo.Logo, DateAdded: tokenInfo.DateAdded}
			}
			tokens = append(tokens, token)
			if truthy(t.IssuerID) {
				if m := issuers[*t.IssuerID]; m.present() {
					side.Issuers[*t.IssuerID] = m
				}
			}
		}
	}
	trackedKeys := map[string]bool{}
	for _, t := range tokens {
		for _, d := range t.Deployments {
			if d.Key != nil {
				trackedKeys[*d.Key] = true
			}
		}
	}

	// 2. one normalized record per Portals key; the newest snapshot wins
	opps := map[string]*Opp{}
	for _, snap := range snaps {
		if snap.Portals == nil {
			continue
		}
		// Within one snapshot records are walked in sorted key order: deterministic (JSONB loses the file order
		// anyway); 1.x used file order, which only differs when two records normalize to the same key.
		for _, records := range []map[string]model.PortalsRecord{snap.Portals.Records, snap.Portals.Exact} {
			for _, name := range slices.Sorted(maps.Keys(records)) {
				opp := NormalizeOpportunity(records[name], settings, platformNames)
				opp.Eligibility = Eligibility(&opp, snap.FetchedAt, settings)
				opps[opp.Key] = &opp
				side.PositionFetchedAt[opp.Key] = snap.FetchedAt
				if md := records[name].Metadata; md != nil && len(md.Strings) > 0 {
					side.PositionMetadata[opp.Key] = md.Strings
				} else {
					delete(side.PositionMetadata, opp.Key)
				}
				if c := records[name].CreatedAt; c != nil {
					side.PositionCreatedAt[opp.Key] = *c
				} else {
					delete(side.PositionCreatedAt, opp.Key)
				}
			}
		}
	}
	symbolsByKey := make(map[string]*string, len(opps))
	for k, o := range opps {
		symbolsByKey[k] = o.Symbol
	}

	// 3. relations per token
	bySnapshot := map[string]*model.Snapshot{}
	for _, s := range snaps {
		bySnapshot[s.UnderlyingID] = s
	}
	for i := range tokens {
		token := &tokens[i]
		snap := bySnapshot[token.UnderlyingID]
		var records map[string]model.PortalsRecord
		if snap.Portals != nil {
			records = snap.Portals.Records
		}
		index := ParentIndex(records)
		var roots []string
		for _, d := range token.Deployments {
			if d.Status == "indexed" {
				roots = append(roots, *d.Key)
			}
		}
		relations := Relations{}
		if len(roots) > 0 {
			relations = FindRelations(roots, index, trackedKeys, settings.MaxGraphDepth)
		}
		localOpps := map[string]*Opp{}
		for k := range records {
			nk := NormKey(k)
			if opps[nk] == nil {
				return nil, fmt.Errorf("compute: position %q of %s has no record", nk, token.UnderlyingID)
			}
			localOpps[nk] = opps[nk]
		}
		for k := range relations {
			if opps[k] == nil {
				return nil, fmt.Errorf("compute: relation %q of token %d has no record", k, token.ID)
			}
		}
		var unconfirmed []string
		if !ExcludedKinds[token.Kind] {
			networks := map[string]bool{}
			for _, d := range token.Deployments {
				if d.Network != nil && *d.Network != "" {
					networks[*d.Network] = true
				}
			}
			exclude := maps.Clone(trackedKeys)
			for k := range relations {
				exclude[k] = true
			}
			unconfirmed = SymbolCandidates(deref(token.Symbol), networks, localOpps, exclude)
		}
		token.Relations = sortedRelations(relations)
		token.UnconfirmedKeys = unconfirmed
		token.Counts = model.TokenCounts{OpportunityCounts: OpportunityCounts(relations, opps), UnconfirmedCandidatesCount: len(unconfirmed)}
		token.UsefulnessScore = token.Counts.EligibleOpportunitiesCount
		token.ByNetwork = breakdown(relations, opps, byNetwork)
		token.ByCategory = breakdown(relations, opps, byCategory)
		token.ByPlatform = breakdown(relations, opps, byPlatform)
		token.Yield = YieldProfile(relations, opps)
	}

	// 4. underlyings: rank versions inside each underlying
	var underlyings []model.Underlying
	for _, snap := range snaps {
		var own, included []*model.Token
		for i := range tokens {
			if tokens[i].UnderlyingID == snap.UnderlyingID {
				own = append(own, &tokens[i])
			}
		}
		for _, t := range own {
			if !ExcludedKinds[t.Kind] {
				included = append(included, t)
			}
		}
		slices.SortStableFunc(included, RankKey)
		for rank, t := range included {
			r := rank + 1
			t.RankInUnderlying = &r
		}
		rel := mergeRelations(included)
		var rwa model.CMCRwa
		if snap.CMC != nil && snap.CMC.RWA != nil {
			rwa = *snap.CMC.RWA
		}
		coverages := map[string]bool{}
		issuerIDs := map[string]bool{}
		inDefi := 0
		for _, t := range included {
			coverages[t.Coverage] = true
			if truthy(t.IssuerID) {
				issuerIDs[*t.IssuerID] = true
			}
			if t.Counts.EligibleOpportunitiesCount > 0 {
				inDefi++
			}
		}
		coverage := "no_tokens"
		for _, c := range CoveragePriority {
			if coverages[c] {
				coverage = c
				break
			}
		}
		var searches map[string]model.PortalsSearch
		if snap.Portals != nil {
			searches = snap.Portals.Searches
		}
		searchComplete := true
		for _, m := range searches {
			searchComplete = searchComplete && m.Complete
		}
		for _, t := range included {
			searchComplete = searchComplete && symbolSearched(t.Symbol, searches)
		}
		u := model.Underlying{
			ID:                       snap.UnderlyingID,
			Ticker:                   snap.Ticker,
			Name:                     model.CollapseSpace(snap.Name),
			ISIN:                     snap.ISIN,
			AssetType:                snap.AssetType,
			Status:                   snap.Status,
			FetchedAt:                timePtr(snap.FetchedAt),
			CMCRwaID:                 rwa.RwaID,
			CMCName:                  model.NormalizeName(rwa.Name),
			TokenizedMarketCapUSD:    rwa.TokenizedMarketCap.Decimal,
			TokenizedVolume24hUSD:    rwa.TokenizedVolume24h.Decimal,
			AverageTokenizedPriceUSD: rwa.AverageTokenizedPrice.Decimal,
			TokenIDs:                 []int64{},
			ExcludedTokenIDs:         []int64{},
			VersionsCount:            len(included),
			IssuersCount:             len(issuerIDs),
			TokensInDefi:             inDefi,
			EligiblePlatform:         EligiblePlatforms(rel, opps, platformNames),
			Coverage:                 coverage,
			SearchTerms:              slices.Sorted(maps.Keys(searches)),
			SearchComplete:           searchComplete,
			OpportunityCounts:        OpportunityCounts(rel, opps),
			YieldSummary:             YieldSummary(rel, opps),
		}
		for _, t := range included {
			u.TokenIDs = append(u.TokenIDs, t.ID)
		}
		for _, t := range own {
			if ExcludedKinds[t.Kind] {
				u.ExcludedTokenIDs = append(u.ExcludedTokenIDs, t.ID)
			}
		}
		underlyings = append(underlyings, u)
	}

	built := buildIssuers(tokens, opps, issuers, underlyings, platformNames)

	used := map[string]bool{}
	for _, t := range tokens {
		for _, r := range t.Relations {
			used[r.Key] = true
		}
		for _, k := range t.UnconfirmedKeys {
			used[k] = true
		}
	}
	opportunities := make([]model.Opportunity, 0, len(used))
	for _, key := range slices.Sorted(maps.Keys(used)) {
		o := *opps[key]
		o.Components = make([]model.Component, len(o.ComponentKeys))
		for i, c := range o.ComponentKeys {
			o.Components[i] = model.Component{Key: c, Symbol: symbolsByKey[c]}
		}
		opportunities = append(opportunities, o.Opportunity)
	}

	var includedTokens []*model.Token
	for i := range tokens {
		if !ExcludedKinds[tokens[i].Kind] {
			includedTokens = append(includedTokens, &tokens[i])
		}
	}
	// A dict comprehension in the original: the LATER token's relation overwrites, whatever its depth.
	allRel := Relations{}
	deployments, indexed, inDefi := 0, 0, 0
	for _, t := range includedTokens {
		for _, r := range t.Relations {
			allRel[r.Key] = r
		}
		deployments += len(t.Deployments)
		for _, d := range t.Deployments {
			if d.Status == "indexed" {
				indexed++
			}
		}
		if t.Counts.EligibleOpportunitiesCount > 0 {
			inDefi++
		}
	}
	var oldest, newest *time.Time
	for _, s := range snaps {
		if s.FetchedAt.IsZero() {
			continue
		}
		t := s.FetchedAt
		if oldest == nil || t.Before(*oldest) {
			oldest = &t
		}
		if newest == nil || t.After(*newest) {
			newest = &t
		}
	}
	res := &Result{Side: side}
	res.Export = model.Export{
		ScoreVersion: ScoreVersion,
		Settings:     settings.ExportSettings(),
		Summary: model.Summary{
			UnderlyingsCount:  len(underlyings),
			TokensCount:       len(includedTokens),
			ExcludedTokens:    len(tokens) - len(includedTokens),
			IssuersCount:      len(built),
			DeploymentsCount:  deployments,
			IndexedDeploys:    indexed,
			TokensInDefi:      inDefi,
			OpportunityCounts: OpportunityCounts(allRel, opps),
			YieldSummary:      YieldSummary(allRel, opps),
			Coverage:          "search",
			OldestFetchedAt:   timePtrOpt(oldest),
			NewestFetchedAt:   timePtrOpt(newest),
		},
		Underlyings:   underlyings,
		Tokens:        tokens,
		Issuers:       built,
		Opportunities: opportunities,
	}
	return res, nil
}

func timePtr(t time.Time) *string {
	if t.IsZero() {
		return nil
	}
	s := FormatTime(t)
	return &s
}

func timePtrOpt(t *time.Time) *string {
	if t == nil {
		return nil
	}
	return timePtr(*t)
}

// sortedRelations is `sorted(relations.values(), key=lambda r: (r["depth"], r["key"]))`.
func sortedRelations(relations Relations) []model.Relation {
	out := slices.Collect(maps.Values(relations))
	slices.SortFunc(out, func(a, b model.Relation) int { return cmp.Or(cmp.Compare(a.Depth, b.Depth), cmp.Compare(a.Key, b.Key)) })
	return out
}

// mergeRelations merges the relations of tokens in order: the minimum depth wins, the first token wins ties.
func mergeRelations(tokens []*model.Token) Relations {
	rel := Relations{}
	for _, t := range tokens {
		for _, r := range t.Relations {
			if cur, ok := rel[r.Key]; !ok || r.Depth < cur.Depth {
				rel[r.Key] = r
			}
		}
	}
	return rel
}

// buildIssuers is analyze.py::build_issuers.
func buildIssuers(tokens []model.Token, opps map[string]*Opp, cache map[string]IssuerMeta, underlyings []model.Underlying,
	platformNames map[string]string) []model.Issuer {
	groups := map[string][]*model.Token{}
	var order []string
	for i := range tokens {
		t := &tokens[i]
		if ExcludedKinds[t.Kind] || !truthy(t.IssuerID) {
			continue
		}
		if _, ok := groups[*t.IssuerID]; !ok {
			order = append(order, *t.IssuerID)
		}
		groups[*t.IssuerID] = append(groups[*t.IssuerID], t)
	}

	// per underlying: distinct eligible opportunities per issuer (for the relative basket score)
	perUnderlying := map[string]map[string]map[string]bool{}
	for _, id := range order {
		for _, t := range groups[id] {
			if perUnderlying[t.UnderlyingID] == nil {
				perUnderlying[t.UnderlyingID] = map[string]map[string]bool{}
			}
			if perUnderlying[t.UnderlyingID][id] == nil {
				perUnderlying[t.UnderlyingID][id] = map[string]bool{}
			}
			for _, r := range t.Relations {
				if opps[r.Key].Eligibility == "eligible" {
					perUnderlying[t.UnderlyingID][id][r.Key] = true
				}
			}
		}
	}
	tickerByID := map[string]string{}
	for _, u := range underlyings {
		tickerByID[u.ID] = u.Ticker
	}

	out := make([]model.Issuer, 0, len(order))
	for _, id := range order {
		own := groups[id]
		rel := mergeRelations(own)
		var eligible []string
		for k := range rel {
			if opps[k].Eligibility == "eligible" {
				eligible = append(eligible, k)
			}
		}
		counts := OpportunityCounts(rel, opps)
		underlyingSet := map[string]bool{}
		for _, t := range own {
			underlyingSet[t.UnderlyingID] = true
		}
		underlyingIDs := slices.Sorted(maps.Keys(underlyingSet))

		var relative []float64
		for _, uid := range underlyingIDs {
			scores := perUnderlying[uid]
			best := 0
			for _, v := range scores {
				best = max(best, len(v))
			}
			if len(scores) >= 2 && best > 0 {
				relative = append(relative, float64(len(scores[id]))/float64(best)*100)
			}
		}

		var poolsLiquidity decimal.Decimal
		hasDirectPools := false
		for _, k := range eligible {
			if rel[k].Depth == 1 && opps[k].Category == "dex_lp" {
				hasDirectPools = true
				poolsLiquidity = poolsLiquidity.Add(*opps[k].LiquidityUSD.V)
			}
		}
		var marketCaps []decimal.Decimal
		for _, t := range own {
			if !t.MarketCapUSD.IsNull() {
				marketCaps = append(marketCaps, *t.MarketCapUSD.V)
			}
		}
		yields := YieldProfile(rel, opps)
		m := cache[id]
		known := m.present()
		var oldestPtr *string
		var oldest time.Time
		for _, t := range own {
			if t.FetchedAt == nil {
				continue
			}
			if ft, ok := ParseTime(*t.FetchedAt); ok && (oldestPtr == nil || ft.Before(oldest)) {
				oldest, oldestPtr = ft, t.FetchedAt
			}
		}
		name := own[0].IssuerName
		if truthy(m.Name) {
			name = m.Name
		}
		name = model.NormalizeName(name)
		status := "unknown"
		if known {
			status = "cmc"
		}
		iss := model.Issuer{
			ID:             id,
			Name:           name,
			CMCLabel:       own[0].IssuerName,
			Website:        m.Website,
			Logo:           m.Logo,
			MetadataStatus: status,
			RosterSize:     m.RosterSize,

			UnderlyingIDs: underlyingIDs,
			TokenIDs:      []int64{},

			BasketUnderlyingsCount: len(underlyingIDs),
			BasketTokensCount:      len(own),
			OpportunityCounts:      counts,
			IssuerUsefulnessScore:  len(eligible),
			EligiblePlatforms:      EligiblePlatforms(rel, opps, platformNames),
			YieldSummary:           yields.YieldSummary,
			BestReportedYieldKeys:  yields.TopKeys[:min(3, len(yields.TopKeys))],
			ByNetwork:              breakdown(rel, opps, byNetwork),
			ByCategory:             breakdown(rel, opps, byCategory),

			MeanOpportunitiesPerUnderlying: model.NewDecimal(quantizedRatio(len(eligible), len(underlyingIDs), 2)),
			MeanOpportunitiesPerToken:      model.NewDecimal(quantizedRatio(len(eligible), len(own), 2)),
			ComparedUnderlyingsCount:       len(relative),
			Coverage:                       "search",
			OldestFetchedAt:                oldestPtr,
		}
		for _, t := range own {
			iss.TokenIDs = append(iss.TokenIDs, t.ID)
			if t.Counts.EligibleOpportunitiesCount > 0 {
				iss.TokensInDefi++
			}
			iss.DeploymentsCount += len(t.Deployments)
			for _, d := range t.Deployments {
				switch d.Status {
				case "indexed", "not_indexed":
					iss.SupportedDeploymentsCount++
				case "unsupported_network", "unmapped_network":
					iss.UnsupportedDeploymentCount++
				}
			}
		}
		slices.Sort(iss.TokenIDs)
		for _, uid := range underlyingIDs {
			label, ok := tickerByID[uid]
			if !ok {
				label = uid
			}
			iss.ByUnderlying.Set(label, len(perUnderlying[uid][id]))
		}
		if iss.ByUnderlying.Values == nil {
			iss.ByUnderlying.Values = map[string]int{}
		}
		if len(relative) > 0 {
			iss.BasketRelativeScore = model.NewDecimal(pyMean(relative))
		}
		if hasDirectPools {
			iss.DirectPoolsLiquidityUSD = model.NewDecimal(poolsLiquidity.RoundBank(2))
		}
		if len(marketCaps) > 0 {
			sum := decimal.Zero
			for _, v := range marketCaps {
				sum = sum.Add(v)
			}
			iss.BasketMarketCapUSD = model.NewDecimal(sum)
		}
		out = append(out, iss)
	}
	slices.SortStableFunc(out, func(a, b model.Issuer) int {
		return cmp.Or(
			cmp.Compare(b.IssuerUsefulnessScore, a.IssuerUsefulnessScore),
			cmp.Compare(b.EligibleProtocolsCount, a.EligibleProtocolsCount),
			cmp.Compare(deref(a.Name), deref(b.Name)),
		)
	})
	return out
}

// pyMean is Decimal(str(statistics.mean(values))).quantize(Decimal("0.1")): statistics.mean over floats is the
// exact mean rounded once to the nearest float; str(float) is the shortest round-trip repr.
func pyMean(values []float64) decimal.Decimal {
	sum := new(big.Rat)
	for _, v := range values {
		sum.Add(sum, new(big.Rat).SetFloat64(v))
	}
	sum.Quo(sum, new(big.Rat).SetInt64(int64(len(values))))
	f, _ := sum.Float64()
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return decimal.Zero
	}
	return decimal.RequireFromString(strconv.FormatFloat(f, 'f', -1, 64)).RoundBank(1)
}

// validateSettings rejects settings that analyze would turn into a nil dereference or an endless/empty graph.
func validateSettings(s *model.Settings) error {
	switch {
	case s == nil:
		return errors.New("compute: settings are required")
	case s.MinEligibleLiquidityUSD.IsNull():
		return errors.New("compute: settings: min_eligible_liquidity_usd is required")
	case s.YieldAPYReviewThresholdPercent.IsNull():
		return errors.New("compute: settings: yield_apy_review_threshold_percent is required")
	case s.MaxPositionAgeHours < 1:
		return errors.New("compute: settings: max_position_age_hours must be >= 1")
	case s.MaxGraphDepth < 1:
		return errors.New("compute: settings: max_graph_depth must be >= 1")
	}
	return nil
}

// quantizedRatio is (Decimal(n) / d).quantize(10**-places) with ROUND_HALF_EVEN.
func quantizedRatio(n, d int, places int32) decimal.Decimal {
	return decimal.NewFromInt(int64(n)).DivRound(decimal.NewFromInt(int64(d)), divPrecision).RoundBank(places)
}
