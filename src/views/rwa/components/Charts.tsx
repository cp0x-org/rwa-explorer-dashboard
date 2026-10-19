import { KeyboardEvent, ReactNode, useId, useState } from 'react';
import { alpha, useTheme } from '@mui/material/styles';
import Box from '@mui/material/Box';
import Stack from '@mui/material/Stack';
import Tooltip from '@mui/material/Tooltip';
import Typography from '@mui/material/Typography';

import VisuallyHidden from 'ui-component/VisuallyHidden';
import { LocaleLink, useFormat, useT } from 'i18n';

// Categorical slots, assigned in this fixed order and never cycled. Validated as a set
// (adjacent CVD ΔE ≥ 8.4, contrast ≥ 3:1) against the dark card surface #1e1e26.
const SERIES_DARK = ['#3987e5', '#d95926', '#199e70', '#c98500', '#d55181', '#008300', '#9085e9', '#e66767'];
const SERIES_LIGHT = ['#2a78d6', '#eb6834', '#1baf7a', '#eda100', '#e87ba4', '#008300', '#4a3aa7', '#e34948'];
export const OTHER_COLOR = '#898781';

export function useSeriesColors(): string[] {
  const theme = useTheme();
  return theme.palette.mode === 'dark' ? SERIES_DARK : SERIES_LIGHT;
}

const tabular = { fontVariantNumeric: 'tabular-nums' } as const;

const focusRing = (color: string) => ({ '&:focus-visible': { outline: `2px solid ${color}`, outlineOffset: 2 } });

// Card that holds one chart: a section with its own heading, optional subtitle, content.
export function ChartCard({
  title,
  subtitle,
  action,
  children
}: {
  title: string;
  subtitle?: ReactNode;
  action?: ReactNode;
  children: ReactNode;
}) {
  const id = useId();
  return (
    <Box
      component="section"
      aria-labelledby={id}
      sx={{ padding: '20px', backgroundColor: 'background.default', borderRadius: '12px', height: '100%' }}
    >
      <Box sx={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 2, marginBottom: 2, flexWrap: 'wrap' }}>
        <Box sx={{ minWidth: 0 }}>
          <Typography variant="h4" component="h3" id={id}>
            {title}
          </Typography>
          {subtitle && (
            <Typography variant="body2" color="text.secondary" component="p" sx={{ marginTop: 0.5 }}>
              {subtitle}
            </Typography>
          )}
        </Box>
        {action}
      </Box>
      {children}
    </Box>
  );
}

// Legend key: rect for bars/segments, dot for markers, line for reference lines. Text stays in text tokens.
export function LegendKey({ color, label, shape = 'rect' }: { color: string; label: ReactNode; shape?: 'rect' | 'dot' | 'line' }) {
  const theme = useTheme();
  const key =
    shape === 'line' ? (
      <Box aria-hidden="true" sx={{ width: '1px', height: 14, backgroundColor: color }} />
    ) : (
      <Box
        aria-hidden="true"
        sx={{
          width: shape === 'dot' ? 8 : 12,
          height: shape === 'dot' ? 8 : 12,
          borderRadius: shape === 'dot' ? '50%' : '3px',
          backgroundColor: color,
          boxShadow: shape === 'dot' ? `0 0 0 2px ${theme.palette.background.default}` : 'none'
        }}
      />
    );
  return (
    <Box sx={{ display: 'inline-flex', alignItems: 'center', gap: 0.75 }}>
      {key}
      <Typography variant="body2" color="text.secondary" component="span">
        {label}
      </Typography>
    </Box>
  );
}

// ==============================|| DONUT ||============================== //

export interface Segment {
  id: string;
  label: string;
  value: number;
  color: string;
  detail?: ReactNode; // extra line in the tooltip and the legend
  to?: string; // page of the segment: its legend row is a link
}

interface DonutProps {
  segments: Segment[];
  centerCaption: string;
  formatValue?: (value: number) => string;
  size?: number;
  thickness?: number;
}

