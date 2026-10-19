import { useId, useMemo, useRef, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import { useTheme } from '@mui/material/styles';
import Autocomplete from '@mui/material/Autocomplete';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import FormControl from '@mui/material/FormControl';
import Grid from '@mui/material/Grid';
import InputLabel from '@mui/material/InputLabel';
import Link from '@mui/material/Link';
import MenuItem from '@mui/material/MenuItem';
import Select from '@mui/material/Select';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableHead from '@mui/material/TableHead';
import TableRow from '@mui/material/TableRow';
import TextField from '@mui/material/TextField';
import Typography from '@mui/material/Typography';

import { isReportedYield, logoUrl, relationRows, RwaIndex } from 'api/rwa';
import { TokenIcon } from 'components/TokenIcon';
import { Issuer, Token, Underlying } from 'types/rwa';
import { DASH } from 'utils/rwa';
import { Format, LocaleLink, T, useFormat, useLocaleNavigate, useT } from 'i18n';
import {
  BarList,
  ChartCard,
  DonutChart,
  LegendKey,
  OTHER_COLOR,
  RangeBarList,
  Segment,
  StackedBarList,
  useSeriesColors
} from './components/Charts';
import DataTableContainer from './components/DataTableContainer';
import DataGate from './components/DataGate';
import Legend from './components/Legend';
import MethodNote from './components/MethodNote';
import { StatGrid } from './components/StatGrid';
import TableCaption from './components/TableCaption';
import { stickyColumn } from './components/tableSx';
import RowLink from './components/RowLink';

// ==============================|| AGGREGATES ||============================== //

interface YieldStats {
  count: number;
  average: number;
  max: number;
}

interface TokenStat {
  token: Token;
  eligible: Set<string>; // eligible position keys
  yieldKeys: Set<string>; // eligible positions with APY > 0 (anomalies excluded)
  yield: YieldStats | null;
}

interface IssuerStat {
  issuer: Issuer;
  color: string;
  versions: number;
  inDefi: number; // tokenized assets with at least one eligible position
  platforms: Map<string, string>; // distinct DeFi protocols (slug → name) across eligible positions of all its tokens
  yield: YieldStats | null;
}

const OTHER_ID = '__other__';

// Average and maximum over positions with a positive reported APY; zero and missing APY never enter the average.
function yieldStats(index: RwaIndex, keys: Iterable<string>): YieldStats | null {
  let count = 0;
  let sum = 0;
  let max = 0;
  for (const key of keys) {
    const apy = Number(index.opportunityByKey.get(key)?.apy_percent ?? 0);
    if (!(apy > 0)) continue;
    count += 1;
    sum += apy;
    max = Math.max(max, apy);
  }
  return count ? { count, average: sum / count, max } : null;
}

const union = (sets: Set<string>[]) => new Set(sets.flatMap((s) => Array.from(s)));

// Distinct protocols behind a set of positions: 200 Uniswap V3 pools count once, Uniswap V2 and V3 count separately.
function distinctPlatforms(index: RwaIndex, keys: Iterable<string>): Map<string, string> {
  const platforms = new Map<string, string>();
  for (const key of keys) {
    const o = index.opportunityByKey.get(key);
    if (o) platforms.set(o.platform, o.platform_name);
  }
  return platforms;
}

function buildStats(index: RwaIndex, series: string[]) {
  const basket = index.data.tokens.filter((t) => t.excluded_reason === null);
  const eligible = new Map<number, Set<string>>();
  const yieldKeys = new Map<number, Set<string>>();
  basket.forEach((t) => {
    eligible.set(t.id, new Set());
    yieldKeys.set(t.id, new Set());
  });
  relationRows(index, basket).forEach((row) => {
    if (row.opportunity.eligibility === 'eligible') eligible.get(row.token.id)!.add(row.opportunity.key);
    if (isReportedYield(row)) yieldKeys.get(row.token.id)!.add(row.opportunity.key);
  });
  const tokens: TokenStat[] = basket.map((t) => ({
    token: t,
    eligible: eligible.get(t.id)!,
    yieldKeys: yieldKeys.get(t.id)!,
    yield: yieldStats(index, yieldKeys.get(t.id)!)
  }));
  const tokenStatById = new Map(tokens.map((s) => [s.token.id, s]));

  // Issuer color follows the issuer (fixed by overall rank), never its rank inside a filtered view.
  const ranked = [...index.data.issuers].sort(
    (a, b) => b.issuer_usefulness_score - a.issuer_usefulness_score || (a.name ?? '').localeCompare(b.name ?? '')
  );
  // Issuers without eligible positions never appear in a chart and stay neutral.
  const withPositions = ranked.filter((i) => i.issuer_usefulness_score > 0);
  const slots = withPositions.length > series.length ? series.length - 1 : series.length; // keep a slot for "Other" when folding
  const issuerColor = new Map(ranked.map((i) => [i.id, OTHER_COLOR]));
  withPositions.slice(0, slots).forEach((i, n) => issuerColor.set(i.id, series[n]));

  const issuers: IssuerStat[] = ranked.map((issuer) => {
    const own = tokens.filter((s) => s.token.issuer_id === issuer.id);
    return {
      issuer,
      color: issuerColor.get(issuer.id)!,
      versions: own.length,
      inDefi: own.filter((s) => s.eligible.size > 0).length,
      platforms: distinctPlatforms(index, union(own.map((s) => s.eligible))),
      yield: yieldStats(index, union(own.map((s) => s.yieldKeys)))
    };
  });

  const overallYield = yieldStats(index, union(tokens.map((s) => s.yieldKeys)));
  return { tokens, tokenStatById, issuers, issuerColor, overallYield };
}

type Stats = ReturnType<typeof buildStats>;

// Issuers ranked by how many distinct protocols host their tokenized assets; ties keep the overall rank (more positions first).
function byProtocolDiversity(issuers: IssuerStat[]): IssuerStat[] {
  return issuers.filter((i) => i.versions > 0).sort((a, b) => b.platforms.size - a.platforms.size);
}

// Issuers ranked by the share of their tokenized assets with at least one eligible position; ties go to the wider line-up.
function byDefiPresence(issuers: IssuerStat[]): IssuerStat[] {
  return [...issuers]
    .filter((i) => i.versions > 0)
    .sort((a, b) => b.inDefi / b.versions - a.inDefi / a.versions || b.versions - a.versions);
}

// Per-issuer donut segments; issuers past the categorical slots fold into one "Other" segment.
function issuerSegments(t: T, items: { id: string; name: string; value: number; color: string; detail?: string }[]): Segment[] {
  const segments: Segment[] = [];
  let other = 0;
  items.forEach((i) => {
    if (i.value <= 0) return;
    if (i.color === OTHER_COLOR) other += i.value;
    else segments.push({ id: i.id, label: i.name, value: i.value, color: i.color, detail: i.detail, to: `/issuers/${i.id}` });
  });
  if (other > 0) segments.push({ id: OTHER_ID, label: t('common.otherIssuers'), value: other, color: OTHER_COLOR });
  return segments.sort((a, b) => (a.id === OTHER_ID ? 1 : b.id === OTHER_ID ? -1 : b.value - a.value));
}

// Eligible positions of each issuer's tokenized assets of an underlying (a pool shared by two of one issuer's tokens counts once).
function issuerParts(t: T, stats: Stats, u: Underlying) {
  const keys = new Map<string, Set<string>[]>();
  u.token_ids.forEach((id) => {
    const s = stats.tokenStatById.get(id);
    if (!s?.token.issuer_id) return;
    keys.set(s.token.issuer_id, [...(keys.get(s.token.issuer_id) ?? []), s.eligible]);
  });
  return stats.issuers
    .filter((i) => keys.has(i.issuer.id))
    .map((i) => ({
      id: i.issuer.id,
      name: i.issuer.name ?? t('common.unknownIssuer'),
      value: union(keys.get(i.issuer.id)!).size,
      color: i.color
    }));
}

const pctOf = (f: Format, value: number | null | undefined) => (value === null || value === undefined ? DASH : f.pct(value));
const shareOf = (f: Format, part: number, whole: number) => (whole > 0 ? f.pct((part / whole) * 100, 1) : DASH);

function TokenLabel({ token, caption }: { token: Token; caption: string }) {
  return (
    <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, minWidth: 0 }}>
      <TokenIcon symbol={token.symbol ?? '?'} src={logoUrl(token.logo)} avatarProps={{ sx: { width: 24, height: 24, fontSize: 10 } }} />
      <Box sx={{ minWidth: 0 }}>
        <Typography variant="body2" noWrap sx={{ fontWeight: 500 }}>
          {token.symbol ?? DASH}
        </Typography>
        <Typography variant="caption" color="text.secondary" noWrap component="div">
          {caption}
        </Typography>
      </Box>
    </Box>
  );
}

