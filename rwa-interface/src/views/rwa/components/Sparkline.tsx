import { ReactNode, useId, useMemo } from 'react';
import { useTheme } from '@mui/material/styles';
import Box from '@mui/material/Box';
import Grid from '@mui/material/Grid';
import Typography from '@mui/material/Typography';

import { useHistory, useHistoryMetrics } from 'api/history';
import useHydrated from 'hooks/useHydrated';
import { HistoryEntity } from 'types/api';
import VisuallyHidden from 'ui-component/VisuallyHidden';
import {
  chartPoints,
  describeSeries,
  formatMetric,
  HistoryRangeOption,
  metricLabel,
  metricTypeResolver,
  rangeFrom,
  seriesStats
} from 'utils/history';
import { DASH, useFormat, useT } from 'i18n';
import { useSeriesColors } from './Charts';

const POSITION_METRICS = ['liquidity_usd', 'apy_percent'];
const W = 120;
const H = 32;
const PAD = 3;

interface SparklineProps {
  values: (number | null)[];
  color: string;
  summary: string; // text alternative: the SVG itself is decorative
}

// A tiny trend line without axes. The drawing is hidden from assistive technology; the summary is real text next to it
// (and the SVG title, for the eye hovering it). One value is a dot, none draws nothing.
export function Sparkline({ values, color, summary }: SparklineProps) {
  const theme = useTheme();
  const present = values.filter((v): v is number => v !== null);
  const min = present.length ? Math.min(...present) : 0;
  const max = present.length ? Math.max(...present) : 0;
  const x = (i: number) => (values.length < 2 ? W / 2 : PAD + (i / (values.length - 1)) * (W - 2 * PAD));
  const y = (v: number) => (max === min ? H / 2 : H - PAD - ((v - min) / (max - min)) * (H - 2 * PAD));
  let d = '';
  let pen = false;
  values.forEach((v, i) => {
    if (v === null) {
      pen = false;
      return;
    }
    d += `${pen ? 'L' : 'M'}${x(i).toFixed(1)} ${y(v).toFixed(1)}`;
    pen = true;
  });
  const lastIndex = values.reduce<number>((acc, v, i) => (v === null ? acc : i), -1);

  return (
    <Box component="span" sx={{ display: 'block', height: H }}>
      <svg
        width={W}
        height={H}
        viewBox={`0 0 ${W} ${H}`}
        aria-hidden="true"
        focusable="false"
        style={{ display: 'block', overflow: 'visible' }}
      >
        <title>{summary}</title>
        {present.length > 1 && <path d={d} fill="none" stroke={color} strokeWidth={1.75} strokeLinejoin="round" strokeLinecap="round" />}
        {lastIndex >= 0 && (
          <circle
            cx={x(lastIndex)}
            cy={y(values[lastIndex] as number)}
            r={2.75}
            fill={color}
            stroke={theme.palette.background.default}
            strokeWidth={1.5}
          />
        )}
      </svg>
      <VisuallyHidden>{summary}</VisuallyHidden>
    </Box>
  );
}

// History of some metrics for small displays (cards, dialogs): one request, client only. Before hydration, and while it
// loads, `ready` is false: the server markup and the first client render both show the placeholder.
function useTrends(entity: HistoryEntity, id: string | number | null, metrics: string[], range: HistoryRangeOption = 90) {
  const hydrated = useHydrated();
  const from = useMemo(() => rangeFrom(range), [range]);
  const { data, error } = useHistory(entity, id, metrics, { from, step: 'day' });
  const { data: meta } = useHistoryMetrics();
  const typeOf = useMemo(() => metricTypeResolver(meta, entity), [meta, entity]);
  const points = useMemo(() => (data ? chartPoints(data.points, metrics) : []), [data, metrics]);
  // an error belongs to the current request: previous data is not shown under it
  return { ready: hydrated && data !== undefined && !error, failed: hydrated && !!error, points, typeOf };
}

export interface TrendItem {
  metric: string;
  label?: string; // default: the dictionary name of the metric
  current?: ReactNode; // the value now, when the page knows it (from the export: the server renders it); else the latest point
}

