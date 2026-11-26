import { ReactNode, useMemo, useState } from 'react';
import Box from '@mui/material/Box';
import ButtonBase from '@mui/material/ButtonBase';
import Chip from '@mui/material/Chip';
import FormControlLabel from '@mui/material/FormControlLabel';
import Switch from '@mui/material/Switch';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableHead from '@mui/material/TableHead';
import TableRow from '@mui/material/TableRow';
import Tooltip from '@mui/material/Tooltip';
import Typography from '@mui/material/Typography';

import { bestYieldByToken, logoUrl, relationRows, RwaIndex, tokenProtocols } from 'api/rwa';
import { TokenIcon } from 'components/TokenIcon';
import VisuallyHidden from 'ui-component/VisuallyHidden';
import DataTableContainer from './DataTableContainer';
import TableCaption from './TableCaption';
import { stickyColumn } from './tableSx';
import RowLink from './RowLink';
import { RelationRow, Token } from 'types/rwa';
import { compareNullable, deploymentChip, networkName, num } from 'utils/rwa';
import { useFormat, useLocaleNavigate, useT } from 'i18n';
import SortHeader, { HeaderCell, SortOrder, useSort } from './SortHeader';
import TablePager from './TablePager';
import ProtocolsCell from './ProtocolsCell';
import Age from './Age';
import OpportunityDialog from './OpportunityDialog';

export type TokenSortField = 'ticker' | 'score' | 'raw' | 'protocols' | 'apy' | 'mcap';
type Field = TokenSortField;

const sortValue: Record<Exclude<Field, 'score'>, (t: Token) => number | string | null> = {
  ticker: (t) => t.ticker,
  raw: (t) => t.counts.raw_confirmed_opportunities_count,
  protocols: (t) => t.counts.eligible_protocols_count,
  apy: (t) => num(t.yield.max_reported_apy_percent),
  mcap: (t) => num(t.market_cap_usd)
};

// Score with the documented tie-breaks, so the order also works across underlyings.
const scoreKey = (t: Token) => [
  t.usefulness_score,
  t.counts.eligible_protocols_count,
  t.counts.eligible_networks_count,
  t.counts.direct_eligible_count
];

export function compareTokens(a: Token, b: Token, field: Field, order: SortOrder): number {
  if (field !== 'score') return compareNullable(sortValue[field](a), sortValue[field](b), order);
  const ka = scoreKey(a);
  const kb = scoreKey(b);
  const i = ka.findIndex((v, n) => v !== kb[n]);
  if (i === -1) return 0;
  return order === 'asc' ? ka[i] - kb[i] : kb[i] - ka[i];
}

export const deploymentLabel = (d: Token['deployments'][number]) => (d.network ? networkName(d.network) : d.cmc_network);

export function NetworkChips({ token }: { token: Token }) {
  const t = useT();
  return (
    <Box component="ul" sx={{ display: 'flex', gap: 0.5, flexWrap: 'wrap', maxWidth: 180, listStyle: 'none', margin: 0, padding: 0 }}>
      {token.deployments.map((d) => {
        const status = deploymentChip(t, d.status).label;
        return (
          <Tooltip key={`${d.cmc_network}:${d.address}`} title={status} describeChild arrow>
            <Chip
              component="li"
              size="small"
              label={
                <>
                  {deploymentLabel(d)}
                  <VisuallyHidden>: {status}</VisuallyHidden>
                </>
              }
              color={d.status === 'indexed' ? 'primary' : 'default'}
              variant={d.status === 'indexed' ? 'filled' : 'outlined'}
            />
          </Tooltip>
        );
      })}
    </Box>
  );
}

interface VersionsTableProps {
  index: RwaIndex;
  tokens: Token[];
  showUnderlying?: boolean; // one table for many underlyings
  underlyingLogos?: Map<string, string | null>; // ticker -> logo path, for showUnderlying
  paginate?: boolean;
  positionColumns?: boolean; // a switch above the table shows Raw, Direct / Indirect and each token's best reported-APY position
  toolbarEnd?: ReactNode; // shown on the right of that switch
}

