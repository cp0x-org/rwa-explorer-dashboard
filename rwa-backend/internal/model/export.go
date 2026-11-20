package model

import "encoding/json"

// Export mirrors rwa-interface/src/types/rwa.ts (RwaExport) one to one, plus two 2.0 fields.
// Nullable fields are pointers or Decimal (null when absent); there is no omitempty, with one exception:
// Token.rank_in_underlying is optional in 1.x and omitted for excluded tokens.
type Export struct {
	SchemaVersion int            `json:"schema_version"`
	GeneratedAt   string         `json:"generated_at"`
	ScoreVersion  string         `json:"score_version"`
	Settings      ExportSettings `json:"settings"`
	Summary       Summary        `json:"summary"`
	Underlyings   []Underlying   `json:"underlyings"`
	Tokens        []Token        `json:"tokens"`
	Issuers       []Issuer       `json:"issuers"`
	Opportunities []Opportunity  `json:"opportunities"`

	// 2.0 additions; null when unknown (e.g. an export built in memory).
	BuildID *int64 `json:"build_id"`
	RunID   *int64 `json:"run_id"`
}

// ExportSettings is export.settings: the subset of settings.json that affects the numbers.
type ExportSettings struct {
	MinEligibleLiquidityUSD        Decimal `json:"min_eligible_liquidity_usd"`
	MaxPositionAgeHours            int     `json:"max_position_age_hours"`
	MaxGraphDepth                  int     `json:"max_graph_depth"`
	YieldAPYReviewThresholdPercent Decimal `json:"yield_apy_review_threshold_percent"`
}

// OpportunityCounts are the counters shared by summary, underlyings, tokens and issuers.
type OpportunityCounts struct {
	RawConfirmedOpportunitiesCount int `json:"raw_confirmed_opportunities_count"`
	EligibleOpportunitiesCount     int `json:"eligible_opportunities_count"`
	UnknownLiquidityCount          int `json:"unknown_liquidity_count"`
	DirectEligibleCount            int `json:"direct_eligible_count"`
	IndirectEligibleCount          int `json:"indirect_eligible_count"`
	EligibleProtocolsCount         int `json:"eligible_protocols_count"`
	EligibleNetworksCount          int `json:"eligible_networks_count"`
}

// TokenCounts is token.counts: OpportunityCounts plus unconfirmed candidates.
type TokenCounts struct {
	OpportunityCounts
	UnconfirmedCandidatesCount int `json:"unconfirmed_candidates_count"`
}

// YieldSummary is computed over eligible positions with a positive reported APY, anomalies excluded.
type YieldSummary struct {
	YieldOpportunitiesCount   int     `json:"yield_opportunities_count"`
	MaxReportedAPYPercent     Decimal `json:"max_reported_apy_percent"`
	AverageReportedAPYPercent Decimal `json:"average_reported_apy_percent"`
	MedianReportedAPYPercent  Decimal `json:"median_reported_apy_percent"`
}

// BreakdownCount is one breakdown bucket.
type BreakdownCount struct {
	Raw      int `json:"raw"`
	Eligible int `json:"eligible"`
}

// Breakdown maps a key (network, category, platform) to counts; key order is significant.
type Breakdown = OrderedMap[BreakdownCount]

// YieldCategory is one yield.by_category bucket.
type YieldCategory struct {
	Count   int     `json:"count"`
	Max     Decimal `json:"max"`
	Average Decimal `json:"average"`
	Median  Decimal `json:"median"`
}

// YieldProfile is token.yield.
type YieldProfile struct {
	Status string `json:"status"` // reported | no_reported_yield_in_coverage | unknown
	YieldSummary
	MaxReportedAPYKey *string                   `json:"max_reported_apy_key"`
	TopKeys           []string                  `json:"top_keys"`
	ByCategory        OrderedMap[YieldCategory] `json:"by_category"`
	MissingAPYCount   int                       `json:"missing_apy_count"`
	AnomalyCount      int                       `json:"anomaly_count"`
}

// Deployment is one token contract on one CMC network.
type Deployment struct {
	CMCNetwork      string  `json:"cmc_network"`
	Network         *string `json:"network"`
	Address         string  `json:"address"`
	Key             *string `json:"key"`
	Status          string  `json:"status"`
	PortalsSymbol   *string `json:"portals_symbol"`
	Decimals        *int    `json:"decimals"`
	PortalsPlatform *string `json:"portals_platform"`
}

// Relation links a token to a position (depth 1 direct, more indirect).
type Relation struct {
	Key   string   `json:"key"`
	Depth int      `json:"depth"`
	Path  []string `json:"path"`
}

// Summary is export.summary.
type Summary struct {
	UnderlyingsCount int `json:"underlyings_count"`
	TokensCount      int `json:"tokens_count"`
	ExcludedTokens   int `json:"excluded_tokens_count"`
	IssuersCount     int `json:"issuers_count"`
	DeploymentsCount int `json:"deployments_count"`
	IndexedDeploys   int `json:"indexed_deployments_count"`
	TokensInDefi     int `json:"tokens_in_defi_count"`
	OpportunityCounts
	YieldSummary
	Coverage        string  `json:"coverage"`
	OldestFetchedAt *string `json:"oldest_fetched_at"`
	NewestFetchedAt *string `json:"newest_fetched_at"`
}

