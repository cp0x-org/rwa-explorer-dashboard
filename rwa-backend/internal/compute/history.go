package compute

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/shopspring/decimal"

	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/model"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/store"
)

// History is the set of history rows of one build, ready for store.Insert*History.
type History struct {
	Summary     store.SummaryHistoryRow
	Underlyings []store.UnderlyingHistoryRow
	Tokens      []store.TokenHistoryRow
	Issuers     []store.IssuerHistoryRow
	Positions   []store.PositionHistoryRow
	// Warnings lists token rows dropped because the same CMC id appears more than once (see dedupeTokens).
	Warnings []string
}

func historyCounts(c model.OpportunityCounts) store.HistoryCounts {
	return store.HistoryCounts{
		RawConfirmedOpportunitiesCount: c.RawConfirmedOpportunitiesCount,
		EligibleOpportunitiesCount:     c.EligibleOpportunitiesCount,
		UnknownLiquidityCount:          c.UnknownLiquidityCount,
		DirectEligibleCount:            c.DirectEligibleCount,
		IndirectEligibleCount:          c.IndirectEligibleCount,
		EligibleProtocolsCount:         c.EligibleProtocolsCount,
		EligibleNetworksCount:          c.EligibleNetworksCount,
	}
}

func historyYield(y model.YieldSummary) store.HistoryYield {
	return store.HistoryYield{
		YieldOpportunitiesCount:   y.YieldOpportunitiesCount,
		MaxReportedAPYPercent:     y.MaxReportedAPYPercent,
		AverageReportedAPYPercent: y.AverageReportedAPYPercent,
		MedianReportedAPYPercent:  y.MedianReportedAPYPercent,
	}
}

// sumNullable adds the non-null values; the sum is null when there is none (None is not 0).
func sumNullable(values ...model.Decimal) model.Decimal {
	sum, seen := decimal.Zero, false
	for _, v := range values {
		if !v.IsNull() {
			sum, seen = sum.Add(*v.V), true
		}
	}
	if !seen {
		return model.Decimal{}
	}
	return model.NewDecimal(sum)
}

// eligibleLiquidity is the summed liquidity_usd of the distinct eligible positions among keys: a position shared
// by several tokens is counted once per entity. No eligible position gives 0 (a real sum of nothing).
func eligibleLiquidity(keys map[string]bool, opps map[string]*model.Opportunity) model.Decimal {
	sum := decimal.Zero
	for k := range keys {
		if o := opps[k]; o != nil && o.Eligibility == "eligible" && !o.LiquidityUSD.IsNull() {
			sum = sum.Add(*o.LiquidityUSD.V)
		}
	}
	return model.NewDecimal(sum)
}

func relationKeys(tokens ...*model.Token) map[string]bool {
	keys := map[string]bool{}
	for _, t := range tokens {
		for _, r := range t.Relations {
			keys[r.Key] = true
		}
	}
	return keys
}

