import { KeyboardEvent, MouseEvent, useEffect, useMemo, useRef, useState } from 'react';
import { useTheme } from '@mui/material/styles';
import Box from '@mui/material/Box';
import Chip from '@mui/material/Chip';
import ToggleButton from '@mui/material/ToggleButton';
import ToggleButtonGroup from '@mui/material/ToggleButtonGroup';
import Typography from '@mui/material/Typography';

import { useHistory, useHistoryMetrics } from 'api/history';
import useHydrated from 'hooks/useHydrated';
import { HistoryEntity, HistoryStep, MetricType } from 'types/api';
import VisuallyHidden from 'ui-component/VisuallyHidden';
import {
  changePercent,
  ChartPoint,
  chartPoints,
  dayTicks,
  describeSeries,
  formatChange,
  formatMetric,
  HISTORY_RANGES,
  HistoryMode,
  HistoryRangeOption,
  historyDays,
  MIN_HISTORY_DAYS,
  metricLabel,
  metricTypeResolver,
  niceScale,
  rangeFrom,
  selectionForMode,
  seriesBases,
  toggleMetric
} from 'utils/history';
import { Format, MessageId, T, useFormat, useT } from 'i18n';
import { ChartCard, LegendKey, useSeriesColors } from './Charts';

// A history chart is loaded in the browser only: the server (and the first client render that hydrates its markup) shows
// the card with its controls and a placeholder of the chart's height, so the server-rendered HTML needs no history request
// and both renders are the same. After hydration the series are fetched and drawn.

const CHART_H = 240; // height of the drawing area; every state of the card keeps at least this height
const MARGIN = { top: 12, right: 16, bottom: 28 };
const MAX_SERIES = 3; // lines as values, each on a scale of its own; more are drawn as change in % on one axis
const DEFAULT_W = 640;
const DOT_LIMIT = 60; // above this many points the dots are shown only for the active one

interface Series {
  metric: string;
  index: number; // position in the `metrics` prop: fixes the colour of the metric
  color: string;
  label: string;
  type: MetricType;
}

const tabular = { fontVariantNumeric: 'tabular-nums' } as const;

// ==============================|| LINE CHART ||============================== //

interface LineChartProps {
  title: string;
  points: ChartPoint[];
  series: Series[];
  mode: HistoryMode;
  bases: (number | null)[]; // first value of every metric in the range (by Series.index): the 0% of the change mode
  t: T;
  f: Format;
}

