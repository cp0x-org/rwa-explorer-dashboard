import { useEffect, useRef } from 'react';
import useSWR from 'swr';

import { HistoryEntity, HistoryMetricsResponse, HistoryResponse, HistoryStep, StatusResponse } from 'types/api';
import { ApiError, apiUrl, fetchJson, SWR_OPTIONS } from './rwa';
import { statusKey } from './runs';

export interface HistoryRange {
  from?: string; // ISO date or timestamp
  to?: string;
  step?: HistoryStep;
  limit?: number;
}

// `/api/v1/history/summary?...` or `/api/v1/history/tokens/42461?...`; the metric list is part of the key.
export function historyKey(entity: HistoryEntity, id: string | number | null, metrics: string[], range: HistoryRange = {}): string {
  const query = new URLSearchParams({ metrics: metrics.join(',') });
  (['from', 'to', 'step', 'limit'] as const).forEach((k) => range[k] !== undefined && query.set(k, String(range[k])));
  const path = entity === 'summary' || id === null ? 'summary' : `${entity}/${encodeURIComponent(String(id))}`;
  return apiUrl(`history/${path}?${query}`);
}

export const historyMetricsKey = () => apiUrl('history/metrics');

// Browser only: charts render a placeholder on the server and load their history after hydration.
const clientKey = (key: string | null) => (typeof window === 'undefined' ? null : key);

export function useHistory(entity: HistoryEntity, id: string | number | null, metrics: string[], range: HistoryRange = {}) {
  const key = metrics.length ? historyKey(entity, id, metrics, range) : null;
  // History changes with a build, not by the clock: no interval refresh; a new build id in the status that the pages already
  // poll (read from the cache, no request of its own) revalidates it. The previous range stays on screen while the next loads.
  const { data: status } = useSWR<StatusResponse>(clientKey(statusKey()));
  const build = status?.current_build?.id;
  const seen = useRef(build);
  const result = useSWR<HistoryResponse>(clientKey(key), fetchJson, {
    revalidateOnFocus: false,
    dedupingInterval: SWR_OPTIONS.dedupingInterval,
    keepPreviousData: true,
    shouldRetryOnError: (error) => !(error instanceof ApiError && error.status === 404)
  });
  const { mutate } = result;
  useEffect(() => {
    if (build !== undefined && seen.current !== undefined && seen.current !== build) mutate();
    seen.current = build;
  }, [build, mutate]);
  return result;
}

export const useHistoryMetrics = () => useSWR<HistoryMetricsResponse>(clientKey(historyMetricsKey()), fetchJson, SWR_OPTIONS);