// Part-to-whole ring with a legend that lists every value, so the tooltip never gates a number. The SVG is decorative
// for assistive technology: the legend (links, keyboard-reachable) and a hidden summary carry the same information.
export function DonutChart({ segments, centerCaption, formatValue, size = 190, thickness = 24 }: DonutProps) {
  const theme = useTheme();
  const t = useT();
  const f = useFormat();
  const fmt = formatValue ?? ((v: number) => f.num(v));
  const pctOf = (value: number, total: number) => f.pct(total > 0 ? (value / total) * 100 : 0, 1);
  const [active, setActive] = useState<string | null>(null);
  const visible = segments.filter((s) => s.value > 0);
  const total = visible.reduce((sum, s) => sum + s.value, 0);
  const radius = (size - thickness - 8) / 2;
  const circumference = 2 * Math.PI * radius;
  const gap = visible.length > 1 ? 2 : 0; // surface gap between segments
  const activeSegment = visible.find((s) => s.id === active);
  const center = size / 2;

  let offset = 0;
  const arcs = visible.map((s) => {
    const length = (s.value / total) * circumference;
    const dash = Math.max(length - gap, 1);
    const arc = { segment: s, dash, offset };
    offset += length;
    return arc;
  });

  const hoverProps = (id: string) => ({
    onMouseEnter: () => setActive(id),
    onMouseLeave: () => setActive(null),
    onFocus: () => setActive(id),
    onBlur: () => setActive(null)
  });

  const summary = t('chart.donut.summary', {
    caption: centerCaption,
    total: fmt(total),
    items: visible.map((s) => t('chart.item', { label: s.label, value: fmt(s.value), share: pctOf(s.value, total) })).join(', ')
  });

  return (
    <Box sx={{ display: 'flex', alignItems: 'center', gap: 3, flexWrap: 'wrap' }}>
      <VisuallyHidden component="p">{summary}</VisuallyHidden>
      <Box aria-hidden="true" sx={{ position: 'relative', width: size, height: size, flexShrink: 0, marginX: { xs: 'auto', sm: 0 } }}>
        <svg width={size} height={size} viewBox={`0 0 ${size} ${size}`} focusable="false">
          <g transform={`rotate(-90 ${center} ${center})`}>
            {total === 0 ? (
              <circle cx={center} cy={center} r={radius} fill="none" stroke={theme.palette.divider} strokeWidth={thickness} />
            ) : (
              arcs.map(({ segment: s, dash, offset: o }) => (
                <Tooltip
                  key={s.id}
                  followCursor
                  describeChild
                  title={
                    <Box>
                      <Typography variant="subtitle2" sx={{ color: 'inherit', fontWeight: 600 }}>
                        {fmt(s.value)} · {pctOf(s.value, total)}
                      </Typography>
                      <Typography variant="caption" sx={{ color: 'inherit', opacity: 0.8, display: 'block' }}>
                        {s.label}
                      </Typography>
                      {s.detail && (
                        <Typography variant="caption" sx={{ color: 'inherit', opacity: 0.8, display: 'block' }}>
                          {s.detail}
                        </Typography>
                      )}
                    </Box>
                  }
                >
                  <circle
                    cx={center}
                    cy={center}
                    r={radius}
                    fill="none"
                    stroke={s.color}
                    strokeWidth={active === s.id ? thickness + 6 : thickness}
                    strokeDasharray={`${dash} ${circumference - dash}`}
                    strokeDashoffset={-o}
                    opacity={active && active !== s.id ? 0.55 : 1}
                    style={{ transition: 'stroke-width 120ms, opacity 120ms' }}
                    onMouseEnter={() => setActive(s.id)}
                    onMouseLeave={() => setActive(null)}
                  />
                </Tooltip>
              ))
            )}
          </g>
        </svg>
        <Box
          sx={{
            position: 'absolute',
            inset: 0,
            display: 'flex',
            flexDirection: 'column',
            alignItems: 'center',
            justifyContent: 'center',
            pointerEvents: 'none',
            textAlign: 'center',
            paddingX: `${thickness + 12}px`
          }}
        >
          <Typography variant="h3" component="div">
            {activeSegment ? pctOf(activeSegment.value, total) : fmt(total)}
          </Typography>
          <Typography variant="caption" color="text.secondary" sx={{ lineHeight: 1.3 }}>
            {activeSegment ? activeSegment.label : centerCaption}
          </Typography>
        </Box>
      </Box>

      <Stack component="ul" spacing={0.5} sx={{ flex: 1, minWidth: 200, listStyle: 'none', margin: 0, padding: 0 }}>
        {visible.map((s) => (
          <Box key={s.id} component="li" sx={{ display: 'block' }}>
            <Box
              component={s.to ? LocaleLink : 'div'}
              {...(s.to ? { to: s.to } : {})}
              {...hoverProps(s.id)}
              sx={{
                display: 'grid',
                gridTemplateColumns: '12px minmax(0, 1fr) auto auto',
                alignItems: 'center',
                columnGap: 1.25,
                paddingX: 1,
                paddingY: 0.5,
                borderRadius: '6px',
                color: 'inherit',
                textDecoration: 'none',
                cursor: s.to ? 'pointer' : 'default',
                backgroundColor: active === s.id ? alpha(theme.palette.text.primary, 0.06) : 'transparent',
                ...focusRing(theme.palette.secondary.main)
              }}
            >
              <Box aria-hidden="true" sx={{ width: 12, height: 12, borderRadius: '3px', backgroundColor: s.color }} />
              <Box sx={{ minWidth: 0 }}>
                <Typography variant="body2" noWrap sx={{ fontWeight: 500 }}>
                  {s.label}
                </Typography>
                {s.detail && (
                  <Typography variant="caption" color="text.secondary" noWrap component="div">
                    {s.detail}
                  </Typography>
                )}
              </Box>
              <Typography variant="body2" sx={{ ...tabular, fontWeight: 500, textAlign: 'right' }}>
                {pctOf(s.value, total)}
              </Typography>
              <Typography variant="body2" color="text.secondary" sx={{ ...tabular, textAlign: 'right', minWidth: 36 }}>
                {fmt(s.value)}
              </Typography>
            </Box>
          </Box>
        ))}
      </Stack>
    </Box>
  );
}

