package compute

import (
	"cmp"
	"slices"

	"github.com/shopspring/decimal"

	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/model"
)

// divPrecision is the number of fractional digits kept before quantizing a quotient. Python's decimal context keeps
// 28 significant digits; every quotient here is quantized to at most 4 places afterwards, so 30 is ample.
const divPrecision = 30

// breakdown is analyze.py::_breakdown: counts per field value over the given relations, sorted by
// (-eligible, -raw, key). An empty field value is bucketed as "unknown".
func breakdown(relations Relations, opps map[string]*Opp, field func(*Opp) string) model.Breakdown {
	type bucket struct {
		key           string
		raw, eligible int
	}
	index := map[string]*bucket{}
	var list []*bucket
	for k := range relations {
		o := opps[k]
		name := field(o)
		if name == "" {
			name = "unknown"
		}
		b := index[name]
		if b == nil {
			b = &bucket{key: name}
			index[name] = b
			list = append(list, b)
		}
		b.raw++
		if o.Eligibility == "eligible" {
			b.eligible++
		}
	}
	slices.SortFunc(list, func(a, b *bucket) int {
		return cmp.Or(cmp.Compare(b.eligible, a.eligible), cmp.Compare(b.raw, a.raw), cmp.Compare(a.key, b.key))
	})
	var out model.Breakdown
	for _, b := range list {
		out.Set(b.key, model.BreakdownCount{Raw: b.raw, Eligible: b.eligible})
	}
	if out.Values == nil {
		out.Values = map[string]model.BreakdownCount{}
	}
	return out
}

func byNetwork(o *Opp) string  { return o.Network }
func byCategory(o *Opp) string { return o.Category }
func byPlatform(o *Opp) string { return o.Platform }

// mean is analyze.py::_mean: the arithmetic mean quantized to 0.0001 (ROUND_HALF_EVEN).
func mean(values []decimal.Decimal) decimal.Decimal {
	sum := decimal.Zero
	for _, v := range values {
		sum = sum.Add(v)
	}
	return sum.DivRound(decimal.NewFromInt(int64(len(values))), divPrecision).RoundBank(4)
}

// median is statistics.median: the middle value, or the mean of the two middle ones for an even count.
func median(values []decimal.Decimal) decimal.Decimal {
	s := slices.Clone(values)
	slices.SortFunc(s, func(a, b decimal.Decimal) int { return a.Cmp(b) })
	n := len(s)
	if n%2 == 1 {
		return s[n/2]
	}
	return s[n/2-1].Add(s[n/2]).DivRound(decimal.NewFromInt(2), divPrecision)
}

// apyStats is analyze.py::_apy_stats over positions that have an APY.
func apyStats(list []*Opp) model.YieldCategory {
	values := make([]decimal.Decimal, len(list))
	for i, o := range list {
		values[i] = *o.APYPercent.V
	}
	maxV := slices.MaxFunc(values, func(a, b decimal.Decimal) int { return a.Cmp(b) })
	return model.YieldCategory{
		Count:   len(values),
		Max:     model.NewDecimal(maxV),
		Average: model.NewDecimal(mean(values)),
		Median:  model.NewDecimal(median(values)),
	}
}