// Stat cards with a trend: the metric's latest value and a sparkline, in the look of StatGrid (a description list).
export function SparklineStats({
  entity,
  id,
  items,
  requestMetrics
}: {
  entity: HistoryEntity;
  id: string | number | null;
  items: TrendItem[];
  requestMetrics?: string[]; // the metrics to request: the same list as a chart on the page makes one shared request
}) {
  const t = useT();
  const f = useFormat();
  const theme = useTheme();
  const colors = useSeriesColors();
  const metrics = useMemo(() => requestMetrics ?? items.map((i) => i.metric), [requestMetrics, items]);
  const { ready, failed, points, typeOf } = useTrends(entity, id, metrics);

  return (
    <>
      <Grid container component="dl" spacing={{ xs: 1, sm: 1.5 }} sx={{ marginTop: 0, marginBottom: 0 }} aria-busy={!ready && !failed}>
        {items.map((item) => {
          const index = metrics.indexOf(item.metric);
          const label = item.label ?? metricLabel(t, item.metric);
          const type = typeOf(item.metric);
          const stats = ready ? seriesStats(points, index) : null;
          return (
            <Grid
              key={item.metric}
              size={{ xs: 6, sm: 4, md: 3 }}
              sx={{
                display: 'flex',
                flexDirection: 'column-reverse',
                justifyContent: 'flex-end',
                gap: { xs: 0.25, sm: 0.5 },
                padding: { xs: 1, sm: '10px 16px' }
              }}
            >
              <Typography variant="h6" component="dt" sx={{ fontWeight: 400, color: theme.palette.grey[500], textWrap: 'balance' }}>
                {label}
              </Typography>
              <Box component="dd" sx={{ margin: 0 }}>
                <Typography variant="h3" component="span" sx={{ display: 'block', ...{ fontVariantNumeric: 'tabular-nums' } }}>
                  {item.current ?? (stats?.last ? formatMetric(f, type, stats.last.value) : DASH)}
                </Typography>
                {ready ? (
                  <Sparkline
                    values={points.map((p) => p.values[index])}
                    color={colors[index % colors.length]}
                    summary={describeSeries(t, f, label, type, points, index) ?? t('history.empty')}
                  />
                ) : (
                  <Box component="span" sx={{ display: 'block', height: H }} />
                )}
              </Box>
            </Grid>
          );
        })}
      </Grid>
      {failed && (
        <Typography variant="caption" color="text.secondary" role="status" component="p" sx={{ marginTop: 0.5 }}>
          {t('history.error')}
        </Typography>
      )}
    </>
  );
}

// Liquidity and APY of one position over time, for the position dialog.
export function PositionTrends({ positionKey }: { positionKey: string }) {
  const t = useT();
  const f = useFormat();
  const colors = useSeriesColors();
  const headingId = useId();
  const { ready, failed, points, typeOf } = useTrends('opportunities', positionKey, POSITION_METRICS);
  if (failed) return null; // a position without history (or an API without it) shows nothing rather than an error

  return (
    <Box component="section" aria-labelledby={headingId} aria-busy={!ready} sx={{ marginTop: 1.5, minHeight: 2 * (H + 28) }}>
      <Typography variant="subtitle2" component="h3" id={headingId} gutterBottom>
        {t('history.dialog.title')}
      </Typography>
      {!ready ? (
        <Typography variant="body2" color="text.secondary" role="status">
          {t('history.loading')}
        </Typography>
      ) : points.length === 0 ? (
        <Typography variant="body2" color="text.secondary">
          {t('history.empty')}
        </Typography>
      ) : (
        POSITION_METRICS.map((metric, index) => {
          const label = metricLabel(t, metric);
          const type = typeOf(metric);
          const last = seriesStats(points, index).last;
          return (
            <Box key={metric} sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 2, paddingY: 0.5 }}>
              <Box>
                <Typography variant="body2" color="text.secondary">
                  {label}
                </Typography>
                <Typography variant="body1" sx={{ fontVariantNumeric: 'tabular-nums' }}>
                  {last ? formatMetric(f, type, last.value) : DASH}
                </Typography>
              </Box>
              <Sparkline
                values={points.map((p) => p.values[index])}
                color={colors[index % colors.length]}
                summary={describeSeries(t, f, label, type, points, index) ?? t('history.empty')}
              />
            </Box>
          );
        })
      )}
    </Box>
  );
}