function LineChart({ title, points, series, mode, bases, t, f }: LineChartProps) {
  const theme = useTheme();
  const wrapRef = useRef<HTMLDivElement>(null);
  const groups = useRef<(SVGGElement | null)[]>([]);
  const [width, setWidth] = useState(DEFAULT_W);
  const [hover, setHover] = useState<number | null>(null);
  const [focused, setFocused] = useState<number | null>(null);
  const [tabStop, setTabStop] = useState<number | null>(null); // the one point in the tab order (roving tabindex)

  // a new set of points (another range or metric list) starts with nothing hovered or focused
  useEffect(() => {
    setHover(null);
    setFocused(null);
  }, [points]);

  useEffect(() => {
    const el = wrapRef.current;
    if (!el) return;
    const measure = () => setWidth(Math.max(240, Math.floor(el.clientWidth)));
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(el);
    return () => observer.disconnect();
  }, []);

  // the first line has the axis on the left; every further line is drawn on a scale of its own (see the caption)
  const left = 64;
  const plotW = width - left - MARGIN.right;
  const plotH = CHART_H - MARGIN.top - MARGIN.bottom;
  const n = points.length;
  const tMin = points[0].time;
  const tMax = points[n - 1].time;
  const x = (time: number) => left + (tMax === tMin ? plotW / 2 : ((time - tMin) / (tMax - tMin)) * plotW);

  const change = mode === 'change';
  // the drawn value: the metric itself, or its change in % against the first value of the range
  const plotted = (p: ChartPoint, s: Series) => (change ? changePercent(p.values[s.index], bases[s.index]) : p.values[s.index]);
  const toScale = (values: number[]) => {
    const scale = values.length ? niceScale(Math.min(...values), Math.max(...values)) : { ticks: [0, 1], min: 0, max: 1 };
    return { ...scale, y: (v: number) => MARGIN.top + plotH - ((v - scale.min) / (scale.max - scale.min)) * plotH };
  };
  const valuesOf = (s: Series) => points.map((p) => plotted(p, s)).filter((v): v is number => v !== null);
  // values: one scale per line (the first has the axis); change: one axis for every line, 0% always on it
  const shared = change ? toScale([0, ...series.flatMap(valuesOf)]) : null;
  const scales = series.map((s) => shared ?? toScale(valuesOf(s)));
  const axisDigits = change && scales[0].ticks.some((v) => !Number.isInteger(v)) ? 1 : 0;
  const axisLabel = (tick: number) => (change ? formatChange(f, tick, axisDigits) : formatMetric(f, series[0].type, tick));
  // "$1.2M" or "$1.2M (+12%)"
  const shownValue = (p: ChartPoint, s: Series) => {
    const v = formatMetric(f, s.type, p.values[s.index]);
    const c = change ? plotted(p, s) : null;
    return c === null ? v : `${v} (${formatChange(f, c)})`;
  };

  const lastIndex = n - 1;
  const inRange = (i: number | null) => (i !== null && i < n ? i : null);
  const active = inRange(focused) ?? inRange(hover);
  const stop = tabStop !== null && tabStop < n ? tabStop : lastIndex;

  // whole UTC days; the year is written on the first tick and where it changes
  const xTicks = dayTicks(tMin, tMax, Math.max(2, Math.min(6, Math.floor(plotW / 100))));
  const tickLabel = (time: number, i: number) => {
    const iso = new Date(time).toISOString();
    const newYear = i === 0 || new Date(time).getUTCFullYear() !== new Date(xTicks[i - 1]).getUTCFullYear();
    return newYear ? f.day(iso) : f.monthDay(iso);
  };

  const valueText = (p: ChartPoint) => series.map((s) => t('history.value', { metric: s.label, value: shownValue(p, s) })).join(', ');

  const onMove = (e: MouseEvent<SVGSVGElement>) => {
    const rect = e.currentTarget.getBoundingClientRect();
    const px = e.clientX - rect.left;
    let best = 0;
    for (let i = 1; i < n; i++) if (Math.abs(x(points[i].time) - px) < Math.abs(x(points[best].time) - px)) best = i;
    setHover(best);
  };

  const onKey = (i: number) => (e: KeyboardEvent<SVGGElement>) => {
    const target = { ArrowRight: i + 1, ArrowLeft: i - 1, Home: 0, End: lastIndex }[e.key];
    if (target === undefined) return;
    e.preventDefault();
    groups.current[Math.min(Math.max(target, 0), lastIndex)]?.focus();
  };

  const paths = series.map((s, k) => {
    let d = '';
    let pen = false;
    for (const p of points) {
      const v = plotted(p, s);
      if (v === null) {
        pen = false;
        continue;
      }
      d += `${pen ? 'L' : 'M'}${x(p.time).toFixed(1)} ${scales[k].y(v).toFixed(1)}`;
      pen = true;
    }
    return d;
  });

  const activePoint = active === null ? null : points[active];
  const activeX = activePoint ? x(activePoint.time) : 0;
  const flip = activeX > width * 0.55; // the tooltip opens towards the middle of the chart

  return (
    <Box ref={wrapRef} sx={{ position: 'relative', width: '100%', minHeight: CHART_H }}>
      <svg
        width={width}
        height={CHART_H}
        viewBox={`0 0 ${width} ${CHART_H}`}
        role="group"
        aria-label={t('history.chart', { title, points: t('history.points', { n }) })}
        data-history-chart=""
        style={{ display: 'block', maxWidth: '100%', overflow: 'visible' }}
        onMouseMove={onMove}
        onMouseLeave={() => setHover(null)}
      >
        {/* grid and y axis of the first line */}
        {scales[0].ticks.map((tick) => (
          <g key={tick}>
            <line
              x1={left}
              x2={width - MARGIN.right}
              y1={scales[0].y(tick)}
              y2={scales[0].y(tick)}
              stroke={change && tick === 0 ? theme.palette.text.secondary : theme.palette.divider}
            />
            <text
              x={left - 8}
              y={scales[0].y(tick)}
              textAnchor="end"
              dominantBaseline="middle"
              fontSize={11}
              fill={theme.palette.text.secondary}
              style={tabular}
              aria-hidden="true"
            >
              {axisLabel(tick)}
            </text>
          </g>
        ))}
        {xTicks.map((time, i) => (
          <text
            key={i}
            x={Math.min(Math.max(x(time), left + 28), width - MARGIN.right - 28)}
            y={CHART_H - 8}
            textAnchor="middle"
            fontSize={11}
            fill={theme.palette.text.secondary}
            style={tabular}
            aria-hidden="true"
          >
            {tickLabel(time, i)}
          </text>
        ))}

        {paths.map((d, k) => (
          <path
            key={series[k].metric}
            d={d}
            fill="none"
            stroke={series[k].color}
            strokeWidth={2}
            strokeLinejoin="round"
            strokeLinecap="round"
          />
        ))}

        {activePoint && (
          <line
            x1={activeX}
            x2={activeX}
            y1={MARGIN.top}
            y2={MARGIN.top + plotH}
            stroke={theme.palette.text.secondary}
            strokeDasharray="3 3"
            pointerEvents="none"
          />
        )}

        {/* one stop per update, in time order: arrow keys move between them (one Tab stop for the whole chart) */}
        {points.map((p, i) => {
          const px = x(p.time);
          const slot = Math.max(plotW / Math.max(n - 1, 1), 8);
          return (
            <g
              key={p.run}
              ref={(el) => {
                groups.current[i] = el;
              }}
              role="img"
              tabIndex={i === stop ? 0 : -1}
              aria-label={t('history.point', { date: f.date(p.iso), values: valueText(p) })}
              onFocus={() => {
                setFocused(i);
                setTabStop(i);
              }}
              onBlur={() => setFocused(null)}
              onKeyDown={onKey(i)}
              style={{ outline: 'none' }}
            >
              <rect x={px - slot / 2} y={MARGIN.top} width={slot} height={plotH} fill="transparent" />
              {focused === i && (
                <rect
                  x={px - slot / 2}
                  y={MARGIN.top}
                  width={slot}
                  height={plotH}
                  fill="none"
                  stroke={theme.palette.secondary.main}
                  strokeWidth={2}
                  rx={4}
                  pointerEvents="none"
                />
              )}
              {series.map((s, k) => {
                const v = plotted(p, s);
                if (v === null) return null;
                const isActive = active === i;
                if (!isActive && n > DOT_LIMIT) return null;
                return (
                  <circle
                    key={s.metric}
                    cx={px}
                    cy={scales[k].y(v)}
                    r={isActive ? 5 : 3}
                    fill={s.color}
                    stroke={theme.palette.background.default}
                    strokeWidth={2}
                  />
                );
              })}
            </g>
          );
        })}
      </svg>

      {activePoint && (
        <Box
          aria-hidden="true"
          sx={{
            position: 'absolute',
            top: 0,
            left: activeX,
            transform: `translateX(${flip ? 'calc(-100% - 12px)' : '12px'})`,
            pointerEvents: 'none',
            zIndex: 1,
            padding: '8px 12px',
            borderRadius: '8px',
            backgroundColor: 'background.paper',
            border: '1px solid',
            borderColor: 'divider',
            boxShadow: 3,
            whiteSpace: 'nowrap',
            ...tabular
          }}
        >
          <Typography variant="caption" color="text.secondary" component="div">
            {f.date(activePoint.iso)}
          </Typography>
          {series.map((s) => (
            <Box key={s.metric} sx={{ display: 'flex', alignItems: 'center', gap: 0.75 }}>
              <Box sx={{ width: 8, height: 8, borderRadius: '50%', backgroundColor: s.color }} />
              <Typography variant="body2" component="span">
                {s.label}: <strong>{shownValue(activePoint, s)}</strong>
              </Typography>
            </Box>
          ))}
        </Box>
      )}
    </Box>
  );
}

