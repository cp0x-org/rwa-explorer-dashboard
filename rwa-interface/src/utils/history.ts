// Pure helpers of the history charts: ranges, metric types and formatting, axis ticks, series extraction.
import { HistoryMetricsResponse, HistoryPoint, MetricType } from 'types/api';
import { Format, MessageId, T } from 'i18n';

export type HistoryRangeOption = 30 | 90 | 365 | 'all';
export const HISTORY_RANGES: HistoryRangeOption[] = [30, 90, 365, 'all'];

const DAY_MS = 86_400_000;

// `from` of a range as a UTC date (`2026-08-21`): the same for the whole day, so the request URL (the cache key) is stable;
// "all" is an explicit early date: the API's default lower bound is a year back, so no `from` would not mean everything.
export const ALL_TIME_FROM = '2000-01-01T00:00:00Z';
export function rangeFrom(range: HistoryRangeOption, now: Date = new Date()): string {
  if (range === 'all') return ALL_TIME_FROM;
  return new Date(now.getTime() - range * DAY_MS).toISOString().slice(0, 10);
}

// Same rule as the API: *_usd / *_usd_* money, *_percent percent, *_score score, everything else a count.
export const metricTypeByName = (name: string): MetricType =>
  /_usd(_|$)/.test(name) ? 'money' : name.endsWith('_percent') ? 'percent' : name.endsWith('_score') ? 'score' : 'count';

// Types of the metrics as the API reports them (GET /history/metrics); the name rule while that has not loaded.
export function metricTypeResolver(response: HistoryMetricsResponse | undefined, entity: keyof HistoryMetricsResponse['entities']) {
  const known = new Map(response?.entities[entity]?.map((m) => [m.name, m.type]));
  return (name: string): MetricType => known.get(name) ?? metricTypeByName(name);
}

export function formatMetric(f: Format, type: MetricType, value: number | string | null | undefined): string {
  switch (type) {
    case 'money':
      return f.usd(value);
    case 'percent':
      return f.pct(value);
    default:
      return f.num(value);
  }
}

// Label of a metric: the dictionary's name when there is one, else the raw name made readable.
export const metricLabel = (t: T, metric: string): string => {
  const id = `history.metric.${metric}` as MessageId;
  const label = t(id);
  return label === id ? metric.replace(/_/g, ' ') : label;
};

export interface ChartPoint {
  time: number; // ms
  iso: string;
  run: number;
  values: (number | null)[]; // one per series, in the order of `metrics`
}

const toNumber = (v: string | number | null | undefined): number | null => {
  if (v === null || v === undefined) return null;
  const n = Number(v);
  return Number.isFinite(n) ? n : null;
};

// Points in time order with the values of the requested metrics as numbers (decimal strings parsed).
// A chart is worth showing once its points span at least this many different UTC days: points of one day are not a trend.
export const MIN_HISTORY_DAYS = 2;

// Number of different UTC days among the points.
export function historyDays(points: { iso: string }[]): number {
  return new Set(points.map((p) => p.iso.slice(0, 10))).size;
}

export function chartPoints(points: HistoryPoint[], metrics: string[]): ChartPoint[] {
  return points
    .map((p) => ({ time: Date.parse(p.observed_at), iso: p.observed_at, run: p.run_id, values: metrics.map((m) => toNumber(p.values[m])) }))
    .filter((p) => Number.isFinite(p.time))
    .sort((a, b) => a.time - b.time || a.run - b.run);
}

export interface SeriesStats {
  first: { value: number; iso: string } | null;
  last: { value: number; iso: string } | null;
  min: number | null;
  max: number | null;
}

export function seriesStats(points: ChartPoint[], index: number): SeriesStats {
  let first: SeriesStats['first'] = null;
  let last: SeriesStats['last'] = null;
  let min: number | null = null;
  let max: number | null = null;
  for (const p of points) {
    const v = p.values[index];
    if (v === null) continue;
    if (!first) first = { value: v, iso: p.iso };
    last = { value: v, iso: p.iso };
    min = min === null ? v : Math.min(min, v);
    max = max === null ? v : Math.max(max, v);
  }
  return { first, last, min, max };
}

