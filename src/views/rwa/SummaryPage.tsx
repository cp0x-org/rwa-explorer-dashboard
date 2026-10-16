import { useMemo, useRef, useState } from 'react';
import { Link as RouterLink, useNavigate, useSearchParams } from 'react-router-dom';
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
import { DASH, fmtNum, fmtPct } from 'utils/rwa';
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
function issuerSegments(items: { id: string; name: string; value: number; color: string; detail?: string }[]): Segment[] {
  const segments: Segment[] = [];
  let other = 0;
  items.forEach((i) => {
    if (i.value <= 0) return;
    if (i.color === OTHER_COLOR) other += i.value;
    else segments.push({ id: i.id, label: i.name, value: i.value, color: i.color, detail: i.detail });
  });
  if (other > 0) segments.push({ id: OTHER_ID, label: 'Other issuers', value: other, color: OTHER_COLOR });
  return segments.sort((a, b) => (a.id === OTHER_ID ? 1 : b.id === OTHER_ID ? -1 : b.value - a.value));
}

// Eligible positions of each issuer's tokenized assets of an underlying (a pool shared by two of one issuer's tokens counts once).
function issuerParts(stats: Stats, u: Underlying) {
  const keys = new Map<string, Set<string>[]>();
  u.token_ids.forEach((id) => {
    const s = stats.tokenStatById.get(id);
    if (!s?.token.issuer_id) return;
    keys.set(s.token.issuer_id, [...(keys.get(s.token.issuer_id) ?? []), s.eligible]);
  });
  return stats.issuers
    .filter((i) => keys.has(i.issuer.id))
    .map((i) => ({ id: i.issuer.id, name: i.issuer.name ?? 'Unknown issuer', value: union(keys.get(i.issuer.id)!).size, color: i.color }));
}

const pct = (value: number | null | undefined) => (value === null || value === undefined ? DASH : fmtPct(String(value)));
const share = (part: number, whole: number) => (whole > 0 ? `${((part / whole) * 100).toFixed(1)}%` : DASH);

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
  return (
    <Typography variant="body2" component="span">
      <Box component="span" sx={{ fontWeight: 600 }}>
        {pct(stats.average)}
      </Box>
      <Box component="span" sx={{ color: 'text.secondary' }}>
        {' / '}
        {pct(stats.max)}
      </Box>
    </Typography>
  );
}

function ApyTooltip({ title, stats, extra }: { title: string; stats: YieldStats; extra?: string }) {
  return (
    <Box>
      <Typography variant="subtitle2" sx={{ color: 'inherit', fontWeight: 600 }}>
        Avg {pct(stats.average)} · Max {pct(stats.max)}
      </Typography>
      <Typography variant="caption" sx={{ color: 'inherit', opacity: 0.8, display: 'block' }}>
        {title}
      </Typography>
      <Typography variant="caption" sx={{ color: 'inherit', opacity: 0.8, display: 'block' }}>
        {stats.count} eligible {stats.count === 1 ? 'position' : 'positions'} with APY &gt; 0{extra ? ` · ${extra}` : ''}
      </Typography>
    </Box>
  );
}

function YieldKeys({ accent, reference }: { accent: string; reference?: number }) {
  const theme = useTheme();
  return (
    <Box sx={{ display: 'flex', gap: 2, flexWrap: 'wrap', marginBottom: 1.5 }}>
      <LegendKey color={accent} label="Average APY" />
      <LegendKey color={accent} label="Max APY" shape="dot" />
      {reference !== undefined && (
        <LegendKey color={theme.palette.text.secondary} label={`Basket average ${pct(reference)}`} shape="line" />
      )}
    </Box>
  );
}

// ==============================|| SECTIONS ||============================== //

