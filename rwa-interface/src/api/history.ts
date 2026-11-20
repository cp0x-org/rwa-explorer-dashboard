import useSWR from 'swr';

import { HistoryEntity, HistoryMetricsResponse, HistoryResponse, HistoryStep } from 'types/api';
import { apiUrl, fetchJson, SWR_OPTIONS } from './rwa';

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
  return useSWR<HistoryResponse>(clientKey(key), fetchJson, SWR_OPTIONS);
}

export const useHistoryMetrics = () => useSWR<HistoryMetricsResponse>(clientKey(historyMetricsKey()), fetchJson, SWR_OPTIONS);