// ==============================|| HORIZONTAL BARS ||============================== //

const BAR = 12; // bar thickness, px

// Data-end rounded, square at the baseline.
const barSx = (color: string) => ({ height: BAR, backgroundColor: color, borderRadius: '0 4px 4px 0' });

interface RowFrameProps {
  label: ReactNode;
  value: ReactNode;
  tooltip?: ReactNode;
  onClick?: () => void;
  to?: string; // a page to open: the whole row is a link (works without JS, in a new tab, followed by crawlers)
  selected?: boolean;
  labelWidth: number;
  description?: string; // what the bar shows, for assistive technology (the bar itself is decorative)
  children: ReactNode;
}

function RowFrame({ label, value, tooltip, onClick, to, selected, labelWidth, description, children }: RowFrameProps) {
  const theme = useTheme();
  const interactive = to
    ? { component: LocaleLink, to }
    : onClick
      ? {
          role: 'button',
          tabIndex: 0,
          onClick,
          'aria-pressed': selected,
          onKeyDown: (e: KeyboardEvent) => {
            if (e.key === 'Enter' || e.key === ' ') {
              e.preventDefault();
              onClick();
            }
          }
        }
      : {};
  const inner = (
    <Box
      {...interactive}
      sx={{
        display: 'grid',
        // phone: label and value on one line, the bar full width below; wider: label | bar | value
        gridTemplateColumns: { xs: 'minmax(0, 1fr) auto', sm: `${labelWidth}px minmax(0, 1fr) auto` },
        gridTemplateAreas: { xs: '"label value" "bar bar"', sm: '"label bar value"' },
        alignItems: 'center',
        columnGap: 1.5,
        paddingX: 1,
        paddingY: 0.75,
        borderRadius: '6px',
        cursor: onClick || to ? 'pointer' : 'default',
        color: 'inherit',
        textDecoration: 'none',
        backgroundColor: selected ? alpha(theme.palette.secondary.main, 0.1) : 'transparent',
        '&:hover, &:focus-visible': {
          backgroundColor: selected ? alpha(theme.palette.secondary.main, 0.14) : alpha(theme.palette.text.primary, 0.05)
        },
        ...focusRing(theme.palette.secondary.main)
      }}
    >
      <Box sx={{ gridArea: 'label', minWidth: 0 }}>{label}</Box>
      <Box aria-hidden="true" sx={{ gridArea: 'bar', position: 'relative', height: BAR + 8, display: 'flex', alignItems: 'center' }}>
        {children}
      </Box>
      <Box sx={{ gridArea: 'value', ...tabular, textAlign: 'right', whiteSpace: 'nowrap' }}>{value}</Box>
      {description && <VisuallyHidden>. {description}</VisuallyHidden>}
    </Box>
  );
  return (
    <Box component="li" sx={{ display: 'block', listStyle: 'none' }}>
      {tooltip ? (
        <Tooltip title={tooltip} placement="top" followCursor describeChild>
          {inner}
        </Tooltip>
      ) : (
        inner
      )}
    </Box>
  );
}

export interface BarItem {
  id: string;
  label: ReactNode;
  value: number;
  valueLabel: ReactNode;
  tooltip?: ReactNode;
  onClick?: () => void;
  to?: string;
}

// One series: one color for every bar, value at the tip.
export function BarList({ items, max, color, labelWidth = 170 }: { items: BarItem[]; max?: number; color: string; labelWidth?: number }) {
  const scale = max ?? Math.max(...items.map((i) => i.value), 0);
  return (
    <Stack component="ul" spacing={0.25} sx={{ margin: 0, padding: 0 }}>
      {items.map((item) => (
        <RowFrame
          key={item.id}
          label={item.label}
          value={item.valueLabel}
          tooltip={item.tooltip}
          onClick={item.onClick}
          to={item.to}
          labelWidth={labelWidth}
        >
          {item.value > 0 && <Box sx={{ ...barSx(color), width: `${Math.max((item.value / (scale || 1)) * 100, 0.6)}%` }} />}
        </RowFrame>
      ))}
    </Stack>
  );
}