// ==============================|| VISIBILITY ||============================== //

// Whether a history block should be hidden: its history (at the default range) spans fewer than MIN_HISTORY_DAYS days,
// which includes no history at all. Until the answer is in (server render, loading, an error) nothing is hidden, and once
// a block has been shown it stays (a narrower range may hold one day only). The request is the same as the chart's first
// one, so SWR shares it.
export function useHistoryHidden(
  entity: HistoryEntity,
  id: string | number | null,
  metrics: string[],
  defaultRange: HistoryRangeOption = 90,
  step: HistoryStep = 'day'
): boolean {
  const hydrated = useHydrated();
  const from = useMemo(() => rangeFrom(defaultRange), [defaultRange]);
  const { data, error } = useHistory(entity, id, metrics, { from, step });
  const days = hydrated && data !== undefined && !error ? historyDays(chartPoints(data.points, metrics)) : null;
  const [shownOnce, setShownOnce] = useState(false);
  useEffect(() => {
    if (days !== null && days >= MIN_HISTORY_DAYS) setShownOnce(true);
  }, [days]);
  return days !== null && days < MIN_HISTORY_DAYS && !shownOnce;
}

// ==============================|| CARD ||============================== //

export interface HistoryChartProps {
  entity: HistoryEntity;
  id: string | number | null; // null for the summary
  metrics: string[]; // metrics offered; the first is shown at first. Up to 3 as values, any number as change in %
  title: string;
  subtitle?: string;
  defaultRange?: HistoryRangeOption;
  step?: HistoryStep;
  level?: 'h2' | 'h3';
  gutterBottom?: boolean; // space below the card (kept out of the page when the card is hidden)
}

