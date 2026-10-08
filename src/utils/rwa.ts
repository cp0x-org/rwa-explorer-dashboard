import { formatShortUSDS } from 'utils/formatters';

// null / undefined = not reported -> "—"; a real zero stays "0".
export const DASH = '—';

export const fmtUsd = (value: string | null | undefined): string => {
  if (value === null || value === undefined) return DASH;
  const num = Number(value);
  if (!Number.isFinite(num)) return DASH;
  return num < 0 ? `-$${formatShortUSDS(-num)}` : `$${formatShortUSDS(num)}`;
};

export const fmtPct = (value: string | null | undefined, digits = 2): string => {
  if (value === null || value === undefined) return DASH;
  const num = Number(value);
  return Number.isFinite(num) ? `${num.toLocaleString('en-US', { maximumFractionDigits: digits, minimumFractionDigits: digits })}%` : DASH;
};

export const fmtNum = (value: number | string | null | undefined): string =>
  value === null || value === undefined ? DASH : Number(value).toLocaleString('en-US');

export const fmtDate = (iso: string | null | undefined): string => {
  if (!iso) return DASH;
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return DASH;
  return `${d.toLocaleString('en-US', { timeZone: 'UTC', year: 'numeric', month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit', hour12: false })} UTC`;
};

export const fmtAge = (iso: string | null | undefined, now: Date = new Date()): string => {
  if (!iso) return DASH;
  const minutes = Math.round((now.getTime() - new Date(iso).getTime()) / 60000);
  if (!Number.isFinite(minutes)) return DASH;
  if (minutes < 60) return `${Math.max(minutes, 0)}m ago`;
  if (minutes < 48 * 60) return `${Math.round(minutes / 60)}h ago`;
  return `${Math.round(minutes / 1440)}d ago`;
};

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

export const networkName = (network: string | null | undefined): string => (network ? (NETWORK_NAMES[network] ?? network) : DASH);

export const CATEGORY_LABELS: Record<string, string> = {
  dex_lp: 'DEX LP',
  lending_supply: 'Lending supply',
  vault: 'Vault',
  staking: 'Staking',
  yield_market: 'Yield market',
  wrapper: 'Wrapper',
  unknown: 'Unknown'
};

export const categoryLabel = (category: string): string => CATEGORY_LABELS[category] ?? category;

type ChipColor = 'default' | 'success' | 'warning' | 'error' | 'info' | 'primary' | 'secondary';

export const COVERAGE: Record<string, { label: string; color: ChipColor; hint: string }> = {
  search: { label: 'Search coverage', color: 'info', hint: 'Positions found via Portals text search and confirmed by contract address' },
  partial: { label: 'Partial fetch', color: 'warning', hint: 'Some deployments were not checked in the last run' },
  not_indexed: { label: 'Not indexed', color: 'warning', hint: 'Supported network, but Portals does not index this token' },
  unsupported_by_portals: {
    label: 'Unsupported by Portals',
    color: 'default',
    hint: 'All deployments are on networks Portals does not cover: DeFi usage is unknown, not zero'
  },
  no_contracts: { label: 'No contracts', color: 'default', hint: 'CMC lists no contract addresses for this token' },
  data_unavailable: { label: 'Data unavailable', color: 'error', hint: 'Source data could not be fetched' },
  no_tokens: { label: 'No tokenized versions', color: 'default', hint: 'CMC tracks no tokenized versions of this asset' }
};

export const DEPLOYMENT_STATUS: Record<string, { label: string; color: ChipColor }> = {
  indexed: { label: 'Indexed', color: 'success' },
  not_indexed: { label: 'Not indexed', color: 'warning' },
  unsupported_network: { label: 'Unsupported by Portals', color: 'default' },
  unmapped_network: { label: 'Unmapped network', color: 'default' },
  not_checked: { label: 'Not checked', color: 'warning' },
  data_unavailable: { label: 'Data unavailable', color: 'error' }
};

export const ELIGIBILITY: Record<string, { label: string; color: ChipColor; hint: string }> = {
  eligible: { label: 'Eligible', color: 'success', hint: 'Confirmed use-case, fresh data, enough liquidity' },
  low_liquidity: { label: 'Low liquidity', color: 'default', hint: 'Liquidity below the eligibility threshold' },
  stale: { label: 'Stale', color: 'warning', hint: 'Source data older than the freshness limit at collection time' },
  unknown_liquidity: { label: 'Unknown liquidity', color: 'default', hint: 'Portals reports no comparable liquidity for this position' },
  unknown_freshness: { label: 'Unknown freshness', color: 'default', hint: 'No source timestamp' },
  category_excluded: { label: 'Not a use-case', color: 'default', hint: 'Wrapper or unclassified position: kept as raw only' },
  unconfirmed: {
    label: 'Unconfirmed',
    color: 'default',
    hint: 'Symbol matches, but the position composition does not contain the token address'
  }
};

export const UNDERLYING_STATUS: Record<string, string> = {
  ok: 'OK',
  not_in_cmc: 'Not tracked by CMC',
  no_tracked_tokens: 'No tokenized versions',
  ambiguous_cmc_match: 'Ambiguous CMC match',
  rwa_id_changed: 'CMC id changed'
};

export const FLAG_LABELS: Record<string, string> = {
  sentinel_price: 'Pool position, not an ERC-20 share (no share price)',
  range_details_unknown: 'Concentrated liquidity: range details unknown',
  negative_volume_net_flow: 'Negative volume reported (net flow)',
  apy_anomaly: 'APY above review threshold: excluded from yield stats',
  apy_negative: 'Negative APY reported',
  apy_missing: 'No APY reported',
  liquidity_missing: 'No liquidity reported'
};

export const EXCLUDED_REASONS: Record<string, string> = {
  derivative: 'Derivative, not a tokenized security',
  missing_symbol: 'No symbol in CMC data',
  symbol_does_not_match_ticker: 'Symbol does not match the underlying ticker (other share class?)'
};