// YieldProfile is analyze.py::yield_profile: computed over eligible positions with a positive reported APY,
// anomalies excluded.
func YieldProfile(relations Relations, opps map[string]*Opp) model.YieldProfile {
	var eligible, withAPY []*Opp
	missing, anomalies := 0, 0
	for k := range relations {
		o := opps[k]
		if o.Eligibility != "eligible" {
			continue
		}
		eligible = append(eligible, o)
		if o.APYPercent.IsNull() {
			missing++
		}
		anomaly := contains(o.Flags, "apy_anomaly")
		if anomaly {
			anomalies++
		}
		if !o.APYPercent.IsNull() && o.APYPercent.V.IsPositive() && !anomaly {
			withAPY = append(withAPY, o)
		}
	}
	slices.SortFunc(withAPY, func(a, b *Opp) int {
		return cmp.Or(b.APYPercent.V.Cmp(*a.APYPercent.V), cmp.Compare(a.Key, b.Key))
	})
	profile := model.YieldProfile{
		MissingAPYCount: missing,
		AnomalyCount:    anomalies,
		TopKeys:         []string{},
	}
	switch {
	case len(withAPY) > 0:
		profile.Status = "reported"
	case len(eligible) > 0:
		profile.Status = "no_reported_yield_in_coverage"
	default:
		profile.Status = "unknown"
	}
	profile.ByCategory.Values = map[string]model.YieldCategory{}
	if len(withAPY) == 0 {
		return profile
	}
	overall := apyStats(withAPY)
	profile.YieldOpportunitiesCount = len(withAPY)
	profile.MaxReportedAPYPercent = withAPY[0].APYPercent
	profile.MaxReportedAPYKey = &withAPY[0].Key
	// Mean over positions with a positive reported APY (what the site's summary charts show), never over zeros.
	profile.AverageReportedAPYPercent = overall.Average
	profile.MedianReportedAPYPercent = overall.Median
	for _, o := range withAPY[:min(5, len(withAPY))] {
		profile.TopKeys = append(profile.TopKeys, o.Key)
	}
	groups := map[string][]*Opp{}
	var order []string
	for _, o := range withAPY {
		if _, ok := groups[o.Category]; !ok {
			order = append(order, o.Category)
		}
		groups[o.Category] = append(groups[o.Category], o)
	}
	for _, c := range order {
		profile.ByCategory.Set(c, apyStats(groups[c]))
	}
	return profile
}

// YieldSummary is analyze.py::yield_summary: the flat yield fields of an aggregate (underlying, issuer, basket).
func YieldSummary(relations Relations, opps map[string]*Opp) model.YieldSummary {
	return YieldProfile(relations, opps).YieldSummary
}

// EligiblePlatforms is analyze.py::eligible_platforms: distinct protocol names behind eligible positions
// (200 Uniswap V3 pools count once, V2 and V3 separately), sorted.
func EligiblePlatforms(relations Relations, opps map[string]*Opp, platformNames map[string]string) []string {
	slugs := map[string]bool{}
	for k := range relations {
		if opps[k].Eligibility == "eligible" {
			slugs[opps[k].Platform] = true
		}
	}
	names := make([]string, 0, len(slugs))
	for p := range slugs {
		name := p
		if n, ok := platformNames[p]; ok && n != "" {
			name = n
		}
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// OpportunityCounts is analyze.py::opportunity_counts.
func OpportunityCounts(relations Relations, opps map[string]*Opp) model.OpportunityCounts {
	c := model.OpportunityCounts{RawConfirmedOpportunitiesCount: len(relations)}
	protocols, networks := map[string]bool{}, map[string]bool{}
	for k, r := range relations {
		o := opps[k]
		switch o.Eligibility {
		case "unknown_liquidity":
			c.UnknownLiquidityCount++
		case "eligible":
			c.EligibleOpportunitiesCount++
			if r.Depth == 1 {
				c.DirectEligibleCount++
			} else if r.Depth > 1 {
				c.IndirectEligibleCount++
			}
			protocols[o.Platform] = true
			networks[o.Network] = true
		}
	}
	c.EligibleProtocolsCount = len(protocols)
	c.EligibleNetworksCount = len(networks)
	return c
}

// RankKey is analyze.py::rank_key as a comparison: more eligible positions, protocols, networks and direct
// positions first, then the lower token id.
func RankKey(a, b *model.Token) int {
	return cmp.Or(
		cmp.Compare(b.Counts.EligibleOpportunitiesCount, a.Counts.EligibleOpportunitiesCount),
		cmp.Compare(b.Counts.EligibleProtocolsCount, a.Counts.EligibleProtocolsCount),
		cmp.Compare(b.Counts.EligibleNetworksCount, a.Counts.EligibleNetworksCount),
		cmp.Compare(b.Counts.DirectEligibleCount, a.Counts.DirectEligibleCount),
		cmp.Compare(a.ID, b.ID),
	)
}
