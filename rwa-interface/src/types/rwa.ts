// Shape of GET /api/v1/export (rwa-backend; the 1.x public/data/rwa.json of rwa-data-pipeline plus build_id and run_id).
// Money, APY and liquidity are decimal strings; null means "not reported", never zero.

export type Decimal = string | null;

export interface Referral {
  url: string;
  deal: string;
  bonus: string | null;
  page_url: string | null;
  as_of: string;
}

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

// Over eligible positions with a positive reported APY (zero and missing APY never enter the average), anomalies excluded.
export interface YieldSummary {
  yield_opportunities_count: number;
  max_reported_apy_percent: Decimal;
  average_reported_apy_percent: Decimal;
  median_reported_apy_percent: Decimal;
}

export interface YieldProfile extends YieldSummary {
  status: 'reported' | 'no_reported_yield_in_coverage' | 'unknown';
  max_reported_apy_key: string | null;
  top_keys: string[];
  by_category: { [category: string]: { count: number; max: Decimal; average: Decimal; median: Decimal } };
  missing_apy_count: number;
  anomaly_count: number;
}

export interface RightsEvidence {
  quote: string;
  url: string;
  title: string;
  doc_date: string;
  retrieved_at: string;
}

export interface RightsFact {
  value: string;
  who?: string;
  countries?: string[];
  zones?: string[];
  us_persons?: boolean;
  note?: { en: string; zh: string };
  evidence?: RightsEvidence[];
  checked?: string[];
}

export interface RightsProfile {
  id: string;
  issuer_id: string;
  product: string;
  token_ids: number[];
  scope_basis: 'per_token_documents' | 'programme_documents';
  scope_evidence?: RightsEvidence[];
  reviewed_at: string;
  stale: boolean;
  legal_issuer: {
    name: string;
    country?: string;
    zone?: string;
    note?: { en: string; zh: string };
    evidence?: RightsEvidence[];
  };
  facts: Record<string, RightsFact>;
  documents: { url: string; title: string; doc_date: string; type?: string }[];
}

export interface RightsSchema {
  who: string[];
  facts: { key: string; group: string; values: string[]; needs_who: string[] }[];
}

export interface Token {
  rights_profile_id?: string | null;
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
  links?: { kind: 'swap'; label: string | null; url: string; network: string }[];
}

export interface Underlying extends OpportunityCounts, YieldSummary {
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
  tokens_in_defi_count: number; // tokenized assets with at least one eligible position
  eligible_platforms: string[]; // distinct protocol names behind eligible positions
  coverage: Coverage;
  search_terms: string[];
  search_complete: boolean;
  website: string | null; // official site from CMC RWA metadata
  logo: string | null; // path under public/, e.g. logos/underlyings/<ISIN>.png
}

export interface Issuer extends OpportunityCounts, YieldSummary {
  rights_profile_ids?: string[];
  rights_reviewed_tokens_count?: number;
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
  tokens_in_defi_count: number; // tokenized assets with at least one eligible position
  deployments_count: number;
  supported_deployments_count: number;
  unsupported_deployments_count: number;
  issuer_usefulness_score: number;
  eligible_platforms: string[]; // distinct protocol names behind eligible positions
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
  app?: { url: string; label: string | null } | null;
  referral?: Referral | null;
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
  link?: { url: string; kind: 'position' | 'home' } | null;
  referral?: Referral | null;
}

export interface RwaExport {
  build_id: number; // build of the API this document belongs to (`/api/v1/export?build=<id>` returns the same document)
  run_id: number; // collector run that produced it
  rights_profiles?: RightsProfile[];
  rights_schema?: RightsSchema;
  schema_version: number;
  generated_at: string;
  score_version: string;
  settings: {
    min_eligible_liquidity_usd: string;
    max_position_age_hours: number;
    max_graph_depth: number;
    yield_apy_review_threshold_percent: string;
  };
  summary: OpportunityCounts &
    YieldSummary & {
      underlyings_count: number;
      tokens_count: number;
      excluded_tokens_count: number;
      issuers_count: number;
      deployments_count: number;
      indexed_deployments_count: number;
      tokens_in_defi_count: number;
      coverage: Coverage;
      oldest_fetched_at: string | null;
      newest_fetched_at: string | null;
      rights_profiles_count?: number;
      rights_reviewed_tokens_count?: number;
      rights_review_valid_days?: number | null;
      rights_excluded_profiles_count?: number;
      rights_warnings_count?: number;
      links_deals_fetched_at?: string | null;
      links_deals_max_age_days?: number | null;
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
