import { readFileSync } from 'node:fs';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { HistoryMetricsResponse, HistoryResponse, RunDetail, RunEvent, RunEventsResponse, RunsResponse, StatusResponse } from 'types/api';
import { RwaExport } from 'types/rwa';
import { historyKey, historyMetricsKey } from './history';
import { DATA_URL, exportUrl, logoUrl } from './rwa';
import { runKey, runsKey, statusKey } from './runs';

import status from './__fixtures__/status.json';
import runs from './__fixtures__/runs.json';
import run2 from './__fixtures__/run-2.json';
import run3 from './__fixtures__/run-3.json';
import runEvents from './__fixtures__/run-events.json';
import historySummary from './__fixtures__/history-summary.json';
import historyToken from './__fixtures__/history-token.json';
import historyMetrics from './__fixtures__/history-metrics.json';
import error400 from './__fixtures__/error-400.json';

const LEVELS: RunEvent['level'][] = ['debug', 'info', 'warn', 'error'];
const STATUSES: RunEventsResponse['run_status'][] = ['running', 'succeeded', 'partial', 'failed'];

// A JSON fixture cannot be assigned to RunEventsResponse (`level` is a string there): check the shape at run time and
// return the typed value.
function asRunEvents(v: unknown): RunEventsResponse {
  const r = v as Record<string, unknown>;
  expect(typeof r.run_id).toBe('number');
  expect(STATUSES).toContain(r.run_status);
  expect(typeof r.journal_open).toBe('boolean');
  expect(r.next_after === null || typeof r.next_after === 'number').toBe(true);
  expect(Array.isArray(r.items)).toBe(true);
  const items = (r.items as Record<string, unknown>[]).map((e) => {
    expect(typeof e.id).toBe('number');
    expect(typeof e.ts).toBe('string');
    expect(LEVELS).toContain(e.level);
    expect(typeof e.step).toBe('string');
    expect(typeof e.message).toBe('string');
    expect(e.attrs !== null && typeof e.attrs === 'object' && !Array.isArray(e.attrs)).toBe(true);
    return e as unknown as RunEvent;
  });
  return { ...(r as unknown as RunEventsResponse), items };
}

// Fixtures are responses captured from `pnpm mock:api`; assigning them to the types is the check (tsc, and vitest
// for the values the types cannot express).
describe('response types match the mock API fixtures', () => {
  it('status', () => {
    const s: StatusResponse = status;
    expect(s.current_build?.id).toBe(1);
    expect(s.last_run?.status).toBe('partial');
    expect(s.schedule).toEqual({ cron: expect.any(String), interval: null });
  });

  it('runs and run detail', () => {
    const list: RunsResponse = runs;
    expect(list.items.map((r) => r.mode)).toEqual(['rebuild', 'refresh', 'refresh', 'import']);
    expect(list.items.map((r) => r.trigger)).toEqual(['manual', 'schedule', 'schedule', 'import']);
    expect(list.items[2].changes?.eligible_delta).toBe(8);
    const refreshed: RunDetail = run2;
    expect(refreshed.changes?.new_tokens).toHaveLength(2);
    expect(Object.keys(refreshed.changes?.details ?? {})).toEqual(['eligible_by_network', 'eligible_by_platform', 'reclassified']); // as compute/changes.go writes them
    const partial: RunDetail = run3;
    expect(partial.underlyings.filter((u) => u.status === 'failed')).toHaveLength(1);
    expect(partial.warnings).toHaveLength(partial.warnings_count);
    const journal = asRunEvents(runEvents);
    expect(journal.journal_open).toBe(false);
    expect(refreshed.journal_open).toBe(false);
    expect(journal.run_id).toBe(partial.id);
    expect(journal.items).toHaveLength(partial.events_count);
    expect(journal.items.filter((e) => e.level === 'error')).toHaveLength(partial.errors_count);
    expect(journal.items.map((e) => e.id)).toEqual([...journal.items.map((e) => e.id)].sort((a, b) => a - b)); // id order: the cursor relies on it
    expect(journal.next_after).toBeNull();
  });

  it('history', () => {
    const summary: HistoryResponse = historySummary as HistoryResponse;
    expect(summary.id).toBeNull();
    expect(summary.points.every((p) => summary.metrics.every((m) => m in p.values))).toBe(true);
    const token: HistoryResponse = historyToken as HistoryResponse;
    expect(token.id).toBe('36992');
    const metrics: HistoryMetricsResponse = historyMetrics as HistoryMetricsResponse;
    expect(Object.keys(metrics.entities).sort()).toEqual(['issuers', 'opportunities', 'summary', 'tokens', 'underlyings']);
    expect(error400.error).toMatchObject({ code: 'unknown_metric', metric: 'x' });
    expect(error400.error.allowed).toContain('tokens_count');
    // money and APY are decimal strings, counts and scores numbers
    for (const h of [summary, token]) {
      for (const point of h.points) {
        for (const [name, value] of Object.entries(point.values)) {
          if (value === null) continue;
          const decimal = /_usd|_percent/.test(name);
          expect(typeof value, name).toBe(decimal ? 'string' : 'number');
        }
      }
    }
  });

  it('the export gains build_id and run_id', () => {
    const rwa: RwaExport = {
      ...JSON.parse(readFileSync(new URL('../../public/data/rwa.json', import.meta.url), 'utf8')),
      build_id: 1,
      run_id: 1
    };
    expect(rwa.build_id).toBe(1);
  });
});

describe('urls and SWR keys', () => {
  afterEach(() => {
    vi.unstubAllEnvs();
    vi.resetModules();
  });

  it('exportUrl pins a build', () => {
    expect(DATA_URL).toBe('/api/v1/export');
    expect(exportUrl()).toBe('/api/v1/export');
    expect(exportUrl(42)).toBe('/api/v1/export?build=42');
  });

  it('logoUrl keeps the relative path of the export under the API origin', () => {
    expect(logoUrl('logos/tokens/1.png')).toBe('/logos/tokens/1.png');
    expect(logoUrl(null)).toBeUndefined();
  });

  it('VITE_API_URL is the API base, without a trailing slash', async () => {
    vi.stubEnv('VITE_API_URL', 'http://localhost:8080/');
    vi.resetModules();
    const rwa = await import('./rwa');
    expect(rwa.DATA_URL).toBe('http://localhost:8080/api/v1/export');
    expect(rwa.exportUrl(3)).toBe('http://localhost:8080/api/v1/export?build=3');
    expect(rwa.logoUrl('logos/issuers/a.png')).toBe('http://localhost:8080/logos/issuers/a.png');
  });

  it('keys of the data hooks', () => {
    expect(statusKey()).toBe('/api/v1/status');
    expect(runsKey(20)).toBe('/api/v1/runs?limit=20');
    expect(runKey(3)).toBe('/api/v1/runs/3');
    expect(historyMetricsKey()).toBe('/api/v1/history/metrics');
    expect(historyKey('summary', null, ['a', 'b'])).toBe('/api/v1/history/summary?metrics=a%2Cb');
    expect(historyKey('tokens', 36992, ['market_cap_usd'], { step: 'week', from: '2026-10-01' })).toBe(
      '/api/v1/history/tokens/36992?metrics=market_cap_usd&from=2026-10-01&step=week'
    );
    expect(historyKey('opportunities', 'arbitrum:0xab', ['apy_percent'])).toBe(
      '/api/v1/history/opportunities/arbitrum%3A0xab?metrics=apy_percent'
    );
  });
});