// Underlying is one stock/ETF.
type Underlying struct {
	ID     string  `json:"id"`
	Ticker string  `json:"ticker"`
	Name   string  `json:"name"`
	ISIN   *string `json:"isin"`

	AssetType string  `json:"asset_type"`
	Status    string  `json:"status"`
	FetchedAt *string `json:"fetched_at"`
	CMCRwaID  *int    `json:"cmc_rwa_id"`
	CMCName   *string `json:"cmc_name"`

	TokenizedMarketCapUSD    Decimal `json:"tokenized_market_cap_usd"`
	TokenizedVolume24hUSD    Decimal `json:"tokenized_volume_24h_usd"`
	AverageTokenizedPriceUSD Decimal `json:"average_tokenized_price_usd"`

	TokenIDs         []int64  `json:"token_ids"`
	ExcludedTokenIDs []int64  `json:"excluded_token_ids"`
	VersionsCount    int      `json:"versions_count"`
	IssuersCount     int      `json:"issuers_count"`
	TokensInDefi     int      `json:"tokens_in_defi_count"`
	EligiblePlatform []string `json:"eligible_platforms"`
	Coverage         string   `json:"coverage"`
	SearchTerms      []string `json:"search_terms"`
	SearchComplete   bool     `json:"search_complete"`

	OpportunityCounts
	YieldSummary

	Website *string `json:"website"`
	Logo    *string `json:"logo"`
}

// Token is one tokenized version of an underlying.
type Token struct {
	ID             int64   `json:"id"`
	Symbol         *string `json:"symbol"`
	Name           *string `json:"name"`
	Kind           string  `json:"kind"` // tokenized | wrapper | derivative | unclassified
	ExcludedReason *string `json:"excluded_reason"`
	UnderlyingID   string  `json:"underlying_id"`
	Ticker         string  `json:"ticker"`
	IssuerID       *string `json:"issuer_id"`
	IssuerName     *string `json:"issuer_name"`

	PriceUSD     Decimal `json:"price_usd"`
	MarketCapUSD Decimal `json:"market_cap_usd"`
	Volume24hUSD Decimal `json:"volume_24h_usd"`

	CMCSlug         *string      `json:"cmc_slug"`
	Logo            *string      `json:"logo"`
	Deployments     []Deployment `json:"deployments"`
	Coverage        string       `json:"coverage"`
	FetchedAt       *string      `json:"fetched_at"`
	Relations       []Relation   `json:"relations"`
	UnconfirmedKeys []string     `json:"unconfirmed_keys"`
	Counts          TokenCounts  `json:"counts"`
	UsefulnessScore int          `json:"usefulness_score"`
	// RankInUnderlying is set only for included (non-excluded) tokens; nil is omitted from the JSON, as in 1.x.
	RankInUnderlying *int         `json:"rank_in_underlying"`
	ByNetwork        Breakdown    `json:"by_network"`
	ByCategory       Breakdown    `json:"by_category"`
	ByPlatform       Breakdown    `json:"by_platform"`
	Yield            YieldProfile `json:"yield"`
}

// tokenAlias drops Token's methods to avoid recursion in MarshalJSON.
type tokenAlias Token

// tokenOut shadows rank_in_underlying with an omit-when-nil field (the shallower field wins in encoding/json).
type tokenOut struct {
	tokenAlias
	RankInUnderlying *int `json:"rank_in_underlying,omitempty"`
}

// MarshalJSON omits rank_in_underlying when nil.
func (t Token) MarshalJSON() ([]byte, error) {
	return json.Marshal(tokenOut{tokenAlias: tokenAlias(t), RankInUnderlying: t.RankInUnderlying})
}

