import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import Box from '@mui/material/Box';
import Chip from '@mui/material/Chip';
import Paper from '@mui/material/Paper';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableContainer from '@mui/material/TableContainer';
import TableHead from '@mui/material/TableHead';
import TableRow from '@mui/material/TableRow';
import Tooltip from '@mui/material/Tooltip';
import Typography from '@mui/material/Typography';

import { Token } from 'types/rwa';
import { compareNullable, DEPLOYMENT_STATUS, fmtPct, fmtUsd, networkName, num } from 'utils/rwa';
import SortHeader, { SortOrder, useSort } from './SortHeader';
import { CoverageChip } from './StatusChip';
import TablePager from './TablePager';

type Field = 'ticker' | 'score' | 'raw' | 'protocols' | 'apy' | 'mcap';

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

function compareTokens(a: Token, b: Token, field: Field, order: SortOrder): number {
  if (field !== 'score') return compareNullable(sortValue[field](a), sortValue[field](b), order);
  const ka = scoreKey(a);
  const kb = scoreKey(b);
  const i = ka.findIndex((v, n) => v !== kb[n]);
  if (i === -1) return 0;
  return order === 'asc' ? ka[i] - kb[i] : kb[i] - ka[i];
}

export const deploymentLabel = (d: Token['deployments'][number]) => (d.network ? networkName(d.network) : d.cmc_network);

export function NetworkChips({ token }: { token: Token }) {
  return (
    <Box sx={{ display: 'flex', gap: 0.5, flexWrap: 'wrap', maxWidth: 180 }}>
      {token.deployments.map((d) => (
        <Tooltip key={`${d.cmc_network}:${d.address}`} title={DEPLOYMENT_STATUS[d.status]?.label ?? d.status} arrow>
          <Chip
            size="small"
            label={deploymentLabel(d)}
            color={d.status === 'indexed' ? 'primary' : 'default'}
            sx={{ opacity: d.status === 'indexed' ? 1 : 0.6 }}
          />
        </Tooltip>
      ))}
    </Box>
  );
}

interface VersionsTableProps {
  tokens: Token[];
  showUnderlying?: boolean; // one table for many underlyings
  paginate?: boolean;
}

// Comparison of tokenized versions, ranked by usefulness score.
export default function VersionsTable({ tokens, showUnderlying = false, paginate = false }: VersionsTableProps) {
  const navigate = useNavigate();
  const { sortField, sortOrder, onSort } = useSort<Field>('score');
  const [page, setPage] = useState(1);
  const [rowsPerPage, setRowsPerPage] = useState(25);

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
      <TableContainer component={Paper} sx={{ marginBottom: paginate ? 2 : 4 }}>
        <Table
          sx={{ minWidth: 650, ...(showUnderlying && { '& .MuiTableCell-root': { paddingX: 1.5 } }) }}
          aria-label="tokenized versions table"
        >
          <TableHead>
            <TableRow>
              <TableCell>
                {showUnderlying
                  ? header('ticker', 'Underlying', 'Underlying ticker and rank among its versions; click to sort by ticker')
                  : '#'}
              </TableCell>
              <TableCell>Version</TableCell>
              <TableCell>Issuer</TableCell>
              <TableCell>Networks</TableCell>
              <TableCell>{header('score', 'Score', 'Eligible positions; ties: protocols, networks, direct positions')}</TableCell>
              <TableCell>{header('raw', 'Raw')}</TableCell>
              <TableCell>{header('protocols', 'Protocols')}</TableCell>
              <TableCell>Direct / Indirect</TableCell>
              <TableCell>{header('apy', 'APY max / median', 'Reported APY over eligible positions: highest and median')}</TableCell>
              <TableCell>{header('mcap', 'MCap (CMC)')}</TableCell>
              <TableCell>Coverage</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {visible.length === 0 && (
              <TableRow>
                <TableCell colSpan={11} sx={{ color: 'text.secondary' }}>
                  {showUnderlying ? 'No tokenized versions match the filters' : 'CMC tracks no tokenized versions of this asset'}
                </TableCell>
              </TableRow>
            )}
            {visible.map((t) => (
              <TableRow key={t.id} hover onClick={() => navigate(`/tokens/${t.id}`)} sx={{ cursor: 'pointer' }}>
                <TableCell>
                  {showUnderlying ? (
                    <>
                      <Typography variant="body2" sx={{ fontWeight: 500 }}>
                        {t.ticker}
                      </Typography>
                      <Typography variant="body2" color="text.secondary">
                        #{t.rank_in_underlying}
                      </Typography>
                    </>
                  ) : (
                    t.rank_in_underlying
                  )}
                </TableCell>
                <TableCell>
                  <Tooltip title={t.name ?? ''} arrow>
                    <Typography variant="body2" component="div" sx={{ fontWeight: 500, whiteSpace: 'nowrap' }}>
                      {t.symbol} {t.kind === 'wrapper' && <Chip size="small" label="Wrapper" sx={{ marginLeft: 0.5 }} />}
                    </Typography>
                  </Tooltip>
                </TableCell>
                <TableCell>{t.issuer_name ?? '—'}</TableCell>
                <TableCell>
                  <NetworkChips token={t} />
                </TableCell>
                <TableCell>
                  <Typography variant="h5" component="span">
                    {t.usefulness_score}
                  </Typography>
                </TableCell>
                <TableCell>{t.counts.raw_confirmed_opportunities_count}</TableCell>
                <TableCell>{t.counts.eligible_protocols_count}</TableCell>
                <TableCell>
                  {t.counts.direct_eligible_count} / {t.counts.indirect_eligible_count}
                </TableCell>
                <TableCell>
                  {fmtPct(t.yield.max_reported_apy_percent)}
                  <Typography variant="body2" color="text.secondary">
                    {fmtPct(t.yield.median_reported_apy_percent)}
                  </Typography>
                </TableCell>
                <TableCell>{fmtUsd(t.market_cap_usd)}</TableCell>
                <TableCell>
                  <CoverageChip coverage={t.coverage} rawCount={t.counts.raw_confirmed_opportunities_count} />
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </TableContainer>
      {paginate && (
        <TablePager count={sorted.length} page={currentPage} rowsPerPage={rowsPerPage} onPage={setPage} onRowsPerPage={setRowsPerPage} />
      )}
    </>
  );
}