// Line chart of the metrics of an entity over time, with a range selector and, when there are several metrics, toggle
// chips and a mode: values (up to three lines, each on a scale of its own) or change in % since the start of the range
// (any number of lines on one axis; a fourth metric switches to it, back to values keeps the three picked last). Text
// alternatives: every update is a focusable point with a full label, and a hidden summary gives the
// first and last value, the lowest and the highest (and, as change, the change over the range).
export default function HistoryChart({
  entity,
  id,
  metrics,
  title,
  subtitle,
  defaultRange = 90,
  step = 'day',
  level = 'h2',
  gutterBottom = false
}: HistoryChartProps) {
  const t = useT();
  const f = useFormat();
  const hydrated = useHydrated();
  const colors = useSeriesColors();
  const [range, setRange] = useState<HistoryRangeOption>(defaultRange);
  const [selected, setSelected] = useState<string[]>(() => metrics.slice(0, 1)); // in the order of the clicks
  const [mode, setMode] = useState<HistoryMode>('values');
  const from = useMemo(() => rangeFrom(range), [range]);
  const { data, error, isValidating } = useHistory(entity, id, metrics, { from, step });
  const { data: meta } = useHistoryMetrics();
  const typeOf = useMemo(() => metricTypeResolver(meta, entity), [meta, entity]);
  const hidden = useHistoryHidden(entity, id, metrics, defaultRange, step);

  const all: Series[] = metrics.map((metric, index) => ({
    metric,
    index,
    color: colors[index % colors.length],
    label: metricLabel(t, metric),
    type: typeOf(metric)
  }));
  const shown = all.filter((s) => selected.includes(s.metric));
  const points = useMemo(() => (data ? chartPoints(data.points, metrics) : []), [data, metrics]);

  const bases = useMemo(() => seriesBases(points, metrics.length), [points, metrics.length]);

  const toggle = (metric: string) => {
    const next = toggleMetric(selected, metric, mode, MAX_SERIES);
    setSelected(next.selected);
    setMode(next.mode);
  };
  const changeMode = (next: HistoryMode) => {
    setMode(next);
    setSelected((current) => selectionForMode(current, next, MAX_SERIES));
  };
  const change = mode === 'change';
  const noBase = change ? shown.filter((s) => bases[s.index] === null || bases[s.index] === 0) : [];

  // an error belongs to the request of the current range: the previous range's data is not drawn under it
  const failed = hydrated && !!error;
  const ready = hydrated && data !== undefined && !error;
  const summaries = ready
    ? shown
        .map((s) => {
          const text = describeSeries(t, f, s.label, s.type, points, s.index);
          const last = [...points].reverse().find((p) => p.values[s.index] !== null);
          const total = change && last ? changePercent(last.values[s.index], bases[s.index]) : null;
          return text && total !== null
            ? `${text} ${t('history.summary.change', { metric: s.label, change: formatChange(f, total) })}`
            : text;
        })
        .filter(Boolean)
    : [];

  let body;
  if (!ready) {
    body = (
      <Typography variant="body2" color="text.secondary" role="status">
        {failed ? t('history.error') : t('history.loading')}
      </Typography>
    );
  } else if (points.length === 0) {
    body = (
      <Typography variant="body2" color="text.secondary" role="status">
        {t('history.empty')}
      </Typography>
    );
  } else if (points.length === 1) {
    body = (
      <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, flexWrap: 'wrap' }}>
        <svg width={shown.length * 20 + 8} height={20} aria-hidden="true" focusable="false">
          {shown.map((s, k) => (
            <circle key={s.metric} cx={14 + k * 20} cy={10} r={5} fill={s.color} />
          ))}
        </svg>
        <Typography variant="body2" color="text.secondary">
          {t('history.startsOn', { date: f.day(points[0].iso) })}
        </Typography>
      </Box>
    );
  } else {
    body = <LineChart title={title} points={points} series={shown} mode={mode} bases={bases} t={t} f={f} />;
  }

  if (hidden) return null;

  const card = (
    <ChartCard
      title={title}
      subtitle={subtitle}
      level={level}
      action={
        <ToggleButtonGroup
          exclusive
          size="small"
          value={range}
          onChange={(_e, value: HistoryRangeOption | null) => value !== null && setRange(value)}
          aria-label={t('history.range.label')}
        >
          {HISTORY_RANGES.map((r) => (
            <ToggleButton key={r} value={r} sx={{ textTransform: 'none', px: 1.5 }}>
              {t(`history.range.${r}` as MessageId)}
            </ToggleButton>
          ))}
        </ToggleButtonGroup>
      }
    >
      <Box
        aria-busy={!failed && (!ready || isValidating)}
        data-history-state={
          failed ? 'error' : !ready ? 'loading' : points.length === 0 ? 'empty' : points.length === 1 ? 'single' : 'chart'
        }
      >
        {metrics.length > 1 && (
          <Box
            sx={{ display: 'flex', gap: 1, flexWrap: 'wrap', alignItems: 'flex-start', justifyContent: 'space-between', marginBottom: 2 }}
          >
            <Box role="group" aria-label={t('history.metrics.label')} sx={{ display: 'flex', gap: 1, flexWrap: 'wrap', flex: '1 1 auto' }}>
              {all.map((s) => {
                const on = selected.includes(s.metric);
                return (
                  <Chip
                    key={s.metric}
                    label={s.label}
                    size="small"
                    clickable
                    variant={on ? 'filled' : 'outlined'}
                    aria-pressed={on}
                    onClick={() => toggle(s.metric)}
                    icon={
                      <Box
                        aria-hidden="true"
                        sx={{ width: 10, height: 10, borderRadius: '50%', backgroundColor: s.color, ml: '8px !important' }}
                      />
                    }
                    sx={{ '&.Mui-focusVisible': { outline: '2px solid', outlineColor: 'secondary.main', outlineOffset: 2 } }}
                  />
                );
              })}
            </Box>
            <ToggleButtonGroup
              exclusive
              size="small"
              value={mode}
              onChange={(_e, value: HistoryMode | null) => value !== null && changeMode(value)}
              aria-label={t('history.mode.label')}
            >
              <ToggleButton value="values" sx={{ textTransform: 'none', px: 1.5 }}>
                {t('history.mode.values')}
              </ToggleButton>
              <ToggleButton value="change" sx={{ textTransform: 'none', px: 1.5 }}>
                {t('history.mode.change')}
              </ToggleButton>
            </ToggleButtonGroup>
          </Box>
        )}
        <Box sx={{ minHeight: CHART_H }}>{body}</Box>
        {summaries.length > 0 && <VisuallyHidden component="p">{summaries.join(' ')}</VisuallyHidden>}
        <Box
          component="ul"
          aria-label={t('legend.title')}
          sx={{ display: 'flex', gap: 2, flexWrap: 'wrap', listStyle: 'none', margin: 0, marginTop: 1.5, padding: 0 }}
        >
          {shown.map((s) => (
            <li key={s.metric}>
              <LegendKey color={s.color} label={s.label} />
            </li>
          ))}
        </Box>
        {ready && points.length > 1 && !change && shown.length > 1 && (
          <Typography variant="caption" color="text.secondary" component="p" sx={{ marginTop: 0.5 }}>
            {t('history.leftAxis', { metric: shown[0].label })}
            {shown.length === MAX_SERIES && metrics.length > MAX_SERIES && ` ${t('history.limit', { max: MAX_SERIES })}`}
          </Typography>
        )}
        {ready && points.length > 1 && change && (
          <Typography variant="caption" color="text.secondary" component="p" sx={{ marginTop: 0.5 }} data-history-mode-note="">
            {t('history.changeAxis', { date: f.day(points[0].iso) })}
            {noBase.length > 0 && ` ${t('history.change.noBase', { metrics: noBase.map((s) => s.label).join(', ') })}`}
          </Typography>
        )}
        {ready && data?.truncated && (
          <Typography variant="caption" color="text.secondary" component="p" sx={{ marginTop: 0.5 }}>
            {t('history.truncated')}
          </Typography>
        )}
      </Box>
    </ChartCard>
  );
  return gutterBottom ? <Box sx={{ marginBottom: 4 }}>{card}</Box> : card;
}
