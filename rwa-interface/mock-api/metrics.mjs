// Metrics the mock's /history endpoints accept: every numeric column of the *_history tables of the backend schema
// (rwa-backend/internal/db/migrations/0001_init.sql; src/api/mockMetrics.test.ts compares this list with the SQL).
// Type by name: *_usd / *_usd_* money, *_percent percent, *_score score, everything else a count.

const COMMON_COUNTS = [
  'raw_confirmed_opportunities_count',
  'eligible_opportunities_count',
  'unknown_liquidity_count',
  'direct_eligible_count',
  'indirect_eligible_count',
  'eligible_protocols_count',
  'eligible_networks_count',
  'yield_opportunities_count'
];
const APY = ['max_reported_apy_percent', 'average_reported_apy_percent', 'median_reported_apy_percent'];

export const COLUMNS = {
  summary: [
    'underlyings_count',
    'tokens_count',
    'excluded_tokens_count',
    'issuers_count',
    'deployments_count',
    'indexed_deployments_count',
    'tokens_in_defi_count',
    ...COMMON_COUNTS,
    ...APY,
    'tokenized_market_cap_usd',
    'tokenized_volume_24h_usd',
    'eligible_liquidity_usd'
  ],
  underlyings: [
    'tokenized_market_cap_usd',
    'tokenized_volume_24h_usd',
    'average_tokenized_price_usd',
    'versions_count',
    'issuers_count',
    'tokens_in_defi_count',
    ...COMMON_COUNTS,
    ...APY,
    'eligible_liquidity_usd'
  ],
  tokens: [
    'price_usd',
    'market_cap_usd',
    'volume_24h_usd',
    'usefulness_score',
    'rank_in_underlying',
    ...COMMON_COUNTS,
    'unconfirmed_candidates_count',
    ...APY,
    'deployments_count',
    'indexed_deployments_count',
    'eligible_liquidity_usd'
  ],
  issuers: [
    'issuer_usefulness_score',
    'basket_relative_score',
    'compared_underlyings_count',
    'basket_underlyings_count',
    'basket_tokens_count',
    'tokens_in_defi_count',
    'deployments_count',
    'supported_deployments_count',
    'unsupported_deployments_count',
    ...COMMON_COUNTS,
    ...APY,
    'direct_pools_liquidity_usd',
    'basket_market_cap_usd',
    'roster_size'
  ],
  opportunities: [
    'liquidity_usd',
    'apy_percent',
    'base_apy_percent',
    'reward_apy_percent',
    'borrow_apy_percent',
    'volume_usd_1d',
    'volume_usd_7d'
  ]
};

export const metricType = (name) =>
  /_usd(_|$)/.test(name) ? 'money' : name.endsWith('_percent') ? 'percent' : name.endsWith('_score') ? 'score' : 'count';
