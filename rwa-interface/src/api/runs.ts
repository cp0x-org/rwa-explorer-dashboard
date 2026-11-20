import useSWR from 'swr';

import { RunDetail, RunsResponse, StatusResponse } from 'types/api';
import { ApiError, apiUrl, fetchJson, SWR_OPTIONS } from './rwa';

// These hooks fetch in the browser only (nothing is fetched while rendering on the server): the pages that use them
// render a placeholder first, so the server-rendered markup does not depend on them.
const clientKey = (key: string | null) => (typeof window === 'undefined' ? null : key);

export const statusKey = () => apiUrl('status');
export const runsKey = (limit = 50) => apiUrl(`runs?limit=${limit}`);
export const runKey = (id: number) => apiUrl(`runs/${id}`);

export const useStatus = () => useSWR<StatusResponse>(clientKey(statusKey()), fetchJson, SWR_OPTIONS);
export const useRuns = (limit = 50) => useSWR<RunsResponse>(clientKey(runsKey(limit)), fetchJson, SWR_OPTIONS);
export const useRun = (id: number | undefined) =>
  useSWR<RunDetail>(clientKey(id === undefined ? null : runKey(id)), fetchJson, {
    ...SWR_OPTIONS,
    shouldRetryOnError: (error) => !(error instanceof ApiError && error.status === 404) // an unknown run stays unknown
  });