function ApyValue({ stats }: { stats: YieldStats }) {
  const f = useFormat();
  return (
    <Typography variant="body2" component="span">
      <Box component="span" sx={{ fontWeight: 600 }}>
        {pctOf(f, stats.average)}
      </Box>
      <Box component="span" sx={{ color: 'text.secondary' }}>
        {' / '}
        {pctOf(f, stats.max)}
      </Box>
    </Typography>
  );
}

function ApyTooltip({ title, stats, extra }: { title: string; stats: YieldStats; extra?: string }) {
  const t = useT();
  const f = useFormat();
  return (
    <Box>
      <Typography variant="subtitle2" sx={{ color: 'inherit', fontWeight: 600 }}>
        {t('summary.yield.tooltipStats', { average: pctOf(f, stats.average), max: pctOf(f, stats.max) })}
      </Typography>
      <Typography variant="caption" sx={{ color: 'inherit', opacity: 0.8, display: 'block' }}>
        {title}
      </Typography>
      <Typography variant="caption" sx={{ color: 'inherit', opacity: 0.8, display: 'block' }}>
        {t('summary.yield.tooltipCount', { n: stats.count })}
        {extra ? ` · ${extra}` : ''}
      </Typography>
    </Box>
  );
}

function YieldKeys({ accent, reference }: { accent: string; reference?: number }) {
  const theme = useTheme();
  const t = useT();
  const f = useFormat();
  return (
    <Box sx={{ display: 'flex', gap: 2, flexWrap: 'wrap', marginBottom: 1.5 }}>
      <LegendKey color={accent} label={t('summary.yield.keyAverage')} />
      <LegendKey color={accent} label={t('summary.yield.keyMax')} shape="dot" />
      {reference !== undefined && (
        <LegendKey color={theme.palette.text.secondary} label={t('summary.yield.keyBasket', { value: pctOf(f, reference) })} shape="line" />
      )}
    </Box>
  );
}