// Comparison of tokenized versions, ranked by usefulness score.
export default function VersionsTable({
  index,
  tokens,
  showUnderlying = false,
  underlyingLogos,
  paginate = false,
  positionColumns = false,
  toolbarEnd
}: VersionsTableProps) {
  const t = useT();
  const f = useFormat();
  const navigate = useLocaleNavigate();
  const { sortField, sortOrder, onSort, reset: resetSort } = useSort<Field>('score');
  const [page, setPage] = useState(1);
  const [rowsPerPage, setRowsPerPage] = useState(25);
  const [showPositions, setShowPositions] = useState(false);
  const [selected, setSelected] = useState<RelationRow | null>(null);
  const withPositions = positionColumns && showPositions;
  const withCounts = !positionColumns || showPositions; // without the switch these columns are always shown
  const toggleColumns = (on: boolean) => {
    setShowPositions(on);
    if (!on && sortField === 'raw') resetSort(); // no sort by a column that is no longer shown
  };
  const best = useMemo(
    () => (withPositions ? bestYieldByToken(relationRows(index, tokens)) : new Map<number, RelationRow>()),
    [withPositions, index, tokens]
  );

  const sorted = [...tokens].sort((a, b) => compareTokens(a, b, sortField, sortOrder) || a.id - b.id);
  const pageCount = Math.max(1, Math.ceil(sorted.length / rowsPerPage));
  const currentPage = Math.min(page, pageCount); // filters may shrink the list
  const visible = paginate ? sorted.slice((currentPage - 1) * rowsPerPage, currentPage * rowsPerPage) : sorted;
  const handleSort = (field: Field) => {
    onSort(field);
    setPage(1);
  };
  const header = (field: Field, label: string, hint?: string) => (
    <SortHeader field={field} label={label} hint={hint} sortField={sortField} sortOrder={sortOrder} onSort={handleSort} />
  );

  return (
    <>
      {(positionColumns || toolbarEnd) && (
        <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', flexWrap: 'wrap', gap: 1, marginBottom: 1 }}>
          {positionColumns ? (
            <FormControlLabel
              control={
                <Switch
                  checked={showPositions}
                  onChange={(e) => toggleColumns(e.target.checked)}
                  slotProps={{ input: { role: 'switch' } }}
                />
              }
              label={t('tokenized.moreColumns')}
            />
          ) : (
            <span />
          )}
          {toolbarEnd}
        </Box>
      )}
      <DataTableContainer sx={[{ marginBottom: paginate ? 2 : 4 }, stickyColumn(2)]}>
        <Table sx={{ minWidth: 650, '& .MuiTableCell-root': { paddingX: 1.5 } }}>
          <TableCaption>{t('table.tokenized')}</TableCaption>
          <TableHead>
            <TableRow>
              {showUnderlying ? header('ticker', t('col.underlying')) : <TableCell>{t('col.rank')}</TableCell>}
              <TableCell>{t('col.token')}</TableCell>
              <TableCell>{t('col.issuer')}</TableCell>
              <TableCell>{t('col.networks')}</TableCell>
              {header('score', t('col.score'), t('col.score.hint'))}
              {withCounts && header('raw', t('col.raw'))}
              {header('protocols', t('col.protocols'))}
              {withCounts && <HeaderCell label={t('col.directIndirect')} />}
              {header('apy', t('col.apyMaxMedian'), t('col.apyMaxMedian.hint'))}
              {header('mcap', t('col.mcapCmc'))}
              {withPositions && (
                <>
                  <HeaderCell label={t('col.bestPosition')} hint={t('col.bestPosition.hint')} />
                  <HeaderCell label={t('col.liquidity')} hint={t('col.liquidity.hintBest')} />
                  <HeaderCell label={t('col.updated')} hint={t('col.updated.hintBest')} />
                </>
              )}
            </TableRow>
          </TableHead>
          <TableBody>
            {visible.length === 0 && (
              <TableRow>
                <TableCell colSpan={8 + (withCounts ? 2 : 0) + (withPositions ? 3 : 0)} sx={{ color: 'text.secondary' }}>
                  {showUnderlying ? t('tokenized.empty') : t('tokenized.emptyUnderlying')}
                </TableCell>
              </TableRow>
            )}
            {visible.map((tk) => (
              <TableRow key={tk.id} hover onClick={() => navigate(`/tokens/${tk.id}`)} sx={{ cursor: 'pointer' }}>
                <TableCell>
                  {showUnderlying ? (
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                      <TokenIcon
                        symbol={tk.ticker}
                        src={logoUrl(underlyingLogos?.get(tk.ticker))}
                        avatarProps={{ sx: { width: 24, height: 24, fontSize: 10 } }}
                      />
                      <Typography variant="body2" sx={{ fontWeight: 500 }}>
                        {tk.ticker}
                      </Typography>
                    </Box>
                  ) : (
                    tk.rank_in_underlying
                  )}
                </TableCell>
                <TableCell component="th" scope="row" sx={{ fontWeight: 'inherit' }}>
                  <RowLink to={`/tokens/${tk.id}`}>
                    <Tooltip title={tk.name ?? ''} describeChild arrow>
                      <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, whiteSpace: 'nowrap' }}>
                        <TokenIcon
                          symbol={tk.symbol ?? '?'}
                          src={logoUrl(tk.logo)}
                          avatarProps={{ sx: { width: 24, height: 24, fontSize: 10 } }}
                        />
                        <Typography variant="body2" component="span" sx={{ fontWeight: 500 }}>
                          {tk.symbol}
                        </Typography>
                        {tk.name && <VisuallyHidden>, {tk.name}</VisuallyHidden>}
                        {tk.kind === 'wrapper' && <Chip size="small" label={t('common.wrapper')} />}
                      </Box>
                    </Tooltip>
                  </RowLink>
                </TableCell>
                <TableCell>{tk.issuer_name ?? '—'}</TableCell>
                <TableCell>
                  <NetworkChips token={tk} />
                </TableCell>
                <TableCell>
                  <Typography variant="h5" component="span">
                    {tk.usefulness_score}
                  </Typography>
                </TableCell>
                {withCounts && <TableCell>{tk.counts.raw_confirmed_opportunities_count}</TableCell>}
                <TableCell>
                  <ProtocolsCell count={tk.counts.eligible_protocols_count} names={tokenProtocols(index, tk)} />
                </TableCell>
                {withCounts && (
                  <TableCell>
                    {tk.counts.direct_eligible_count} / {tk.counts.indirect_eligible_count}
                  </TableCell>
                )}
                <TableCell>
                  {f.pct(tk.yield.max_reported_apy_percent)}
                  <Typography variant="body2" color="text.secondary">
                    {f.pct(tk.yield.median_reported_apy_percent)}
                  </Typography>
                </TableCell>
                <TableCell>{f.usd(tk.market_cap_usd)}</TableCell>
                {withPositions && <PositionCells row={best.get(tk.id)} onOpen={setSelected} />}
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </DataTableContainer>
      {paginate && (
        <TablePager count={sorted.length} page={currentPage} rowsPerPage={rowsPerPage} onPage={setPage} onRowsPerPage={setRowsPerPage} />
      )}
      {positionColumns && <OpportunityDialog index={index} row={selected} onClose={() => setSelected(null)} />}
    </>
  );
}

// Best reported-APY position of a token: the name opens the position details, the row click still opens the token.
function PositionCells({ row, onOpen }: { row: RelationRow | undefined; onOpen: (row: RelationRow) => void }) {
  const t = useT();
  const f = useFormat();
  if (!row)
    return (
      <>
        <TableCell sx={{ color: 'text.secondary' }}>{t('tokenized.noBestPosition')}</TableCell>
        <TableCell>—</TableCell>
        <TableCell>—</TableCell>
      </>
    );
  const o = row.opportunity;
  return (
    <>
      <TableCell>
        <ButtonBase
          onClick={(e) => {
            e.stopPropagation();
            onOpen(row);
          }}
          aria-haspopup="dialog"
          title={o.name ?? undefined}
          sx={{
            display: 'block',
            textAlign: 'left',
            borderRadius: '4px',
            '&:focus-visible': { outline: '2px solid', outlineColor: 'secondary.main', outlineOffset: 2 }
          }}
        >
          <Typography variant="body2" sx={{ fontWeight: 500, whiteSpace: 'nowrap' }}>
            {o.platform_name} · {networkName(o.network)}
          </Typography>
          <Typography
            variant="body2"
            color="text.secondary"
            sx={{ maxWidth: 220, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}
          >
            {o.symbol ?? o.key}
          </Typography>
          <VisuallyHidden>. {t('common.details')}</VisuallyHidden>
        </ButtonBase>
      </TableCell>
      <TableCell>{f.usd(o.liquidity_usd)}</TableCell>
      <TableCell>
        <Age iso={o.updated_at} />
      </TableCell>
    </>
  );
}