function IssuersSection({ stats }: { stats: Stats }) {
  const theme = useTheme();
  const navigate = useNavigate();
  const accent = theme.palette.secondary.main;
  const total = stats.issuers.reduce((sum, i) => sum + i.issuer.issuer_usefulness_score, 0);
  const segments = issuerSegments(
    stats.issuers.map((i) => ({
      id: i.issuer.id,
      name: i.issuer.name ?? 'Unknown issuer',
      value: i.issuer.issuer_usefulness_score,
      color: i.color,
      detail: `${i.inDefi} of ${i.versions} tokenized assets in DeFi`
    }))
  );
  const presence = byDefiPresence(stats.issuers);
  const diversity = byProtocolDiversity(stats.issuers);
  const maxPlatforms = Math.max(...diversity.map((i) => i.platforms.size), 0);

  return (
    <Box sx={{ marginBottom: 5 }}>
      <Typography variant="h3" gutterBottom>
        Issuers: DeFi readiness
      </Typography>
      <Grid container spacing={2} sx={{ marginBottom: 2 }}>
        <Grid size={{ xs: 12, md: 6, lg: 4 }}>
          <ChartCard
            title="Share of eligible positions"
            subtitle="Which issuer’s tokenized assets are used in DeFi the most (issuer score / sum of scores)"
          >
            <DonutChart
              segments={segments}
              centerCaption="eligible positions"
              onSelect={(id) => id !== OTHER_ID && navigate(`/issuers/${id}`)}
            />
          </ChartCard>
        </Grid>
        <Grid size={{ xs: 12, md: 6, lg: 4 }}>
          <ChartCard title="Tokenized assets in DeFi" subtitle="Share of the issuer’s tokenized assets with at least one eligible position">
            <BarList
              color={accent}
              max={100}
              labelWidth={150}
              items={presence.map((i) => ({
                id: i.issuer.id,
                // The count sits under the name so the value column stays one width and every bar shares the same track.
                label: (
                  <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, minWidth: 0 }}>
                    <Box sx={{ width: 8, height: 8, borderRadius: '50%', backgroundColor: i.color, flexShrink: 0 }} />
                    <Box sx={{ minWidth: 0 }}>
                      <Typography variant="body2" noWrap>
                        {i.issuer.name ?? 'Unknown issuer'}
                      </Typography>
                      <Typography variant="caption" noWrap sx={{ display: 'block', color: 'text.secondary', lineHeight: 1.2 }}>
                        {i.inDefi} of {i.versions} assets
                      </Typography>
                    </Box>
                  </Box>
                ),
                value: (i.inDefi / i.versions) * 100,
                valueLabel: (
                  <Typography variant="body2" component="span" sx={{ fontWeight: 600 }}>
                    {share(i.inDefi, i.versions)}
                  </Typography>
                ),
                tooltip: `${i.inDefi} of ${i.versions} tokenized assets have at least one eligible position`,
                to: `/issuers/${i.issuer.id}`
              }))}
            />
          </ChartCard>
        </Grid>
        <Grid size={{ xs: 12, md: 6, lg: 4 }}>
          <ChartCard
            title="Protocol diversity"
            subtitle="Distinct DeFi protocols hosting the issuer’s tokenized assets; many pools of one protocol count once"
          >
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
                      <Box sx={{ width: 8, height: 8, borderRadius: '50%', backgroundColor: i.color, flexShrink: 0 }} />
                      <Typography variant="body2" noWrap>
                        {i.issuer.name ?? 'Unknown issuer'}
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
                        {i.platforms.size === 1 ? ' protocol' : ' protocols'}
                      </Box>
                    </Typography>
                  ),
                  tooltip: names.length ? names.join(', ') : 'No eligible positions',
                  to: `/issuers/${i.issuer.id}`
                };
              })}
            />
          </ChartCard>
        </Grid>
      </Grid>

      <DataTableContainer sx={stickyColumn(1)}>
        <Table size="small" aria-label="issuer summary table">
          <TableHead>
            <TableRow>
              <TableCell>Issuer</TableCell>
              <TableCell align="right">Share</TableCell>
              <TableCell align="right">Eligible positions</TableCell>
              <TableCell align="right">Tokenized assets in DeFi</TableCell>
              <TableCell align="right">Protocols</TableCell>
              <TableCell align="right">Positions with APY</TableCell>
              <TableCell align="right">Avg APY</TableCell>
              <TableCell align="right">Max APY</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {stats.issuers.map((i) => (
              <TableRow key={i.issuer.id} hover onClick={() => navigate(`/issuers/${i.issuer.id}`)} sx={{ cursor: 'pointer' }}>
                <TableCell>
                  <RowLink to={`/issuers/${i.issuer.id}`}>
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                      <Box sx={{ width: 10, height: 10, borderRadius: '3px', backgroundColor: i.color, flexShrink: 0 }} />
                      <TokenIcon
                        symbol={i.issuer.name ?? '?'}
                        src={logoUrl(i.issuer.logo)}
                        avatarProps={{ sx: { width: 24, height: 24, fontSize: 10 } }}
                      />
                      <Typography variant="body2" sx={{ fontWeight: 500 }}>
                        {i.issuer.name ?? 'Unknown issuer'}
                      </Typography>
                    </Box>
                  </RowLink>
                </TableCell>
                <TableCell align="right">{share(i.issuer.issuer_usefulness_score, total)}</TableCell>
                <TableCell align="right">{fmtNum(i.issuer.issuer_usefulness_score)}</TableCell>
                <TableCell align="right">
                  {i.inDefi} / {i.versions}{' '}
                  <Typography component="span" variant="body2" color="text.secondary">
                    ({share(i.inDefi, i.versions)})
                  </Typography>
                </TableCell>
                <TableCell align="right">{i.platforms.size}</TableCell>
                <TableCell align="right">{i.yield?.count ?? 0}</TableCell>
                <TableCell align="right">{pct(i.yield?.average)}</TableCell>
                <TableCell align="right">{pct(i.yield?.max)}</TableCell>
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
    <Box sx={{ marginBottom: 5 }}>
      <Typography variant="h3" gutterBottom>
        Most profitable tokenized stocks
      </Typography>
      <Typography variant="body2" color="text.secondary" sx={{ marginBottom: 2 }}>
        Only eligible positions with a reported APY above zero are counted: zero or missing APY is left out so it does not drag the average
        down, APY above {fmtPct(index.data.settings.yield_apy_review_threshold_percent, 0)} is excluded as anomaly. {withYield.length} of{' '}
        {stats.tokens.length} tokenized assets have such positions.
      </Typography>

      <Grid container spacing={2} sx={{ marginBottom: 2 }}>
        <Grid size={{ xs: 12, sm: 4, md: 3 }}>
          <FormControl size="small" fullWidth>
            <InputLabel>Sort by</InputLabel>
            <Select value={sortBy} label="Sort by" onChange={(e) => setSortBy(e.target.value as YieldSort)}>
              <MenuItem value="average">Average APY</MenuItem>
              <MenuItem value="max">Max APY</MenuItem>
            </Select>
          </FormControl>
        </Grid>
        <Grid size={{ xs: 6, sm: 4, md: 3 }}>
          <FormControl size="small" fullWidth>
            <InputLabel>Min. positions with APY</InputLabel>
            <Select value={minPositions} label="Min. positions with APY" onChange={(e) => setMinPositions(Number(e.target.value))}>
              {[1, 2, 3, 5, 10].map((n) => (
                <MenuItem key={n} value={n}>
                  {n === 1 ? 'Any (≥ 1)' : `≥ ${n}`}
                </MenuItem>
              ))}
            </Select>
          </FormControl>
        </Grid>
        <Grid size={{ xs: 6, sm: 4, md: 3 }}>
          <FormControl size="small" fullWidth>
            <InputLabel>Show</InputLabel>
            <Select value={limit} label="Show" onChange={(e) => setLimit(Number(e.target.value))}>
              <MenuItem value={15}>Top 15</MenuItem>
              <MenuItem value={30}>Top 30</MenuItem>
              <MenuItem value={10000}>All</MenuItem>
            </Select>
          </FormControl>
        </Grid>
      </Grid>

      <ChartCard
        title={`Tokenized assets by ${sortBy === 'average' ? 'average' : 'max'} APY`}
        subtitle={`Showing ${rows.length} of ${candidates.length} tokenized assets · value: average / max APY`}
      >
        <YieldKeys accent={accent} reference={overall?.average} />
        {rows.length ? (
          <RangeBarList
            color={accent}
            reference={overall?.average}
            labelWidth={190}
            items={rows.map((s) => ({
              id: String(s.token.id),
              label: <TokenLabel token={s.token} caption={`${s.token.issuer_name ?? 'Unknown issuer'} · ${s.yield.count} pos.`} />,
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
            No tokenized assets with at least {minPositions} positions with APY.
          </Typography>
        )}
      </ChartCard>
    </Box>
  );
}

function UnderlyingSection({ index, stats }: { index: RwaIndex; stats: Stats }) {
  const theme = useTheme();
  const accent = theme.palette.secondary.main;
  const [params, setParams] = useSearchParams();
  const [showAll, setShowAll] = useState(false);
  const detailRef = useRef<HTMLDivElement>(null);

  const overview = useMemo(
    () =>
      index.data.underlyings
        .map((u) => {
          const parts = issuerParts(stats, u);
          return { u, parts, total: parts.reduce((sum, p) => sum + p.value, 0) };
        })
        .sort((a, b) => b.total - a.total || a.u.ticker.localeCompare(b.u.ticker)),
    [index, stats]
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
    stats.issuers.map((i) => ({
      id: i.issuer.id,
      name: i.issuer.name ?? 'Unknown issuer',
      value: i.issuer.issuer_usefulness_score,
      color: i.color
    }))
  );

  return (
    <Box sx={{ marginBottom: 2 }}>
      <Typography variant="h3" gutterBottom>
        By underlying asset
      </Typography>
      <ChartCard
        title="Eligible positions by issuer"
        subtitle="Each bar is one asset, split by the issuer of the tokenized assets used. Click an asset for its breakdown below."
      >
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
                {fmtNum(total)}
              </Typography>
            ),
            segments: issuerSegments(parts),
            selected: u.id === selected?.id,
            onClick: () => select(u.ticker, true)
          }))}
        />
        {overview.length > 15 && (
          <Button size="small" onClick={() => setShowAll(!showAll)} sx={{ marginTop: 1 }}>
            {showAll ? 'Show top 15' : `Show all ${overview.length} assets`}
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
              renderInput={(p) => <TextField {...p} label="Underlying asset" size="small" />}
              size="small"
              fullWidth
            />
          </Grid>
        </Grid>
        {selected && <UnderlyingDetail index={index} stats={stats} u={selected} parts={issuerParts(stats, selected)} accent={accent} />}
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
  const navigate = useNavigate();
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
          <Typography variant="h4">{u.ticker}</Typography>
          <Typography variant="body2" color="text.secondary">
            {u.name} ·{' '}
            <Link component={RouterLink} to={`/assets/${u.ticker}`} underline="hover">
              Open asset page
            </Link>
          </Typography>
        </Box>
      </Box>

      <Box sx={{ marginBottom: 2 }}>
        <StatGrid
          stats={[
            {
              label: 'Eligible positions',
              value: fmtNum(u.eligible_opportunities_count),
              hint: 'Distinct positions across all tokenized assets'
            },
            {
              label: 'Tokenized assets in DeFi',
              value: `${inDefi} / ${versions.length}`,
              hint: 'Tokenized assets with at least one eligible position'
            },
            { label: 'Positions with APY > 0', value: fmtNum(assetYield?.count ?? 0) },
            { label: 'Average APY', value: pct(assetYield?.average), hint: 'Mean over distinct positions with APY > 0' },
            { label: 'Max APY', value: pct(assetYield?.max) }
          ]}
        />
      </Box>

      <Grid container spacing={2} sx={{ marginBottom: 2 }}>
        <Grid size={{ xs: 12, md: 6 }}>
          <ChartCard
            title="DeFi readiness by issuer"
            subtitle={`Share of ${u.ticker} eligible positions by the issuer of the tokenized asset used`}
          >
            {u.eligible_opportunities_count > 0 ? (
              <DonutChart
                segments={issuerSegments(parts.map((p) => ({ ...p, detail: versionsOf(p.id) })))}
                centerCaption="eligible positions"
                onSelect={(id) => id !== OTHER_ID && navigate(`/issuers/${id}`)}
              />
            ) : (
              <Typography variant="body2" color="text.secondary">
                No eligible positions found for {u.ticker} in the current coverage.
              </Typography>
            )}
          </ChartCard>
        </Grid>
        <Grid size={{ xs: 12, md: 6 }}>
          <ChartCard title="Yield by tokenized asset" subtitle="Average and max APY over each token’s positions with APY > 0">
            {withYield.length ? (
              <>
                <YieldKeys accent={accent} reference={stats.overallYield?.average} />
                <RangeBarList
                  color={accent}
                  reference={stats.overallYield?.average}
                  labelWidth={150}
                  items={withYield.map((s) => ({
                    id: String(s.token.id),
                    label: <TokenLabel token={s.token} caption={`${s.token.issuer_name ?? 'Unknown issuer'} · ${s.yield.count} pos.`} />,
                    average: s.yield.average,
                    max: s.yield.max,
                    valueLabel: <ApyValue stats={s.yield} />,
                    tooltip: <ApyTooltip title={`${s.token.symbol} · ${s.token.issuer_name ?? 'Unknown issuer'}`} stats={s.yield} />,
                    to: `/tokens/${s.token.id}`
                  }))}
                />
              </>
            ) : (
              <Typography variant="body2" color="text.secondary">
                No tokenized {u.ticker} has an eligible position with APY above zero.
              </Typography>
            )}
            {withYield.length > 0 && noYield.length > 0 && (
              <Typography variant="caption" color="text.secondary" component="div" sx={{ marginTop: 1.5 }}>
                No APY &gt; 0: {noYield.map((s) => s.token.symbol).join(', ')}
              </Typography>
            )}
          </ChartCard>
        </Grid>
      </Grid>

      <DataTableContainer sx={stickyColumn(1)}>
        <Table size="small" aria-label="tokenized assets summary table">
          <TableHead>
            <TableRow>
              <TableCell>Token</TableCell>
              <TableCell>Issuer</TableCell>
              <TableCell align="right">Eligible positions</TableCell>
              <TableCell align="right">Positions with APY</TableCell>
              <TableCell align="right">Avg APY</TableCell>
              <TableCell align="right">Max APY</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {versions.map((s) => (
              <TableRow key={s.token.id} hover onClick={() => navigate(`/tokens/${s.token.id}`)} sx={{ cursor: 'pointer' }}>
                <TableCell>
                  <RowLink to={`/tokens/${s.token.id}`}>
                    <TokenLabel token={s.token} caption={s.token.name ?? s.token.ticker} />
                  </RowLink>
                </TableCell>
                <TableCell>
                  <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                    <Box
                      sx={{
                        width: 10,
                        height: 10,
                        borderRadius: '3px',
                        flexShrink: 0,
                        backgroundColor: stats.issuerColor.get(s.token.issuer_id ?? '') ?? OTHER_COLOR
                      }}
                    />
                    {s.token.issuer_name ?? 'Unknown issuer'}
                  </Box>
                </TableCell>
                <TableCell align="right">{s.eligible.size}</TableCell>
                <TableCell align="right">{s.yield?.count ?? 0}</TableCell>
                <TableCell align="right">{pct(s.yield?.average)}</TableCell>
                <TableCell align="right">{pct(s.yield?.max)}</TableCell>
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
  const series = useSeriesColors();
  const stats = useMemo(() => buildStats(index, series), [index, series]);
  const { summary, settings: s } = index.data;
  const inDefi = stats.tokens.filter((t) => t.eligible.size > 0).length;
  const overall = stats.overallYield;
  const best = stats.tokens.reduce<TokenStat | null>((top, t) => (t.yield && (!top?.yield || t.yield.max > top.yield.max) ? t : top), null);
  const leader = stats.issuers[0];
  const presenceLeader = byDefiPresence(stats.issuers)[0];
  const totalScore = stats.issuers.reduce((sum, i) => sum + i.issuer.issuer_usefulness_score, 0);

  return (
    <Box sx={{ width: '100%' }}>
      <Typography variant="h3" gutterBottom>
        Summary
      </Typography>
      <MethodNote data={index.data} />

      <Box sx={{ marginBottom: 4 }}>
        <StatGrid
          stats={[
            { label: 'Eligible positions', value: fmtNum(summary.eligible_opportunities_count) },
            {
              label: 'Tokenized assets in DeFi',
              value: `${share(inDefi, stats.tokens.length)}`,
              hint: `${inDefi} of ${stats.tokens.length} tokenized assets have at least one eligible position`
            },
            {
              label: 'Top issuer',
              value: leader ? `${leader.issuer.name ?? DASH}` : DASH,
              hint: leader ? `${share(leader.issuer.issuer_usefulness_score, totalScore)} of eligible positions` : undefined
            },
            {
              label: 'Top issuer by DeFi coverage',
              value: presenceLeader ? `${presenceLeader.issuer.name ?? DASH}` : DASH,
              hint: presenceLeader
                ? `${presenceLeader.inDefi} of ${presenceLeader.versions} tokenized assets (${share(presenceLeader.inDefi, presenceLeader.versions)}) have at least one eligible position`
                : undefined
            },
            {
              label: 'Positions with APY > 0',
              value: fmtNum(overall?.count ?? 0),
              hint: 'Distinct eligible positions with a positive reported APY; zero and missing APY excluded'
            },
            { label: 'Average APY', value: pct(overall?.average), hint: 'Mean over distinct eligible positions with APY > 0' },
            {
              label: 'Max APY',
              value: pct(overall?.max),
              hint: best ? `${best.token.symbol} (${best.token.issuer_name ?? 'Unknown issuer'})` : undefined
            },
            {
              label: 'Tokenized assets with APY > 0',
              value: fmtNum(stats.tokens.filter((t) => t.yield).length),
              hint: `Out of ${stats.tokens.length} tokenized assets`
            }
          ]}
        />
      </Box>

      <IssuersSection stats={stats} />
      <YieldSection index={index} stats={stats} />
      <UnderlyingSection index={index} stats={stats} />

      <Legend
        items={[
          {
            term: 'Eligible position',
            description:
              `A DeFi position that contains the token, confirmed by its contract address (directly or via LP → vault), is a real ` +
              `use-case (DEX LP, lending supply, vault, staking, yield market), has liquidity ≥ $${Number(s.min_eligible_liquidity_usd).toLocaleString('en-US')} ` +
              `and source data not older than ${s.max_position_age_hours}h at collection.`
          },
          {
            term: 'Share',
            description:
              'Issuer score (unique eligible positions of its tokenized assets) divided by the sum of all issuer scores. A pool holding ' +
              'tokenized assets of two issuers counts for each of them.'
          },
          {
            term: 'Tokenized assets in DeFi',
            description:
              'Tokenized assets with at least one eligible position / all tokenized assets of the issuer or underlying in the basket. ' +
              'Shows how much of the issuer’s line-up is usable in DeFi, independent of how many positions each token has.'
          },
          {
            term: 'Protocols',
            description:
              'Number of distinct DeFi protocols (Uniswap V3, Morpho, Beefy…) with at least one eligible position holding any of the ' +
              'issuer’s tokenized assets. Many pools of the same protocol count once; protocol versions (Uniswap V2 vs V3) count separately.'
          },
          {
            term: 'Average APY',
            description:
              'Mean reported APY over distinct eligible positions with APY above zero. Positions with zero or no reported APY are left out, ' +
              `APY above ${fmtPct(s.yield_apy_review_threshold_percent, 0)} is excluded as anomaly. Reported APY of one position is not an expected return.`
          },
          { term: 'Max APY', description: 'Highest reported APY among those positions.' },
          {
            term: 'Basket average',
            description: 'Average APY over all eligible positions with APY > 0 in the basket; drawn as a vertical line on the yield charts.'
          },
          {
            term: 'pos.',
            description:
              'Number of eligible positions with APY > 0 behind the token’s average. One position means the average equals the max.'
          }
        ]}
      />
    </Box>
  );
}

export default function SummaryPage() {
  return <DataGate>{(index) => <SummaryView index={index} />}</DataGate>;
}