// X axis ticks at whole UTC days, evenly spread over the time span (one tick when the span is within a day).
export function dayTicks(tMin: number, tMax: number, count: number): number[] {
  const first = Math.ceil(tMin / DAY_MS) * DAY_MS;
  const last = Math.floor(tMax / DAY_MS) * DAY_MS;
  if (first >= last) return [Math.round(tMin / DAY_MS) * DAY_MS];
  const ticks = Array.from({ length: count }, (_, i) => {
    const t = Math.round((tMin + ((tMax - tMin) * i) / (count - 1)) / DAY_MS) * DAY_MS;
    return Math.min(Math.max(t, first), last);
  });
  return [...new Set(ticks)];
}

// Round axis values covering [min, max] with about `count` steps: 1, 2, 2.5, 5 times a power of ten.
export function niceScale(min: number, max: number, count = 4): { ticks: number[]; min: number; max: number } {
  if (min === max) {
    const pad = Math.abs(min) * 0.05 || 1;
    min -= pad;
    max += pad;
  }
  const raw = (max - min) / count;
  const magnitude = 10 ** Math.floor(Math.log10(raw));
  const step = ([1, 2, 2.5, 5, 10].find((m) => m * magnitude >= raw) ?? 10) * magnitude;
  const start = Math.floor(min / step) * step;
  const end = Math.ceil(max / step) * step;
  const ticks: number[] = [];
  for (let v = start; v <= end + step / 2; v += step) ticks.push(Math.abs(v) < step * 1e-9 ? 0 : v);
  return { ticks, min: start, max: end };
}

// Text alternative of one series: first and last value with their dates, lowest and highest; null when it has no values.
export function describeSeries(t: T, f: Format, label: string, type: MetricType, points: ChartPoint[], index: number): string | null {
  const st = seriesStats(points, index);
  if (!st.first || !st.last || st.min === null || st.max === null) return null;
  const fmt = (v: number) => formatMetric(f, type, v);
  return points.length === 1
    ? t('history.summary.single', { metric: label, value: fmt(st.last.value), date: f.day(st.last.iso) })
    : t('history.summary', {
        metric: label,
        first: fmt(st.first.value),
        firstDate: f.day(st.first.iso),
        last: fmt(st.last.value),
        lastDate: f.day(st.last.iso),
        min: fmt(st.min),
        max: fmt(st.max)
      });
}

// ==============================|| CHANGE MODE ||============================== //
// "Change, %": every line is its change against its own first value in the range, so lines of any unit share one axis.

export type HistoryMode = 'values' | 'change';

// First value of every series in the points (null when a series has none).
export function seriesBases(points: ChartPoint[], count: number): (number | null)[] {
  return Array.from({ length: count }, (_, i) => points.find((p) => p.values[i] !== null)?.values[i] ?? null);
}

// Change of `value` against `base` in percent; null when either is missing or the base is zero (no ratio to it).
export function changePercent(value: number | null, base: number | null): number | null {
  if (value === null || base === null || base === 0) return null;
  return ((value - base) / Math.abs(base)) * 100;
}

// "+12.5%", "−3%", "0%": a change with its sign; whole numbers above 10 points, one decimal below.
export function formatChange(f: Format, value: number | null, digits?: number): string {
  if (value === null) return f.pct(null);
  const d = digits ?? (Math.abs(value) >= 10 ? 0 : 1);
  const rounded = Number(value.toFixed(d));
  if (rounded === 0) return f.pct(0, 0);
  return `${rounded > 0 ? '+' : '−'}${f.pct(Math.abs(rounded), d)}`;
}

// Selection of metrics after a click: off when on (the last one stays), else on; as values at most `max` lines, so one
// more switches the chart to the change mode. Returns the new selection (in the order of the clicks) and mode.
export function toggleMetric(
  selected: string[],
  metric: string,
  mode: HistoryMode,
  max: number
): { selected: string[]; mode: HistoryMode } {
  if (selected.includes(metric)) return { selected: selected.length > 1 ? selected.filter((m) => m !== metric) : selected, mode };
  const next = [...selected, metric];
  return { selected: next, mode: mode === 'values' && next.length > max ? 'change' : mode };
}

// Selection when the mode changes: back to values keeps the `max` metrics picked last.
export function selectionForMode(selected: string[], mode: HistoryMode, max: number): string[] {
  return mode === 'values' && selected.length > max ? selected.slice(-max) : selected;
}