export interface RangeItem extends Omit<BarItem, 'value'> {
  average: number;
  max: number;
}

// Bar to the average, whisker with an end-dot to the maximum; both on one axis (same unit).
export function RangeBarList({
  items,
  color,
  reference,
  labelWidth = 170
}: {
  items: RangeItem[];
  color: string;
  reference?: number; // vertical hairline, e.g. the basket average
  labelWidth?: number;
}) {
  const theme = useTheme();
  const surface = theme.palette.background.default;
  const scale = Math.max(...items.map((i) => i.max), reference ?? 0) || 1;
  const at = (v: number) => `${(v / scale) * 100}%`;
  return (
    <Stack component="ul" spacing={0.25} sx={{ margin: 0, padding: 0 }}>
      {items.map((item) => (
        <RowFrame
          key={item.id}
          label={item.label}
          value={item.valueLabel}
          tooltip={item.tooltip}
          onClick={item.onClick}
          to={item.to}
          labelWidth={labelWidth}
        >
          {reference !== undefined && (
            <Box
              sx={{
                position: 'absolute',
                left: at(reference),
                top: -6,
                bottom: -6,
                width: '1px',
                backgroundColor: theme.palette.text.secondary,
                opacity: 0.6
              }}
            />
          )}
          {item.max > item.average && (
            <Box
              sx={{
                position: 'absolute',
                left: at(item.average),
                width: `calc(${at(item.max - item.average)})`,
                height: 2,
                backgroundColor: alpha(color, 0.55)
              }}
            />
          )}
          <Box sx={{ ...barSx(color), position: 'relative', width: at(item.average), minWidth: 2 }} />
          <Box
            sx={{
              position: 'absolute',
              left: `calc(${at(item.max)} - 5px)`,
              width: 10,
              height: 10,
              borderRadius: '50%',
              backgroundColor: color,
              boxShadow: `0 0 0 2px ${surface}`
            }}
          />
        </RowFrame>
      ))}
    </Stack>
  );
}

export interface StackedRow {
  id: string;
  label: ReactNode;
  valueLabel: ReactNode;
  segments: Segment[];
  onClick?: () => void;
  selected?: boolean;
}

// Part-to-whole per row, segments separated by a 2px surface gap; each segment has its own tooltip, and the row
// carries the segment values as hidden text.
export function StackedBarList({ rows, labelWidth = 170 }: { rows: StackedRow[]; labelWidth?: number }) {
  const t = useT();
  const f = useFormat();
  const totals = rows.map((r) => r.segments.reduce((sum, s) => sum + s.value, 0));
  const scale = Math.max(...totals, 0) || 1;
  const pctOf = (value: number, total: number) => f.pct(total > 0 ? (value / total) * 100 : 0, 1);
  return (
    <Stack component="ul" spacing={0.25} sx={{ margin: 0, padding: 0 }}>
      {rows.map((row, n) => {
        const visible = row.segments.filter((s) => s.value > 0);
        return (
          <RowFrame
            key={row.id}
            label={row.label}
            value={row.valueLabel}
            onClick={row.onClick}
            selected={row.selected}
            labelWidth={labelWidth}
            description={t('chart.stacked.summary', {
              total: f.num(totals[n]),
              items: visible
                .map((s) => t('chart.item', { label: s.label, value: f.num(s.value), share: pctOf(s.value, totals[n]) }))
                .join(', ')
            })}
          >
            <Box sx={{ display: 'flex', gap: '2px', width: `${(totals[n] / scale) * 100}%`, minWidth: visible.length ? 2 : 0 }}>
              {visible.map((s, i) => (
                <Tooltip
                  key={s.id}
                  followCursor
                  describeChild
                  title={
                    <Box>
                      <Typography variant="subtitle2" sx={{ color: 'inherit', fontWeight: 600 }}>
                        {f.num(s.value)} · {pctOf(s.value, totals[n])}
                      </Typography>
                      <Typography variant="caption" sx={{ color: 'inherit', opacity: 0.8 }}>
                        {s.label}
                      </Typography>
                    </Box>
                  }
                >
                  <Box
                    sx={{
                      flex: `${s.value} 1 0`,
                      minWidth: 2,
                      height: BAR,
                      backgroundColor: s.color,
                      borderRadius: i === visible.length - 1 ? '0 4px 4px 0' : 0,
                      '&:hover': { filter: 'brightness(1.2)' }
                    }}
                  />
                </Tooltip>
              ))}
            </Box>
          </RowFrame>
        );
      })}
    </Stack>
  );
}