// ==============================|| SECTIONS ||============================== //

function IssuersSection({ stats }: { stats: Stats }) {
  const theme = useTheme();
  const t = useT();
  const f = useFormat();
  const navigate = useLocaleNavigate();
  const accent = theme.palette.secondary.main;
  const total = stats.issuers.reduce((sum, i) => sum + i.issuer.issuer_usefulness_score, 0);
  const segments = issuerSegments(
    t,
    stats.issuers.map((i) => ({
      id: i.issuer.id,
      name: i.issuer.name ?? t('common.unknownIssuer'),
      value: i.issuer.issuer_usefulness_score,
      color: i.color,
      detail: t('summary.issuers.inDefi.detail', { n: i.inDefi, total: i.versions })
    }))
  );
  const presence = byDefiPresence(stats.issuers);
  const diversity = byProtocolDiversity(stats.issuers);
  const maxPlatforms = Math.max(...diversity.map((i) => i.platforms.size), 0);

  return (
    <Box component="section" sx={{ marginBottom: 5 }}>
      <Typography variant="h3" component="h2" gutterBottom>
        {t('summary.issuers.title')}
      </Typography>
      <Grid container spacing={2} sx={{ marginBottom: 2 }}>
        <Grid size={{ xs: 12, md: 6, lg: 4 }}>
          <ChartCard title={t('summary.issuers.share')} subtitle={t('summary.issuers.share.subtitle')}>
            <DonutChart segments={segments} centerCaption={t('summary.eligiblePositions')} />
          </ChartCard>
        </Grid>
        <Grid size={{ xs: 12, md: 6, lg: 4 }}>
          <ChartCard title={t('summary.issuers.inDefi')} subtitle={t('summary.issuers.inDefi.subtitle')}>
            <BarList
              color={accent}
              max={100}
              labelWidth={150}
              items={presence.map((i) => ({
                id: i.issuer.id,
                // The count sits under the name so the value column stays one width and every bar shares the same track.
                label: (
                  <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, minWidth: 0 }}>
                    <Box aria-hidden="true" sx={{ width: 8, height: 8, borderRadius: '50%', backgroundColor: i.color, flexShrink: 0 }} />
                    <Box sx={{ minWidth: 0 }}>
                      <Typography variant="body2" noWrap>
                        {i.issuer.name ?? t('common.unknownIssuer')}
                      </Typography>
                      <Typography variant="caption" noWrap sx={{ display: 'block', color: 'text.secondary', lineHeight: 1.2 }}>
                        {t('summary.issuers.inDefi.caption', { n: i.inDefi, total: i.versions })}
                      </Typography>
                    </Box>
                  </Box>
                ),
                value: (i.inDefi / i.versions) * 100,
                valueLabel: (
                  <Typography variant="body2" component="span" sx={{ fontWeight: 600 }}>
                    {shareOf(f, i.inDefi, i.versions)}
                  </Typography>
                ),
                tooltip: t('summary.issuers.inDefi.tooltip', { n: i.inDefi, total: i.versions }),
                to: `/issuers/${i.issuer.id}`
              }))}
            />
          </ChartCard>
        </Grid>
        <Grid size={{ xs: 12, md: 6, lg: 4 }}>
          <ChartCard title={t('summary.issuers.diversity')} subtitle={t('summary.issuers.diversity.subtitle')}>
            <BarList
              color={accent}
              max={maxPlatforms || 1}
              labelWidth={150}
              items={diversity.map((i) => {
                const names = Array.from(i.platforms.values()).sort((a, b) => a.localeCompare(b));
                return {
                  id: i.issuer.id,
                  label: (
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, minWidth: 0 }}>
                      <Box aria-hidden="true" sx={{ width: 8, height: 8, borderRadius: '50%', backgroundColor: i.color, flexShrink: 0 }} />
                      <Typography variant="body2" noWrap>
                        {i.issuer.name ?? t('common.unknownIssuer')}
                      </Typography>
                    </Box>
                  ),
                  value: i.platforms.size,
                  valueLabel: (
                    <Typography variant="body2" component="span">
                      <Box component="span" sx={{ fontWeight: 600 }}>
                        {i.platforms.size}
                      </Box>
                      <Box component="span" sx={{ color: 'text.secondary' }}>
                        {' '}
                        {t('summary.issuers.protocols', { n: i.platforms.size })}
                      </Box>
                    </Typography>
                  ),
                  tooltip: names.length ? names.join(', ') : t('summary.issuers.noEligible'),
                  to: `/issuers/${i.issuer.id}`
                };
              })}
            />
          </ChartCard>
        </Grid>
      </Grid>

      <DataTableContainer sx={stickyColumn(1)}>
        <Table size="small">
          <TableCaption>{t('table.issuerSummary')}</TableCaption>
          <TableHead>
            <TableRow>
              <TableCell>{t('col.issuer')}</TableCell>
              <TableCell align="right">{t('col.share')}</TableCell>
              <TableCell align="right">{t('col.eligiblePositions')}</TableCell>
              <TableCell align="right">{t('col.tokensInDefi')}</TableCell>
              <TableCell align="right">{t('col.protocols')}</TableCell>
              <TableCell align="right">{t('col.positionsWithApy')}</TableCell>
              <TableCell align="right">{t('col.avgApy')}</TableCell>
              <TableCell align="right">{t('col.maxApy')}</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {stats.issuers.map((i) => (
              <TableRow key={i.issuer.id} hover onClick={() => navigate(`/issuers/${i.issuer.id}`)} sx={{ cursor: 'pointer' }}>
                <TableCell component="th" scope="row" sx={{ fontWeight: 'inherit' }}>
                  <RowLink to={`/issuers/${i.issuer.id}`}>
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                      <Box
                        aria-hidden="true"
                        sx={{ width: 10, height: 10, borderRadius: '3px', backgroundColor: i.color, flexShrink: 0 }}
                      />
                      <TokenIcon
                        symbol={i.issuer.name ?? '?'}
                        src={logoUrl(i.issuer.logo)}
                        avatarProps={{ sx: { width: 24, height: 24, fontSize: 10 } }}
                      />
                      <Typography variant="body2" sx={{ fontWeight: 500 }}>
                        {i.issuer.name ?? t('common.unknownIssuer')}
                      </Typography>
                    </Box>
                  </RowLink>
                </TableCell>
                <TableCell align="right">{shareOf(f, i.issuer.issuer_usefulness_score, total)}</TableCell>
                <TableCell align="right">{f.num(i.issuer.issuer_usefulness_score)}</TableCell>
                <TableCell align="right">
                  {i.inDefi} / {i.versions}{' '}
                  <Typography component="span" variant="body2" color="text.secondary">
                    ({shareOf(f, i.inDefi, i.versions)})
                  </Typography>
                </TableCell>
                <TableCell align="right">{i.platforms.size}</TableCell>
                <TableCell align="right">{i.yield?.count ?? 0}</TableCell>
                <TableCell align="right">{pctOf(f, i.yield?.average)}</TableCell>
                <TableCell align="right">{pctOf(f, i.yield?.max)}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </DataTableContainer>
    </Box>
  );
}

type YieldSort = 'average' | 'max';

function YieldSection({ index, stats }: { index: RwaIndex; stats: Stats }) {
  const theme = useTheme();
  const t = useT();
  const f = useFormat();
  const sortId = useId();
  const minId = useId();
  const showId = useId();
  const accent = theme.palette.secondary.main;
  const [minPositions, setMinPositions] = useState(1);
  const [sortBy, setSortBy] = useState<YieldSort>('average');
  const [limit, setLimit] = useState(15);

  const withYield = stats.tokens.filter((s): s is TokenStat & { yield: YieldStats } => !!s.yield);
  const candidates = withYield
    .filter((s) => s.yield.count >= minPositions)
    .sort((a, b) => b.yield[sortBy] - a.yield[sortBy] || b.yield.count - a.yield.count);
  const rows = candidates.slice(0, limit);
  const overall = stats.overallYield;

  return (
    <Box component="section" sx={{ marginBottom: 5 }}>
      <Typography variant="h3" component="h2" gutterBottom>
        {t('summary.yield.title')}
      </Typography>
      <Typography variant="body2" color="text.secondary" sx={{ marginBottom: 2 }}>
        {t('summary.yield.text', {
          threshold: f.pct(index.data.settings.yield_apy_review_threshold_percent, 0),
          n: withYield.length,
          total: stats.tokens.length
        })}
      </Typography>

      <Grid container spacing={2} sx={{ marginBottom: 2 }}>
        <Grid size={{ xs: 12, sm: 4, md: 3 }}>
          <FormControl size="small" fullWidth>
            <InputLabel id={sortId}>{t('summary.yield.sortBy')}</InputLabel>
            <Select
              labelId={sortId}
              value={sortBy}
              label={t('summary.yield.sortBy')}
              onChange={(e) => setSortBy(e.target.value as YieldSort)}
            >
              <MenuItem value="average">{t('summary.yield.sortAverage')}</MenuItem>
              <MenuItem value="max">{t('summary.yield.sortMax')}</MenuItem>
            </Select>
          </FormControl>
        </Grid>
        <Grid size={{ xs: 6, sm: 4, md: 3 }}>
          <FormControl size="small" fullWidth>
            <InputLabel id={minId}>{t('summary.yield.minPositions')}</InputLabel>
            <Select
              labelId={minId}
              value={minPositions}
              label={t('summary.yield.minPositions')}
              onChange={(e) => setMinPositions(Number(e.target.value))}
            >
              {[1, 2, 3, 5, 10].map((n) => (
                <MenuItem key={n} value={n}>
                  {n === 1 ? t('common.any') : t('common.atLeast', { n })}
                </MenuItem>
              ))}
            </Select>
          </FormControl>
        </Grid>
        <Grid size={{ xs: 6, sm: 4, md: 3 }}>
          <FormControl size="small" fullWidth>
            <InputLabel id={showId}>{t('summary.yield.show')}</InputLabel>
            <Select labelId={showId} value={limit} label={t('summary.yield.show')} onChange={(e) => setLimit(Number(e.target.value))}>
              <MenuItem value={15}>{t('common.top', { n: 15 })}</MenuItem>
              <MenuItem value={30}>{t('common.top', { n: 30 })}</MenuItem>
              <MenuItem value={10000}>{t('common.all')}</MenuItem>
            </Select>
          </FormControl>
        </Grid>
      </Grid>

      <ChartCard
        title={sortBy === 'average' ? t('summary.yield.chartAverage') : t('summary.yield.chartMax')}
        subtitle={t('summary.yield.chartSubtitle', { n: rows.length, total: candidates.length })}
      >
        <YieldKeys accent={accent} reference={overall?.average} />
        {rows.length ? (
          <RangeBarList
            color={accent}
            reference={overall?.average}
            labelWidth={190}
            items={rows.map((s) => ({
              id: String(s.token.id),
              label: (
                <TokenLabel
                  token={s.token}
                  caption={`${s.token.issuer_name ?? t('common.unknownIssuer')} · ${t('summary.yield.positions', { n: s.yield.count })}`}
                />
              ),
              average: s.yield.average,
              max: s.yield.max,
              valueLabel: <ApyValue stats={s.yield} />,
              tooltip: (
                <ApyTooltip
                  title={`${s.token.symbol} · ${s.token.name ?? s.token.ticker}`}
                  stats={s.yield}
                  extra={s.token.issuer_name ?? undefined}
                />
              ),
              to: `/tokens/${s.token.id}`
            }))}
          />
        ) : (
          <Typography variant="body2" color="text.secondary">
            {t('summary.yield.empty', { n: minPositions })}
          </Typography>
        )}
      </ChartCard>
    </Box>
  );
}

function UnderlyingSection({ index, stats }: { index: RwaIndex; stats: Stats }) {
  const theme = useTheme();
  const t = useT();
  const f = useFormat();
  const accent = theme.palette.secondary.main;
  const [params, setParams] = useSearchParams();
  const [showAll, setShowAll] = useState(false);
  const detailRef = useRef<HTMLDivElement>(null);

  const overview = useMemo(
    () =>
      index.data.underlyings
        .map((u) => {
          const parts = issuerParts(t, stats, u);
          return { u, parts, total: parts.reduce((sum, p) => sum + p.value, 0) };
        })
        .sort((a, b) => b.total - a.total || a.u.ticker.localeCompare(b.u.ticker)),
    [index, stats, t]
  );
  // The chart ranks underlyings by positions; the picker lists them alphabetically.
  const pickerOptions = useMemo(() => [...overview].sort((a, b) => a.u.ticker.localeCompare(b.u.ticker)).map((o) => o.u), [overview]);

  const selectedTicker = params.get('asset') ?? overview[0]?.u.ticker;
  const selected = index.underlyingByTicker.get(selectedTicker ?? '') ?? overview[0]?.u;
  const select = (ticker: string, scroll = false) => {
    const next = new URLSearchParams(params);
    next.set('asset', ticker);
    setParams(next, { replace: true });
    if (scroll) detailRef.current?.scrollIntoView({ behavior: 'smooth', block: 'start' });
  };

  const shown = showAll ? overview : overview.slice(0, 15);
  const legendIssuers = issuerSegments(
    t,
    stats.issuers.map((i) => ({
      id: i.issuer.id,
      name: i.issuer.name ?? t('common.unknownIssuer'),
      value: i.issuer.issuer_usefulness_score,
      color: i.color
    }))
  );

  return (
    <Box component="section" sx={{ marginBottom: 2 }}>
      <Typography variant="h3" component="h2" gutterBottom>
        {t('summary.underlying.title')}
      </Typography>
      <ChartCard title={t('summary.underlying.chart')} subtitle={t('summary.underlying.chartSubtitle')}>
        <Box sx={{ display: 'flex', gap: 2, flexWrap: 'wrap', marginBottom: 1.5 }}>
          {legendIssuers.map((s) => (
            <LegendKey key={s.id} color={s.color} label={s.label} />
          ))}
        </Box>
        <StackedBarList
          labelWidth={150}
          rows={shown.map(({ u, parts, total }) => ({
            id: u.id,
            label: (
              <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, minWidth: 0 }}>
                <TokenIcon symbol={u.ticker} src={logoUrl(u.logo)} avatarProps={{ sx: { width: 24, height: 24, fontSize: 10 } }} />
                <Typography variant="body2" noWrap sx={{ fontWeight: 500 }}>
                  {u.ticker}
                </Typography>
              </Box>
            ),
            valueLabel: (
              <Typography variant="body2" component="span" sx={{ fontWeight: 600 }}>
                {f.num(total)}
              </Typography>
            ),
            segments: issuerSegments(t, parts),
            selected: u.id === selected?.id,
            onClick: () => select(u.ticker, true)
          }))}
        />
        {overview.length > 15 && (
          <Button size="small" onClick={() => setShowAll(!showAll)} aria-expanded={showAll} sx={{ marginTop: 1 }}>
            {showAll ? t('common.showTop', { n: 15 }) : t('common.showAll', { n: overview.length })}
          </Button>
        )}
      </ChartCard>

      <Box ref={detailRef} sx={{ scrollMarginTop: '100px', marginTop: 3 }}>
        <Grid container spacing={2} sx={{ marginBottom: 2 }}>
          <Grid size={{ xs: 12, md: 4 }}>
            <Autocomplete
              options={pickerOptions}
              value={selected ?? null}
              disableClearable={!!selected}
              getOptionLabel={(u) => `${u.ticker} — ${u.name}`}
              isOptionEqualToValue={(a, b) => a.id === b.id}
              onChange={(_, u) => u && select(u.ticker)}
              renderInput={(p) => <TextField {...p} label={t('summary.underlying.picker')} size="small" />}
              size="small"
              fullWidth
            />
          </Grid>
        </Grid>
        {selected && <UnderlyingDetail index={index} stats={stats} u={selected} parts={issuerParts(t, stats, selected)} accent={accent} />}
      </Box>
    </Box>
  );
}

