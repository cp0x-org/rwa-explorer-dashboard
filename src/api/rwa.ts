import { useMemo } from 'react';
import useSWR from 'swr';

import { Issuer, Opportunity, RelationRow, RwaExport, Token, Underlying } from 'types/rwa';

// Static JSON produced by rwa-data-pipeline; the browser never calls CMC or Portals.
const DATA_URL = `${import.meta.env.BASE_URL}data/rwa.json`;

const fetcher = async (url: string): Promise<RwaExport> => {
  const res = await fetch(url, { cache: 'no-cache' });
  if (!res.ok) throw new Error(`HTTP ${res.status} while loading ${url}`);
  return res.json();
};

export interface RwaIndex {
  data: RwaExport;
  underlyingById: Map<string, Underlying>;
  underlyingByTicker: Map<string, Underlying>;
  tokenById: Map<number, Token>;
  issuerById: Map<string, Issuer>;
  opportunityByKey: Map<string, Opportunity>;
  symbolByKey: Map<string, string>;
}

function buildIndex(data: RwaExport): RwaIndex {
  const symbolByKey = new Map<string, string>();
  data.tokens.forEach((t) => t.deployments.forEach((d) => d.key && t.symbol && symbolByKey.set(d.key, t.symbol)));
  data.opportunities.forEach((o) => o.symbol && symbolByKey.set(o.key, o.symbol));
  return {
    data,
    underlyingById: new Map(data.underlyings.map((u) => [u.id, u])),
    underlyingByTicker: new Map(data.underlyings.map((u) => [u.ticker, u])),
    tokenById: new Map(data.tokens.map((t) => [t.id, t])),
    issuerById: new Map(data.issuers.map((i) => [i.id, i])),
    opportunityByKey: new Map(data.opportunities.map((o) => [o.key, o])),
    symbolByKey
  };
}

export function useRwaData() {
  const { data, error, isLoading } = useSWR(DATA_URL, fetcher, {
    revalidateIfStale: false,
    revalidateOnFocus: false,
    revalidateOnReconnect: false
  });
  const index = useMemo(() => (data ? buildIndex(data) : undefined), [data]);
  return { index, error: error as Error | undefined, isLoading };
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
