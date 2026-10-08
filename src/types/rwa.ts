// Shape of public/data/rwa.json written by rwa-data-pipeline (`rwa-pipeline build`).
// Money, APY and liquidity are decimal strings; null means "not reported", never zero.

export type Decimal = string | null;

export type Coverage = 'search' | 'partial' | 'not_indexed' | 'unsupported_by_portals' | 'no_contracts' | 'data_unavailable' | 'no_tokens';

export type DeploymentStatus = 'indexed' | 'not_indexed' | 'unsupported_network' | 'unmapped_network' | 'not_checked' | 'data_unavailable';

export type Eligibility = 'eligible' | 'low_liquidity' | 'stale' | 'unknown_liquidity' | 'unknown_freshness' | 'category_excluded';

export type Category = 'dex_lp' | 'lending_supply' | 'vault' | 'staking' | 'yield_market' | 'wrapper' | 'unknown';

export interface Breakdown {
  [key: string]: { raw: number; eligible: number };
}

export interface OpportunityCounts {
  raw_confirmed_opportunities_count: number;
  eligible_opportunities_count: number;
  unknown_liquidity_count: number;
  direct_eligible_count: number;
  indirect_eligible_count: number;
  eligible_protocols_count: number;
  eligible_networks_count: number;
}

export interface Deployment {
  cmc_network: string;
  network: string | null;
  address: string;
  key: string | null;
  status: DeploymentStatus;
  portals_symbol: string | null;
  decimals: number | null;
  portals_platform: string | null;
}

export interface Relation {
  key: string;
  depth: number;
  path: string[];
}

export interface YieldProfile {
  status: 'reported' | 'no_reported_yield_in_coverage' | 'unknown';
  yield_opportunities_count: number;
  max_reported_apy_percent: Decimal;
  max_reported_apy_key: string | null;
  median_reported_apy_percent: Decimal;
  top_keys: string[];
  by_category: { [category: string]: { count: number; max: Decimal; median: Decimal } };
  missing_apy_count: number;
  anomaly_count: number;
}

export interface Token {
  id: number;
  symbol: string | null;
  name: string | null;
  kind: 'tokenized' | 'wrapper' | 'derivative' | 'unclassified';
  excluded_reason: string | null;
  underlying_id: string;
  ticker: string;
  issuer_id: string | null;
  issuer_name: string | null;
  price_usd: Decimal;
  market_cap_usd: Decimal;
  volume_24h_usd: Decimal;
  cmc_slug: string | null;
  logo: string | null; // path under public/, e.g. logos/tokens/42461.png
  deployments: Deployment[];
  coverage: Coverage;
  fetched_at: string | null;
  relations: Relation[];
  unconfirmed_keys: string[];
  counts: OpportunityCounts & { unconfirmed_candidates_count: number };
  usefulness_score: number;
  rank_in_underlying?: number;
  by_network: Breakdown;
  by_category: Breakdown;
  by_platform: Breakdown;
  yield: YieldProfile;
}

export interface Underlying extends OpportunityCounts {
  id: string;
  ticker: string;
  name: string;
  isin: string | null;
  asset_type: string;
  status: string;
  fetched_at: string | null;
  cmc_rwa_id: number | null;
  cmc_name: string | null;
  tokenized_market_cap_usd: Decimal;
  tokenized_volume_24h_usd: Decimal;
  average_tokenized_price_usd: Decimal;
  token_ids: number[];
  excluded_token_ids: number[];
  versions_count: number;
  issuers_count: number;
  coverage: Coverage;
  search_terms: string[];
  search_complete: boolean;
}

export interface Issuer extends OpportunityCounts {
  id: string;
  name: string | null;
  cmc_label: string | null;
  website: string | null;
  logo: string | null; // path under public/, e.g. logos/issuers/<id>.png
  metadata_status: 'cmc' | 'unknown';
  roster_size: number | null;
  underlying_ids: string[];
  token_ids: number[];
  basket_underlyings_count: number;
  basket_tokens_count: number;
  deployments_count: number;
  supported_deployments_count: number;
  unsupported_deployments_count: number;
  issuer_usefulness_score: number;
  yield_opportunities_count: number;
  best_reported_yield_keys: string[];
  by_network: Breakdown;
  by_category: Breakdown;
  by_underlying: { [ticker: string]: number };
  mean_opportunities_per_underlying: Decimal;
  mean_opportunities_per_token: Decimal;
  basket_relative_score: Decimal;
  compared_underlyings_count: number;
  direct_pools_liquidity_usd: Decimal;
  basket_market_cap_usd: Decimal;
  coverage: Coverage;
  oldest_fetched_at: string | null;
}

export interface Opportunity {
  key: string;
  network: string;
  platform: string;
  platform_name: string;
  symbol: string | null;
  name: string | null;
  address: string;
  position_kind: 'pool_id' | 'contract';
  receipt_token_key: string | null;
  category: Category;
  tags: string[];
  components: { key: string; symbol: string | null }[];
  liquidity_usd: Decimal;
  liquidity_semantics: 'pool_tvl' | 'vault_tvl' | 'token_metric' | 'unknown';
  apy_percent: Decimal;
  base_apy_percent: Decimal;
  reward_apy_percent: Decimal;
  borrow_apy_percent: Decimal;
  volume_usd_1d: Decimal;
  volume_usd_7d: Decimal;
  is_portable: boolean | null;
  updated_at: string | null;
  flags: string[];
  eligibility: Eligibility;
}

export interface RwaExport {
  schema_version: number;
  generated_at: string;
  score_version: string;
  settings: {
    min_eligible_liquidity_usd: string;
    max_position_age_hours: number;
    max_graph_depth: number;
    yield_apy_review_threshold_percent: string;
  };
  summary: OpportunityCounts & {
    underlyings_count: number;
    tokens_count: number;
    excluded_tokens_count: number;
    issuers_count: number;
    deployments_count: number;
    indexed_deployments_count: number;
    coverage: Coverage;
    oldest_fetched_at: string | null;
    newest_fetched_at: string | null;
  };
  underlyings: Underlying[];
  tokens: Token[];
  issuers: Issuer[];
  opportunities: Opportunity[];
}

// One row of the "token uses position" table
export interface RelationRow {
  token: Token;
  underlying: Underlying | undefined;
  opportunity: Opportunity;
  relation: Relation | null; // null = unconfirmed (symbol match only)
}
