import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import useSWR from 'swr';

import { RunDetail, RunEvent, RunEventLevel, RunEventsResponse, RunsResponse, StatusResponse } from 'types/api';
import { ApiError, apiUrl, fetchJson, SWR_OPTIONS } from './rwa';

// These hooks fetch in the browser only (nothing is fetched while rendering on the server): the pages that use them
// render a placeholder first, so the server-rendered markup does not depend on them.
const clientKey = (key: string | null) => (typeof window === 'undefined' ? null : key);

export const statusKey = () => apiUrl('status');
export const runsKey = (limit = 50) => apiUrl(`runs?limit=${limit}`);
export const runKey = (id: number) => apiUrl(`runs/${id}`);

export const useStatus = () => useSWR<StatusResponse>(clientKey(statusKey()), fetchJson, SWR_OPTIONS);
export const useRuns = (limit = 50) => useSWR<RunsResponse>(clientKey(runsKey(limit)), fetchJson, SWR_OPTIONS);
export const RUN_POLL_MS = 5000;

// 4xx is the caller's mistake (an unknown run stays unknown, a bad parameter stays bad): do not retry it.
export const noRetryOn4xx = (error: unknown) => !(error instanceof ApiError && error.status >= 400 && error.status < 500);

// A run is live while its journal is open: the job writes events also after the status left `running`, until the terminal
// `journal closed` event. Older servers have no `journal_open`: `running` then.
const isLive = (v: { journal_open?: boolean; status?: string; runStatus?: string } | undefined) =>
  v !== undefined && (v.journal_open ?? (v.status ?? v.runStatus) === 'running');

// SWR evaluates a `refreshInterval` function only when its timer is set up (not when the data arrives), and restarts the timer
// whenever the option changes identity. So the interval is a plain number, flipped by `live` (a state: the data is known only
// after the first answer), and the other options are module-level constants: re-rendering (a toggle, a filter) leaves the timer alone.
const RUN_OPTIONS = { ...SWR_OPTIONS, dedupingInterval: 1000, shouldRetryOnError: noRetryOn4xx };
const LIVE_OPTIONS = { refreshInterval: RUN_POLL_MS };

export const useRun = (id: number | undefined) => {
  const current = useRef<RunDetail | undefined>(undefined); // the last answer says the run is live: ask the server, not the browser cache (max-age=30)
  const fetcher = useCallback(async (url: string) => {
    const run = await fetchJson<RunDetail>(url, isLive(current.current) ? { cache: 'no-store' } : undefined);
    current.current = run;
    return run;
  }, []);
  const [live, setLive] = useState(false);
  const options = useMemo(() => (live ? { ...RUN_OPTIONS, ...LIVE_OPTIONS } : RUN_OPTIONS), [live]);
  const swr = useSWR<RunDetail>(clientKey(id === undefined ? null : runKey(id)), fetcher, options);
  const now = isLive(swr.data);
  useEffect(() => setLive(now), [now]);
  return swr;
};

export const EVENTS_PAGE = 500;
export const runEventsKey = (id: number, level: RunEventLevel, limit = EVENTS_PAGE) =>
  apiUrl(`runs/${id}/events?level=${level}&limit=${limit}`);

export interface RunEventsState {
  key: string; // the request this state answers (the level): differs from the current one while a level change loads
  runStatus: RunEventsResponse['run_status'];
  journalOpen: boolean; // the job still writes events: keep polling
  items: RunEvent[]; // everything loaded so far, in id order
  nextAfter: number | null; // not null: the page was full, more events follow
}

const EVENTS_OPTIONS = {
  ...SWR_OPTIONS,
  refreshInterval: 0, // no timer for a closed journal; see LIVE_OPTIONS
  dedupingInterval: 1000,
  revalidateOnReconnect: false,
  keepPreviousData: true, // a level change keeps the old rows (and the toolbar) on screen until the new ones arrive
  shouldRetryOnError: noRetryOn4xx
};

// The journal of one run. The cursor is `after=<last id seen>`: every response is appended to what is already there (the
// events of a run are append-only), so a refresh costs only the new lines. While the journal is open (`journal_open`, or
// `running` for a server without it) the hook polls every 5 s and a full page is followed by the next one at once; once it
// is closed there is no timer and `loadMore()` fetches the next page. A level change is a new list. `after` is where to
// start (default: the beginning).
export function useRunEvents(runId: number | undefined, { level = 'debug', after = 0 }: { level?: RunEventLevel; after?: number } = {}) {
  const cursors = useRef(new Map<string, RunEventsState>());
  const key = runId === undefined ? null : runEventsKey(runId, level);
  const fetcher = useCallback(
    async (url: string): Promise<RunEventsState> => {
      const have = cursors.current.get(url);
      const last = have?.items.length ? have.items[have.items.length - 1].id : after;
      const page = await fetchJson<RunEventsResponse>(
        last > 0 ? `${url}&after=${last}` : url,
        have?.journalOpen ? { cache: 'no-store' } : undefined
      );
      const state: RunEventsState = {
        key: url,
        runStatus: page.run_status,
        journalOpen: isLive({ journal_open: page.journal_open, runStatus: page.run_status }),
        items: have ? [...have.items, ...page.items.filter((e) => e.id > last)] : page.items,
        nextAfter: page.next_after
      };
      cursors.current.set(url, state);
      return state;
    },
    [after]
  );
  const [live, setLive] = useState(false);
  const options = useMemo(() => (live ? { ...EVENTS_OPTIONS, ...LIVE_OPTIONS } : EVENTS_OPTIONS), [live]);
  const { data, error, isValidating, mutate } = useSWR<RunEventsState>(clientKey(key), fetcher, options);
  const open = data?.journalOpen === true;
  useEffect(() => setLive(open), [open]);
  const loadMore = useCallback(() => mutate(), [mutate]);
  // Catch-up: a full page of a live journal is followed by the next one at once, not after the next tick.
  const catchUp = data?.journalOpen === true && data.nextAfter !== null && data.key === key && !isValidating && !error;
  useEffect(() => {
    if (catchUp) void mutate();
  }, [catchUp, data?.nextAfter, mutate]);
  return { data, error, isValidating, loadMore, stale: data !== undefined && data.key !== key };
}