// Issuer is a tokenization issuer with its basket-level scores.
type Issuer struct {
	ID             string  `json:"id"`
	Name           *string `json:"name"`
	CMCLabel       *string `json:"cmc_label"`
	Website        *string `json:"website"`
	Logo           *string `json:"logo"`
	MetadataStatus string  `json:"metadata_status"` // cmc | unknown
	RosterSize     *int    `json:"roster_size"`

	UnderlyingIDs []string `json:"underlying_ids"`
	TokenIDs      []int64  `json:"token_ids"`

	BasketUnderlyingsCount     int `json:"basket_underlyings_count"`
	BasketTokensCount          int `json:"basket_tokens_count"`
	TokensInDefi               int `json:"tokens_in_defi_count"`
	DeploymentsCount           int `json:"deployments_count"`
	SupportedDeploymentsCount  int `json:"supported_deployments_count"`
	UnsupportedDeploymentCount int `json:"unsupported_deployments_count"`

	OpportunityCounts
	YieldSummary

	IssuerUsefulnessScore int      `json:"issuer_usefulness_score"`
	EligiblePlatforms     []string `json:"eligible_platforms"`
	BestReportedYieldKeys []string `json:"best_reported_yield_keys"`

	ByNetwork    Breakdown       `json:"by_network"`
	ByCategory   Breakdown       `json:"by_category"`
	ByUnderlying OrderedMap[int] `json:"by_underlying"`

	MeanOpportunitiesPerUnderlying Decimal `json:"mean_opportunities_per_underlying"`
	MeanOpportunitiesPerToken      Decimal `json:"mean_opportunities_per_token"`
	BasketRelativeScore            Decimal `json:"basket_relative_score"`
	ComparedUnderlyingsCount       int     `json:"compared_underlyings_count"`
	DirectPoolsLiquidityUSD        Decimal `json:"direct_pools_liquidity_usd"`
	BasketMarketCapUSD             Decimal `json:"basket_market_cap_usd"`
	Coverage                       string  `json:"coverage"`
	OldestFetchedAt                *string `json:"oldest_fetched_at"`
}

// Component is one child position/token of a composite position.
type Component struct {
	Key    string  `json:"key"`
	Symbol *string `json:"symbol"`
}

// Opportunity is one Portals position that involves a tracked token.
type Opportunity struct {
	Key               string      `json:"key"`
	Network           string      `json:"network"`
	Platform          string      `json:"platform"`
	PlatformName      string      `json:"platform_name"`
	Symbol            *string     `json:"symbol"`
	Name              *string     `json:"name"`
	Address           string      `json:"address"`
	PositionKind      string      `json:"position_kind"` // pool_id | contract
	ReceiptTokenKey   *string     `json:"receipt_token_key"`
	Category          string      `json:"category"`
	Tags              []string    `json:"tags"`
	Components        []Component `json:"components"`
	LiquidityUSD      Decimal     `json:"liquidity_usd"`
	LiquiditySemantic string      `json:"liquidity_semantics"` // pool_tvl | vault_tvl | token_metric | unknown
	APYPercent        Decimal     `json:"apy_percent"`
	BaseAPYPercent    Decimal     `json:"base_apy_percent"`
	RewardAPYPercent  Decimal     `json:"reward_apy_percent"`
	BorrowAPYPercent  Decimal     `json:"borrow_apy_percent"`
	VolumeUSD1d       Decimal     `json:"volume_usd_1d"`
	VolumeUSD7d       Decimal     `json:"volume_usd_7d"`
	IsPortable        *bool       `json:"is_portable"`
	UpdatedAt         *string     `json:"updated_at"`
	Flags             []string    `json:"flags"`
	Eligibility       string      `json:"eligibility"`
}

func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

// Normalize replaces every nil slice with an empty one, so that arrays never marshal as null
// (rwa.ts types them as arrays and the frontend calls .map/.length on them).
// Export.MarshalJSON calls it, so compute does not have to remember.
func (e *Export) Normalize() {
	e.Underlyings = nonNil(e.Underlyings)
	e.Tokens = nonNil(e.Tokens)
	e.Issuers = nonNil(e.Issuers)
	e.Opportunities = nonNil(e.Opportunities)
	for i := range e.Underlyings {
		u := &e.Underlyings[i]
		u.TokenIDs = nonNil(u.TokenIDs)
		u.ExcludedTokenIDs = nonNil(u.ExcludedTokenIDs)
		u.EligiblePlatform = nonNil(u.EligiblePlatform)
		u.SearchTerms = nonNil(u.SearchTerms)
	}
	for i := range e.Tokens {
		t := &e.Tokens[i]
		t.Deployments = nonNil(t.Deployments)
		t.Relations = nonNil(t.Relations)
		for j := range t.Relations {
			t.Relations[j].Path = nonNil(t.Relations[j].Path)
		}
		t.UnconfirmedKeys = nonNil(t.UnconfirmedKeys)
		t.Yield.TopKeys = nonNil(t.Yield.TopKeys)
	}
	for i := range e.Issuers {
		is := &e.Issuers[i]
		is.UnderlyingIDs = nonNil(is.UnderlyingIDs)
		is.TokenIDs = nonNil(is.TokenIDs)
		is.EligiblePlatforms = nonNil(is.EligiblePlatforms)
		is.BestReportedYieldKeys = nonNil(is.BestReportedYieldKeys)
	}
	for i := range e.Opportunities {
		o := &e.Opportunities[i]
		o.Tags = nonNil(o.Tags)
		o.Components = nonNil(o.Components)
		o.Flags = nonNil(o.Flags)
	}
}

// exportAlias drops Export's methods to avoid recursion in MarshalJSON.
type exportAlias Export

// MarshalJSON normalizes nil slices to [] before encoding (on a copy of the top-level struct).
func (e Export) MarshalJSON() ([]byte, error) {
	e.Normalize()
	return json.Marshal(exportAlias(e))
}
