import { MessageId, T } from 'i18n';

export { DASH } from 'i18n';

// Sorting helper: nulls always last regardless of direction.
export const num = (value: string | number | null | undefined): number | null => {
  if (value === null || value === undefined) return null;
  const n = Number(value);
  return Number.isFinite(n) ? n : null;
};

export function compareNullable(a: number | string | null, b: number | string | null, order: 'asc' | 'desc'): number {
  if (a === null && b === null) return 0;
  if (a === null) return 1;
  if (b === null) return -1;
  const result = typeof a === 'string' && typeof b === 'string' ? a.localeCompare(b) : Number(a) - Number(b);
  return order === 'asc' ? result : -result;
}

// Proper names of networks: the same in every language.
export const NETWORK_NAMES: Record<string, string> = {
  ethereum: 'Ethereum',
  bsc: 'BNB Chain',
  base: 'Base',
  arbitrum: 'Arbitrum',
  robinhood: 'Robinhood Chain',
  hyperevm: 'HyperEVM',
  avalanche: 'Avalanche',
  polygon: 'Polygon',
  optimism: 'Optimism',
  sonic: 'Sonic',
  plasma: 'Plasma',
  flare: 'Flare'
};

export const networkName = (network: string | null | undefined): string => (network ? (NETWORK_NAMES[network] ?? network) : '—');

// Labels of enumerated data values live in the dictionaries (`category.dex_lp`, ...); an unknown value is shown as is.
const CATEGORIES = new Set(['dex_lp', 'lending_supply', 'vault', 'staking', 'yield_market', 'wrapper', 'unknown']);
export const categoryLabel = (t: T, category: string): string =>
  CATEGORIES.has(category) ? t(`category.${category}` as MessageId) : category;

export type ChipColor = 'default' | 'success' | 'warning' | 'error' | 'info' | 'primary' | 'secondary';

// `search` (the normal case: positions found via Portals search and confirmed by address) has no chip, only problems are shown.
const COVERAGE_COLOR: Record<string, ChipColor> = {
  partial: 'warning',
  not_indexed: 'warning',
  unsupported_by_portals: 'default',
  no_contracts: 'default',
  data_unavailable: 'error',
  no_tokens: 'default'
};
export const coverageChip = (t: T, coverage: string): { label: string; color: ChipColor; hint: string } =>
  coverage in COVERAGE_COLOR
    ? { label: t(`coverage.${coverage}` as MessageId), color: COVERAGE_COLOR[coverage], hint: t(`coverage.${coverage}.hint` as MessageId) }
    : { label: coverage, color: 'default', hint: '' };

const DEPLOYMENT_COLOR: Record<string, ChipColor> = {
  indexed: 'success',
  not_indexed: 'warning',
  unsupported_network: 'default',
  unmapped_network: 'default',
  not_checked: 'warning',
  data_unavailable: 'error'
};
export const deploymentChip = (t: T, status: string): { label: string; color: ChipColor } =>
  status in DEPLOYMENT_COLOR
    ? { label: t(`deployment.${status}` as MessageId), color: DEPLOYMENT_COLOR[status] }
    : { label: status, color: 'default' };

const ELIGIBILITY_COLOR: Record<string, ChipColor> = {
  eligible: 'success',
  low_liquidity: 'default',
  stale: 'warning',
  unknown_liquidity: 'default',
  unknown_freshness: 'default',
  category_excluded: 'default',
  unconfirmed: 'default'
};
export const eligibilityChip = (t: T, eligibility: string): { label: string; color: ChipColor; hint: string } =>
  eligibility in ELIGIBILITY_COLOR
    ? {
        label: t(`eligibility.${eligibility}` as MessageId),
        color: ELIGIBILITY_COLOR[eligibility],
        hint: t(`eligibility.${eligibility}.hint` as MessageId)
      }
    : { label: eligibility, color: 'default', hint: '' };

const UNDERLYING_STATUSES = new Set(['ok', 'not_in_cmc', 'no_tracked_tokens', 'ambiguous_cmc_match', 'rwa_id_changed']);
export const underlyingStatusLabel = (t: T, status: string): string =>
  UNDERLYING_STATUSES.has(status) ? t(`underlyingStatus.${status}` as MessageId) : status;

const FLAGS = new Set([
  'sentinel_price',
  'range_details_unknown',
  'negative_volume_net_flow',
  'apy_anomaly',
  'apy_negative',
  'apy_missing',
  'liquidity_missing'
]);
export const flagLabel = (t: T, flag: string): string => (FLAGS.has(flag) ? t(`flag.${flag}` as MessageId) : flag);

const EXCLUDED = new Set(['derivative', 'missing_symbol', 'symbol_does_not_match_ticker']);
export const excludedReason = (t: T, reason: string | null | undefined): string =>
  reason ? (EXCLUDED.has(reason) ? t(`excluded.${reason}` as MessageId) : reason) : '';
