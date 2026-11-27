import { describe, expect, it } from 'vitest';

import { makeFormat } from 'i18n';
import {
  changePercent,
  chartPoints,
  formatChange,
  formatMetric,
  historyDays,
  metricTypeByName,
  niceScale,
  rangeFrom,
  selectionForMode,
  seriesBases,
  seriesStats,
  toggleMetric
} from './history';

describe('history helpers', () => {
  it('rangeFrom is a UTC date, stable within a day, and open for "all"', () => {
    const now = new Date('2026-11-20T23:59:00Z');
    expect(rangeFrom(30, now)).toBe('2026-10-21');
    expect(rangeFrom(365, now)).toBe('2025-11-20');
    expect(rangeFrom('all', now)).toBe('2000-01-01T00:00:00Z');
    expect(rangeFrom(90, new Date('2026-11-20T00:00:00Z'))).toBe(rangeFrom(90, now));
  });

  it('types metrics by name like the API does', () => {
    expect(metricTypeByName('tokenized_market_cap_usd')).toBe('money');
    expect(metricTypeByName('direct_pools_liquidity_usd')).toBe('money');
    expect(metricTypeByName('median_reported_apy_percent')).toBe('percent');
    expect(metricTypeByName('issuer_usefulness_score')).toBe('score');
    expect(metricTypeByName('eligible_opportunities_count')).toBe('count');
  });

  it('formats by type', () => {
    const f = makeFormat('en');
    expect(formatMetric(f, 'money', '1234567.8')).toBe('$1.23M');
    expect(formatMetric(f, 'percent', '4.5')).toBe('4.50%');
    expect(formatMetric(f, 'count', 1234)).toBe('1,234');
    expect(formatMetric(f, 'count', null)).toBe('—');
  });

  it('parses decimal strings, sorts by time and keeps gaps as null', () => {
    const points = chartPoints(
      [
        { run_id: 2, observed_at: '2026-10-19T00:00:00Z', values: { a: '2.5', b: null } },
        { run_id: 1, observed_at: '2026-10-18T00:00:00Z', values: { a: 1, b: 7 } }
      ],
      ['a', 'b']
    );
    expect(points.map((p) => p.run)).toEqual([1, 2]);
    expect(points[1].values).toEqual([2.5, null]);
    const stats = seriesStats(points, 1);
    expect(stats.first?.value).toBe(7);
    expect(stats.last?.value).toBe(7);
    expect(seriesStats(points, 0)).toMatchObject({ min: 1, max: 2.5 });
  });

  it('niceScale covers the data with round ticks, also for a flat series', () => {
    const s = niceScale(3, 97);
    expect(s.min).toBeLessThanOrEqual(3);
    expect(s.max).toBeGreaterThanOrEqual(97);
    expect(s.ticks.length).toBeGreaterThanOrEqual(3);
    const flat = niceScale(5, 5);
    expect(flat.min).toBeLessThan(5);
    expect(flat.max).toBeGreaterThan(5);
  });

  it('formats durations in the locale', () => {
    const f = makeFormat('en');
    expect(f.duration(4.2)).toBe('4.2s');
    expect(f.duration(540)).toBe('9m 0s');
    expect(f.duration(null)).toBe('—');
  });

  it('historyDays counts different UTC days', () => {
    expect(historyDays([])).toBe(0);
    expect(historyDays([{ iso: '2026-11-25T21:59:30Z' }, { iso: '2026-11-25T22:00:41Z' }])).toBe(1);
    expect(historyDays([{ iso: '2026-11-25T21:59:30Z' }, { iso: '2026-11-26T06:00:00Z' }])).toBe(2);
  });

  it('change mode: bases, percent change and its format', () => {
    const f = makeFormat('en');
    const pts = chartPoints(
      [
        { run_id: 1, observed_at: '2026-11-01T06:00:00Z', values: { a: null, b: '200', c: 0 } },
        { run_id: 2, observed_at: '2026-11-02T06:00:00Z', values: { a: 50, b: '150', c: 3 } },
        { run_id: 3, observed_at: '2026-11-03T06:00:00Z', values: { a: 60, b: '230', c: 4 } }
      ],
      ['a', 'b', 'c']
    );
    expect(seriesBases(pts, 3)).toEqual([50, 200, 0]);
    expect(changePercent(60, 50)).toBe(20);
    expect(changePercent(150, 200)).toBe(-25);
    expect(changePercent(4, 0)).toBeNull(); // no ratio to zero
    expect(changePercent(null, 50)).toBeNull();
    expect(changePercent(-1, -2)).toBe(50); // a negative base: up is still +
    expect(formatChange(f, 20)).toBe('+20%');
    expect(formatChange(f, -2.345)).toBe('−2.3%');
    expect(formatChange(f, 0.01)).toBe('0%');
    expect(formatChange(f, null)).toBe('—');
  });

  it('a fourth metric switches to change, back to values keeps the last three', () => {
    let st = { selected: ['a'], mode: 'values' as const } as { selected: string[]; mode: 'values' | 'change' };
    for (const m of ['b', 'c']) st = toggleMetric(st.selected, m, st.mode, 3);
    expect(st).toEqual({ selected: ['a', 'b', 'c'], mode: 'values' });
    st = toggleMetric(st.selected, 'd', st.mode, 3);
    expect(st).toEqual({ selected: ['a', 'b', 'c', 'd'], mode: 'change' });
    expect(toggleMetric(['a'], 'a', 'values', 3).selected).toEqual(['a']); // the last one stays on
    expect(toggleMetric(st.selected, 'b', st.mode, 3)).toEqual({ selected: ['a', 'c', 'd'], mode: 'change' });
    expect(selectionForMode(st.selected, 'values', 3)).toEqual(['b', 'c', 'd']);
    expect(selectionForMode(['a', 'b'], 'values', 3)).toEqual(['a', 'b']);
  });
});