// HistoryRows maps an export to the history rows of one run. observed_at is the entity's fetched_at (the snapshot
// time, not the write time); a position's is the fetched_at of the snapshot that supplied its record (side; nil
// side falls back to the summary's newest_fetched_at). 2.0 additions: tokenized market cap and volume sums in the
// summary, eligible_liquidity_usd on summary, underlying and token rows (issuer rows carry
// direct_pools_liquidity_usd and basket_market_cap_usd instead), and breakdowns on token and issuer rows.
// eligible_liquidity_usd sums every eligible position linked to the entity, so nested positions (an LP and a
// vault holding that LP) are both summed, as specified; it is a "liquidity touching the entity" figure, not a
// de-duplicated TVL.
// HistoryRows returns every row of the export; the caller (publish) keeps only the underlying, token and position rows
// of snapshots collected by the run being published, and always the summary and issuer rows.
func HistoryRows(exp *model.Export, side *Side, runID int64) (*History, error) {
	fallback, ok := time.Time{}, false
	if exp.Summary.NewestFetchedAt != nil {
		fallback, ok = ParseTime(*exp.Summary.NewestFetchedAt)
	}
	if !ok {
		fallback, ok = ParseTime(exp.GeneratedAt)
	}
	if !ok {
		return nil, errors.New("compute: history: export has neither newest_fetched_at nor generated_at")
	}
	observed := func(s *string) time.Time {
		if s != nil {
			if t, ok := ParseTime(*s); ok {
				return t
			}
		}
		return fallback
	}

	opps := make(map[string]*model.Opportunity, len(exp.Opportunities))
	for i := range exp.Opportunities {
		opps[exp.Opportunities[i].Key] = &exp.Opportunities[i]
	}
	var included []*model.Token
	for i := range exp.Tokens {
		if !ExcludedKinds[exp.Tokens[i].Kind] {
			included = append(included, &exp.Tokens[i])
		}
	}

	h := &History{}
	keep, warnings := dedupeTokens(exp)
	h.Warnings = warnings
	var caps, vols []model.Decimal
	for _, u := range exp.Underlyings {
		caps, vols = append(caps, u.TokenizedMarketCapUSD), append(vols, u.TokenizedVolume24hUSD)
	}
	h.Summary = store.SummaryHistoryRow{
		RunID:                   runID,
		ObservedAt:              fallback,
		UnderlyingsCount:        exp.Summary.UnderlyingsCount,
		TokensCount:             exp.Summary.TokensCount,
		ExcludedTokensCount:     exp.Summary.ExcludedTokens,
		IssuersCount:            exp.Summary.IssuersCount,
		DeploymentsCount:        exp.Summary.DeploymentsCount,
		IndexedDeploymentsCount: exp.Summary.IndexedDeploys,
		TokensInDefiCount:       exp.Summary.TokensInDefi,
		HistoryCounts:           historyCounts(exp.Summary.OpportunityCounts),

		YieldOpportunitiesCount:   exp.Summary.YieldOpportunitiesCount,
		MaxReportedAPYPercent:     exp.Summary.MaxReportedAPYPercent,
		AverageReportedAPYPercent: exp.Summary.AverageReportedAPYPercent,
		MedianReportedAPYPercent:  exp.Summary.MedianReportedAPYPercent,
		TokenizedMarketCapUSD:     sumNullable(caps...),
		TokenizedVolume24hUSD:     sumNullable(vols...),
		EligibleLiquidityUSD:      eligibleLiquidity(relationKeys(included...), opps),
	}

	for _, u := range exp.Underlyings {
		var own []*model.Token
		for _, t := range included {
			if t.UnderlyingID == u.ID {
				own = append(own, t)
			}
		}
		h.Underlyings = append(h.Underlyings, store.UnderlyingHistoryRow{
			RunID:                    runID,
			UnderlyingID:             u.ID,
			ObservedAt:               observed(u.FetchedAt),
			Status:                   u.Status,
			TokenizedMarketCapUSD:    u.TokenizedMarketCapUSD,
			TokenizedVolume24hUSD:    u.TokenizedVolume24hUSD,
			AverageTokenizedPriceUSD: u.AverageTokenizedPriceUSD,
			VersionsCount:            u.VersionsCount,
			IssuersCount:             u.IssuersCount,
			TokensInDefiCount:        u.TokensInDefi,
			HistoryCounts:            historyCounts(u.OpportunityCounts),
			HistoryYield:             historyYield(u.YieldSummary),
			EligibleLiquidityUSD:     eligibleLiquidity(relationKeys(own...), opps),
		})
	}

	for i := range exp.Tokens {
		t := &exp.Tokens[i]
		if !keep[i] {
			continue // history_tokens is keyed by (token_id, run_id)
		}
		indexed := 0
		for _, d := range t.Deployments {
			if d.Status == "indexed" {
				indexed++
			}
		}
		breakdowns, err := json.Marshal(struct {
			ByNetwork  model.Breakdown `json:"by_network"`
			ByCategory model.Breakdown `json:"by_category"`
			ByPlatform model.Breakdown `json:"by_platform"`
		}{t.ByNetwork, t.ByCategory, t.ByPlatform})
		if err != nil {
			return nil, fmt.Errorf("compute: history: token %d breakdowns: %w", t.ID, err)
		}
		h.Tokens = append(h.Tokens, store.TokenHistoryRow{
			RunID:                      runID,
			TokenID:                    t.ID,
			ObservedAt:                 observed(t.FetchedAt),
			PriceUSD:                   t.PriceUSD,
			MarketCapUSD:               t.MarketCapUSD,
			Volume24hUSD:               t.Volume24hUSD,
			UsefulnessScore:            t.UsefulnessScore,
			RankInUnderlying:           t.RankInUnderlying,
			HistoryCounts:              historyCounts(t.Counts.OpportunityCounts),
			UnconfirmedCandidatesCount: t.Counts.UnconfirmedCandidatesCount,
			HistoryYield:               historyYield(t.Yield.YieldSummary),
			DeploymentsCount:           len(t.Deployments),
			IndexedDeploymentsCount:    indexed,
			EligibleLiquidityUSD:       eligibleLiquidity(relationKeys(t), opps),
			Breakdowns:                 breakdowns,
		})
	}

	for _, is := range exp.Issuers {
		breakdowns, err := json.Marshal(struct {
			ByNetwork    model.Breakdown       `json:"by_network"`
			ByCategory   model.Breakdown       `json:"by_category"`
			ByUnderlying model.OrderedMap[int] `json:"by_underlying"`
		}{is.ByNetwork, is.ByCategory, is.ByUnderlying})
		if err != nil {
			return nil, fmt.Errorf("compute: history: issuer %s breakdowns: %w", is.ID, err)
		}
		h.Issuers = append(h.Issuers, store.IssuerHistoryRow{
			RunID:                       runID,
			IssuerID:                    is.ID,
			ObservedAt:                  observed(is.OldestFetchedAt),
			IssuerUsefulnessScore:       is.IssuerUsefulnessScore,
			BasketRelativeScore:         is.BasketRelativeScore,
			ComparedUnderlyingsCount:    is.ComparedUnderlyingsCount,
			BasketUnderlyingsCount:      is.BasketUnderlyingsCount,
			BasketTokensCount:           is.BasketTokensCount,
			TokensInDefiCount:           is.TokensInDefi,
			DeploymentsCount:            is.DeploymentsCount,
			SupportedDeploymentsCount:   is.SupportedDeploymentsCount,
			UnsupportedDeploymentsCount: is.UnsupportedDeploymentCount,
			HistoryCounts:               historyCounts(is.OpportunityCounts),
			HistoryYield:                historyYield(is.YieldSummary),
			DirectPoolsLiquidityUSD:     is.DirectPoolsLiquidityUSD,
			BasketMarketCapUSD:          is.BasketMarketCapUSD,
			RosterSize:                  is.RosterSize,
			Breakdowns:                  breakdowns,
		})
	}

	for _, o := range exp.Opportunities {
		at := fallback
		if side != nil {
			if t, ok := side.PositionFetchedAt[o.Key]; ok {
				at = t
			}
		}
		h.Positions = append(h.Positions, store.PositionHistoryRow{
			RunID:            runID,
			PositionKey:      o.Key,
			ObservedAt:       at,
			LiquidityUSD:     o.LiquidityUSD,
			APYPercent:       o.APYPercent,
			BaseAPYPercent:   o.BaseAPYPercent,
			RewardAPYPercent: o.RewardAPYPercent,
			BorrowAPYPercent: o.BorrowAPYPercent,
			VolumeUSD1d:      o.VolumeUSD1d,
			VolumeUSD7d:      o.VolumeUSD7d,
			SourceUpdatedAt:  parseTimePtr(o.UpdatedAt),
			Eligibility:      o.Eligibility,
			Flags:            o.Flags,
		})
	}
	return h, nil
}

func parseTimePtr(s *string) *time.Time {
	if s == nil {
		return nil
	}
	t, ok := ParseTime(*s)
	if !ok {
		return nil
	}
	return &t
}
