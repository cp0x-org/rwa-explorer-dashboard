import { createContext, useContext, useEffect, useMemo } from 'react';
import useSWR from 'swr';

import { StatusResponse } from 'types/api';
import { Issuer, Opportunity, RelationRow, RwaExport, Token, Underlying } from 'types/rwa';

// The browser talks only to our API (and our logos); it never calls CMC or Portals. Empty VITE_API_URL = same origin
// (production, behind the reverse proxy); `http://localhost:8080` when the dev server runs next to `pnpm mock:api`.
export const API_BASE: string = (import.meta.env.VITE_API_URL ?? '').replace(/\/$/, '');

export const apiUrl = (path: string) => `${API_BASE}/api/v1/${path}`;

// The latest export, and one build of it: `?build=<id>` never changes (immutable), the server pins it in the page.
export const DATA_URL = apiUrl('export');
export const exportUrl = (buildId?: number | string) => (buildId === undefined ? DATA_URL : `${DATA_URL}?build=${buildId}`);

// Logo paths in the export are relative (`logos/tokens/1.png`): the API serves them at `/logos/...`.
export const logoUrl = (path: string | null | undefined): string | undefined => (path ? `${API_BASE}/${path}` : undefined);

// Same options for every API hook: a page left open for a day picks up new data without a reload.
export const SWR_OPTIONS = {
  refreshInterval: 10 * 60 * 1000,
  dedupingInterval: 60_000,
  revalidateOnFocus: false
} as const;

// Failed API call: `status` is the HTTP status, `code` the `error.code` of the API's error body when there is one.
export class ApiError extends Error {
  constructor(
    message: string,
    readonly status: number,
    readonly code?: string
  ) {
    super(message);
    this.name = 'ApiError';
  }
}

export async function fetchJson<T>(url: string, init?: RequestInit): Promise<T> {
  const res = await fetch(url, init);
  if (!res.ok) {
    const body = await res.json().catch(() => undefined);
    throw new ApiError(body?.error?.message ?? `HTTP ${res.status} while loading ${url}`, res.status, body?.error?.code);
  }
  return res.json();
}

// The latest export: revalidated by the browser (ETag), so an unchanged build costs a 304.
const fetcher = (url: string) => fetchJson<RwaExport>(url, { cache: 'no-cache' });

// A pinned build is immutable: the browser cache (and the <link rel="preload"> of the page) may answer it.
export const loadRwaBuild = (buildId: number | string) => fetchJson<RwaExport>(exportUrl(buildId));

export const loadRwaData = () => fetcher(DATA_URL);

// Export loaded before the first render: by the server render, and by index.tsx (the build named in <meta name="rwa-build">)
// before it hydrates a server-rendered page, so the first render shows the same data as the HTML instead of a loader.
export const RwaDataContext = createContext<RwaExport | undefined>(undefined);

export interface RwaIndex {
  data: RwaExport;
  underlyingById: Map<string, Underlying>;
  underlyingByTicker: Map<string, Underlying>;
  tokenById: Map<number, Token>;
  issuerById: Map<string, Issuer>;
  opportunityByKey: Map<string, Opportunity>;
  symbolByKey: Map<string, string>;
  platformNameBySlug: Map<string, string>; // Portals platform slug -> protocol name
}

function buildIndex(data: RwaExport): RwaIndex {
  const symbolByKey = new Map<string, string>();
  data.tokens.forEach((t) => t.deployments.forEach((d) => d.key && t.symbol && symbolByKey.set(d.key, t.symbol)));
  data.opportunities.forEach((o) => o.symbol && symbolByKey.set(o.key, o.symbol));
  const platformNameBySlug = new Map(data.opportunities.map((o) => [o.platform, o.platform_name]));
  return {
    data,
    underlyingById: new Map(data.underlyings.map((u) => [u.id, u])),
    underlyingByTicker: new Map(data.underlyings.map((u) => [u.ticker, u])),
    tokenById: new Map(data.tokens.map((t) => [t.id, t])),
    issuerById: new Map(data.issuers.map((i) => [i.id, i])),
    opportunityByKey: new Map(data.opportunities.map((o) => [o.key, o])),
    symbolByKey,
    platformNameBySlug
  };
}

// Names of the protocols with at least one eligible position of the token, alphabetically.
export const tokenProtocols = (index: RwaIndex, token: Token): string[] =>
  Object.entries(token.by_platform)
    .filter(([, v]) => v.eligible > 0)
    .map(([slug]) => index.platformNameBySlug.get(slug) ?? slug)
    .sort((a, b) => a.localeCompare(b));

const STATUS_URL = apiUrl('status');
const BUILD_CHECK_MS = 60_000;

export function useRwaData() {
  const preloaded = useContext(RwaDataContext);
  // Started from a server-rendered page, the data is the pinned build: it is the fallback, so the first render equals the
  // server's, and nothing is downloaded again on mount. A newer build is noticed by the cheap /status check below (or by
  // the 10 minute refresh) and then fetched once.
  const { data, error, isLoading, mutate } = useSWR(DATA_URL, fetcher, {
    ...SWR_OPTIONS,
    fallbackData: preloaded,
    revalidateOnMount: !preloaded,
    revalidateOnReconnect: false
  });
  const { data: status } = useSWR<StatusResponse>(preloaded ? STATUS_URL : null, (url: string) => fetchJson<StatusResponse>(url), {
    refreshInterval: BUILD_CHECK_MS,
    revalidateOnMount: false,
    revalidateOnFocus: false
  });
  const latest = status?.current_build?.id;
  const loaded = data?.build_id;
  useEffect(() => {
    if (latest !== undefined && loaded !== undefined && latest !== loaded) mutate();
  }, [latest, loaded, mutate]);
  const index = useMemo(() => (data ? buildIndex(data) : undefined), [data]);
  return { index, error: error as Error | undefined, isLoading: isLoading && !data }; // a pinned fallback is data: no loader
}

// Confirmed relations (+ optional unconfirmed symbol matches) of the given tokens as flat rows.
export function relationRows(index: RwaIndex, tokens: Token[], includeUnconfirmed = false): RelationRow[] {
  const rows: RelationRow[] = [];
  tokens.forEach((token) => {
    const underlying = index.underlyingById.get(token.underlying_id);
    token.relations.forEach((relation) => {
      const opportunity = index.opportunityByKey.get(relation.key);
      if (opportunity) rows.push({ token, underlying, opportunity, relation });
    });
    if (includeUnconfirmed) {
      token.unconfirmed_keys.forEach((key) => {
        const opportunity = index.opportunityByKey.get(key);
        if (opportunity) rows.push({ token, underlying, opportunity, relation: null });
      });
    }
  });
  return rows;
}

// Same rule as the pipeline's yield profile: eligible, positive APY, not flagged as anomaly.
export const isReportedYield = (row: RelationRow): boolean =>
  !!row.relation &&
  row.opportunity.eligibility === 'eligible' &&
  Number(row.opportunity.apy_percent ?? 0) > 0 &&
  !row.opportunity.flags.includes('apy_anomaly');