interface DetailProps {
  index: RwaIndex;
  stats: Stats;
  u: Underlying;
  parts: { id: string; name: string; value: number; color: string }[];
  accent: string;
}

function UnderlyingDetail({ index, stats, u, parts, accent }: DetailProps) {
  const t = useT();
  const f = useFormat();
  const navigate = useLocaleNavigate();
  const versions = u.token_ids
    .map((id) => stats.tokenStatById.get(id))
    .filter((s): s is TokenStat => !!s)
    .sort((a, b) => b.eligible.size - a.eligible.size || (b.yield?.average ?? 0) - (a.yield?.average ?? 0));
  const inDefi = versions.filter((s) => s.eligible.size > 0).length;
  const assetYield = yieldStats(index, union(versions.map((s) => s.yieldKeys)));
  const withYield = versions
    .filter((s): s is TokenStat & { yield: YieldStats } => !!s.yield)
    .sort((a, b) => b.yield.average - a.yield.average);
  const noYield = versions.filter((s) => !s.yield);
  const versionsOf = (issuerId: string) =>
    versions
      .filter((s) => s.token.issuer_id === issuerId)
      .map((s) => s.token.symbol)
      .join(', ');

  return (
    <Box>
      <Box sx={{ display: 'flex', alignItems: 'center', gap: 1.5, marginBottom: 2, flexWrap: 'wrap' }}>
        <TokenIcon symbol={u.ticker} src={logoUrl(u.logo)} />
        <Box>
          <Typography variant="h4" component="h3">
            {u.ticker}
          </Typography>
          <Typography variant="body2" color="text.secondary">
            {u.name} ·{' '}
            <Link component={LocaleLink} to={`/assets/${u.ticker}`} underline="always">
              {t('summary.underlying.open')}
            </Link>
          </Typography>
        </Box>
      </Box>

      <Box sx={{ marginBottom: 2 }}>
        <StatGrid
          stats={[
            { label: t('stat.eligible'), value: f.num(u.eligible_opportunities_count), hint: t('stat.eligible.hintDistinctAll') },
            { label: t('summary.underlying.inDefi'), value: `${inDefi} / ${versions.length}`, hint: t('summary.underlying.inDefi.hint') },
            { label: t('summary.stat.positionsWithApy'), value: f.num(assetYield?.count ?? 0) },
            { label: t('summary.stat.avgApy'), value: pctOf(f, assetYield?.average), hint: t('summary.stat.avgApy.hint') },
            { label: t('summary.stat.maxApy'), value: pctOf(f, assetYield?.max) }
          ]}
        />
      </Box>

      <Grid container spacing={2} sx={{ marginBottom: 2 }}>
        <Grid size={{ xs: 12, md: 6 }}>
          <ChartCard title={t('summary.underlying.readiness')} subtitle={t('summary.underlying.readiness.subtitle', { ticker: u.ticker })}>
            {u.eligible_opportunities_count > 0 ? (
              <DonutChart
                segments={issuerSegments(
                  t,
                  parts.map((p) => ({ ...p, detail: versionsOf(p.id) }))
                )}
                centerCaption={t('summary.eligiblePositions')}
              />
            ) : (
              <Typography variant="body2" color="text.secondary">
                {t('summary.underlying.noEligible', { ticker: u.ticker })}
              </Typography>
            )}
          </ChartCard>
        </Grid>
        <Grid size={{ xs: 12, md: 6 }}>
          <ChartCard title={t('summary.underlying.yield')} subtitle={t('summary.underlying.yield.subtitle')}>
            {withYield.length ? (
              <>
                <YieldKeys accent={accent} reference={stats.overallYield?.average} />
                <RangeBarList
                  color={accent}
                  reference={stats.overallYield?.average}
                  labelWidth={150}
                  items={withYield.map((s) => ({
                    id: String(s.token.id),
                    label: (
                      <TokenLabel
                        token={s.token}
                        caption={`${s.token.issuer_name ?? t('common.unknownIssuer')} · ${t('summary.yield.positions', { n: s.yield.count })}`}
                      />
                    ),
                    average: s.yield.average,
                    max: s.yield.max,
                    valueLabel: <ApyValue stats={s.yield} />,
                    tooltip: (
                      <ApyTooltip title={`${s.token.symbol} · ${s.token.issuer_name ?? t('common.unknownIssuer')}`} stats={s.yield} />
                    ),
                    to: `/tokens/${s.token.id}`
                  }))}
                />
              </>
            ) : (
              <Typography variant="body2" color="text.secondary">
                {t('summary.underlying.noYield', { ticker: u.ticker })}
              </Typography>
            )}
            {withYield.length > 0 && noYield.length > 0 && (
              <Typography variant="caption" color="text.secondary" component="div" sx={{ marginTop: 1.5 }}>
                {t('summary.underlying.noApyList', { list: noYield.map((s) => s.token.symbol).join(', ') })}
              </Typography>
            )}
          </ChartCard>
        </Grid>
      </Grid>

      <DataTableContainer sx={stickyColumn(1)}>
        <Table size="small">
          <TableCaption>{t('table.tokenSummary')}</TableCaption>
          <TableHead>
            <TableRow>
              <TableCell>{t('col.token')}</TableCell>
              <TableCell>{t('col.issuer')}</TableCell>
              <TableCell align="right">{t('col.eligiblePositions')}</TableCell>
              <TableCell align="right">{t('col.positionsWithApy')}</TableCell>
              <TableCell align="right">{t('col.avgApy')}</TableCell>
              <TableCell align="right">{t('col.maxApy')}</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {versions.map((s) => (
              <TableRow key={s.token.id} hover onClick={() => navigate(`/tokens/${s.token.id}`)} sx={{ cursor: 'pointer' }}>
                <TableCell component="th" scope="row" sx={{ fontWeight: 'inherit' }}>
                  <RowLink to={`/tokens/${s.token.id}`}>
                    <TokenLabel token={s.token} caption={s.token.name ?? s.token.ticker} />
                  </RowLink>
                </TableCell>
                <TableCell>
                  <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                    <Box
                      aria-hidden="true"
                      sx={{
                        width: 10,
                        height: 10,
                        borderRadius: '3px',
                        flexShrink: 0,
                        backgroundColor: stats.issuerColor.get(s.token.issuer_id ?? '') ?? OTHER_COLOR
                      }}
                    />
                    {s.token.issuer_name ?? t('common.unknownIssuer')}
                  </Box>
                </TableCell>
                <TableCell align="right">{s.eligible.size}</TableCell>
                <TableCell align="right">{s.yield?.count ?? 0}</TableCell>
                <TableCell align="right">{pctOf(f, s.yield?.average)}</TableCell>
                <TableCell align="right">{pctOf(f, s.yield?.max)}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </DataTableContainer>
    </Box>
  );
}

// ==============================|| PAGE ||============================== //

function SummaryView({ index }: { index: RwaIndex }) {
  const t = useT();
  const f = useFormat();
  const series = useSeriesColors();
  const stats = useMemo(() => buildStats(index, series), [index, series]);
  const { summary, settings: s } = index.data;
  const thresholds = {
    liquidity: f.usd(s.min_eligible_liquidity_usd),
    hours: s.max_position_age_hours,
    threshold: f.pct(s.yield_apy_review_threshold_percent, 0)
  };
  const inDefi = stats.tokens.filter((tk) => tk.eligible.size > 0).length;
  const overall = stats.overallYield;
  const best = stats.tokens.reduce<TokenStat | null>(
    (top, tk) => (tk.yield && (!top?.yield || tk.yield.max > top.yield.max) ? tk : top),
    null
  );
  const leader = stats.issuers[0];
  const presenceLeader = byDefiPresence(stats.issuers)[0];
  const totalScore = stats.issuers.reduce((sum, i) => sum + i.issuer.issuer_usefulness_score, 0);

  return (
    <Box sx={{ width: '100%' }}>
      <Typography variant="h3" component="h1" gutterBottom>
        {t('summary.title')}
      </Typography>
      <Typography variant="body2" color="text.secondary" sx={{ marginBottom: 2, maxWidth: 900 }}>
        {t('summary.intro')}
      </Typography>
      <MethodNote data={index.data} />

      <Box sx={{ marginBottom: 4 }}>
        <StatGrid
          stats={[
            { label: t('stat.eligible'), value: f.num(summary.eligible_opportunities_count) },
            {
              label: t('summary.stat.tokensInDefi'),
              value: shareOf(f, inDefi, stats.tokens.length),
              hint: t('summary.stat.tokensInDefi.hint', { n: inDefi, total: stats.tokens.length })
            },
            {
              label: t('summary.stat.topIssuer'),
              value: leader ? (leader.issuer.name ?? DASH) : DASH,
              hint: leader
                ? t('summary.stat.topIssuer.hint', { share: shareOf(f, leader.issuer.issuer_usefulness_score, totalScore) })
                : undefined
            },
            {
              label: t('summary.stat.topCoverage'),
              value: presenceLeader ? (presenceLeader.issuer.name ?? DASH) : DASH,
              hint: presenceLeader
                ? t('summary.stat.topCoverage.hint', {
                    n: presenceLeader.inDefi,
                    total: presenceLeader.versions,
                    share: shareOf(f, presenceLeader.inDefi, presenceLeader.versions)
                  })
                : undefined
            },
            { label: t('summary.stat.positionsWithApy'), value: f.num(overall?.count ?? 0), hint: t('summary.stat.positionsWithApy.hint') },
            { label: t('summary.stat.avgApy'), value: pctOf(f, overall?.average), hint: t('summary.stat.avgApy.hint') },
            {
              label: t('summary.stat.maxApy'),
              value: pctOf(f, overall?.max),
              hint: best ? `${best.token.symbol} (${best.token.issuer_name ?? t('common.unknownIssuer')})` : undefined
            },
            {
              label: t('summary.stat.tokensWithApy'),
              value: f.num(stats.tokens.filter((tk) => tk.yield).length),
              hint: t('summary.stat.tokensWithApy.hint', { total: stats.tokens.length })
            }
          ]}
        />
      </Box>

      <IssuersSection stats={stats} />
      <YieldSection index={index} stats={stats} />
      <UnderlyingSection index={index} stats={stats} />

      <Legend
        items={[
          { term: t('legend.summary.eligibleTerm'), description: t('legend.summary.eligible', thresholds) },
          { term: t('col.share'), description: t('legend.summary.share') },
          { term: t('col.tokensInDefi'), description: t('legend.summary.tokensInDefi') },
          { term: t('col.protocols'), description: t('legend.summary.protocols') },
          { term: t('summary.stat.avgApy'), description: t('legend.summary.avgApy', thresholds) },
          { term: t('summary.stat.maxApy'), description: t('legend.summary.maxApy') },
          { term: t('legend.summary.basketAverageTerm'), description: t('legend.summary.basketAverage') },
          { term: t('legend.summary.posTerm'), description: t('legend.summary.pos') }
        ]}
      />
    </Box>
  );
}

export default function SummaryPage() {
  return <DataGate>{(index) => <SummaryView index={index} />}</DataGate>;
}
