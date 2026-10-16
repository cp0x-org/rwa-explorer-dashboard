import { useMemo, useState } from 'react';
import Box from '@mui/material/Box';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableHead from '@mui/material/TableHead';
import TableRow from '@mui/material/TableRow';
import Tooltip from '@mui/material/Tooltip';
import Typography from '@mui/material/Typography';

import { logoUrl, RwaIndex } from 'api/rwa';
import { TokenIcon } from 'components/TokenIcon';
import DataTableContainer from './DataTableContainer';
import { stickyColumn } from './tableSx';
import { RelationRow } from 'types/rwa';
import { categoryLabel, compareNullable, FLAG_LABELS, fmtPct, fmtUsd, networkName, num } from 'utils/rwa';
import Age from './Age';
import { EligibilityChip } from './StatusChip';
import SortHeader, { useSort } from './SortHeader';
import TablePager from './TablePager';
import OpportunityDialog from './OpportunityDialog';

type Field = 'apy' | 'liquidity' | 'updated';

const truncate = (s: string, max = 34) => (s.length > max ? `${s.slice(0, max)}…` : s);

export function linkLabel(row: RelationRow, index: RwaIndex): string {
  if (!row.relation) return 'Symbol match';
  if (row.relation.depth === 1) return 'Direct';
  const via = row.relation.path.slice(1, -1).map((k) => index.symbolByKey.get(k) ?? k);
  return `Via ${via.join(' → ')}`;
}

interface OpportunityTableProps {
  index: RwaIndex;
  rows: RelationRow[];
  showToken?: boolean;
  defaultSort?: Field;
  defaultRows?: number;
  emptyText?: string;
}

export default function OpportunityTable({
  index,
  rows,
  showToken = false,
  defaultSort = 'apy',
  defaultRows = 10,
  emptyText = 'No confirmed positions in current coverage'
}: OpportunityTableProps) {
  const { sortField, sortOrder, onSort } = useSort<Field>(defaultSort);
  const [page, setPage] = useState(1);
  const [rowsPerPage, setRowsPerPage] = useState(defaultRows);
  const [selected, setSelected] = useState<RelationRow | null>(null);

  const sorted = useMemo(() => {
    const value = (r: RelationRow) =>
      sortField === 'apy'
        ? num(r.opportunity.apy_percent)
        : sortField === 'liquidity'
          ? num(r.opportunity.liquidity_usd)
          : r.opportunity.updated_at;
    return [...rows].sort((a, b) => compareNullable(value(a), value(b), sortOrder) || a.opportunity.key.localeCompare(b.opportunity.key));
  }, [rows, sortField, sortOrder]);

  const visible = sorted.slice((page - 1) * rowsPerPage, page * rowsPerPage);
  const handleSort = (f: Field) => {
    onSort(f);
    setPage(1);
  };

  return (
    <Box>
      <DataTableContainer sx={[{ marginBottom: 2 }, stickyColumn(1)]}>
        {/* denser cells: with the token column the table must still fit the lg container so the header can stick */}
        <Table sx={{ minWidth: 650, '& .MuiTableCell-root': { paddingX: 1.5 } }} aria-label="positions table">
          <TableHead>
            <TableRow>
              <TableCell>Position</TableCell>
              {showToken && <TableCell>Token</TableCell>}
              <TableCell>Protocol</TableCell>
              <TableCell>Network</TableCell>
              <TableCell>Category</TableCell>
              <TableCell>Link</TableCell>
              <TableCell>
                <SortHeader field="liquidity" label="Liquidity" {...{ sortField, sortOrder }} onSort={handleSort} />
              </TableCell>
              <TableCell>
                <SortHeader
                  field="apy"
                  label="Reported APY"
                  hint="Highest reported APY first. APY of one position, not an expected return"
                  {...{ sortField, sortOrder }}
                  onSort={handleSort}
                />
              </TableCell>
              <TableCell>Status</TableCell>
              <TableCell>
                <SortHeader field="updated" label="Updated" {...{ sortField, sortOrder }} onSort={handleSort} />
              </TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {visible.length === 0 && (
              <TableRow>
                <TableCell colSpan={showToken ? 10 : 9} sx={{ color: 'text.secondary' }}>
                  {emptyText}
                </TableCell>
              </TableRow>
            )}
            {visible.map((row) => {
              const o = row.opportunity;
              return (
                <TableRow key={`${row.token.id}:${o.key}`} hover onClick={() => setSelected(row)} sx={{ cursor: 'pointer' }}>
                  <TableCell>
                    <Tooltip title={o.name ?? ''} arrow>
                      <Box>
                        <Typography variant="body2" sx={{ fontWeight: 500 }}>
                          {truncate(o.symbol ?? o.key, 26)}
                        </Typography>
                        <Typography variant="body2" color="text.secondary">
                          {truncate(o.name ?? '')}
                        </Typography>
                      </Box>
                    </Tooltip>
                  </TableCell>
                  {showToken && (
                    <TableCell>
                      <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                        <TokenIcon
                          symbol={row.token.symbol ?? '?'}
                          src={logoUrl(row.token.logo)}
                          avatarProps={{ sx: { width: 24, height: 24, fontSize: 10 } }}
                        />
                        <Box>
                          <Typography variant="body2">{row.token.symbol}</Typography>
                          <Typography variant="body2" color="text.secondary">
                            {row.token.ticker} · {row.token.issuer_name}
                          </Typography>
                        </Box>
                      </Box>
                    </TableCell>
                  )}
                  <TableCell>{o.platform_name}</TableCell>
                  <TableCell>{networkName(o.network)}</TableCell>
                  <TableCell>{categoryLabel(o.category)}</TableCell>
                  <TableCell>
                    <Tooltip title={linkLabel(row, index)} arrow>
                      <span>
                        {!row.relation ? 'Symbol match' : row.relation.depth === 1 ? 'Direct' : `Indirect (${row.relation.depth})`}
                      </span>
                    </Tooltip>
                  </TableCell>
                  <TableCell>{fmtUsd(o.liquidity_usd)}</TableCell>
                  <TableCell>
                    {o.flags.includes('apy_anomaly') ? (
                      <Tooltip title={FLAG_LABELS.apy_anomaly} arrow>
                        <Typography variant="body2" component="span" color="warning.main">
                          {fmtPct(o.apy_percent)} ⚠
                        </Typography>
                      </Tooltip>
                    ) : (
                      fmtPct(o.apy_percent)
                    )}
                  </TableCell>
                  <TableCell>
                    <EligibilityChip eligibility={row.relation ? o.eligibility : 'unconfirmed'} />
                  </TableCell>
                  <TableCell>
                    <Age iso={o.updated_at} />
                  </TableCell>
                </TableRow>
              );
            })}
          </TableBody>
        </Table>
      </DataTableContainer>
      <TablePager count={rows.length} page={page} rowsPerPage={rowsPerPage} onPage={setPage} onRowsPerPage={setRowsPerPage} />
      <OpportunityDialog index={index} row={selected} onClose={() => setSelected(null)} />
    </Box>
  );
}
