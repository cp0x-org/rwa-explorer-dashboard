import { useMemo, useState } from 'react';
import Box from '@mui/material/Box';
import ButtonBase from '@mui/material/ButtonBase';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableHead from '@mui/material/TableHead';
import TableRow from '@mui/material/TableRow';
import Tooltip from '@mui/material/Tooltip';
import Typography from '@mui/material/Typography';

import { logoUrl, RwaIndex } from 'api/rwa';
import { TokenIcon } from 'components/TokenIcon';
import VisuallyHidden from 'ui-component/VisuallyHidden';
import DataTableContainer from './DataTableContainer';
import TableCaption from './TableCaption';
import { stickyColumn } from './tableSx';
import { RelationRow } from 'types/rwa';
import { categoryLabel, compareNullable, flagLabel, networkName, num } from 'utils/rwa';
import { T, useFormat, useT } from 'i18n';
import Age from './Age';
import { EligibilityChip } from './StatusChip';
import SortHeader, { useSort } from './SortHeader';
import TablePager from './TablePager';
import OpportunityDialog from './OpportunityDialog';

type Field = 'apy' | 'liquidity' | 'updated';

const truncate = (s: string, max = 34) => (s.length > max ? `${s.slice(0, max)}…` : s);

export function linkLabel(t: T, row: RelationRow, index: RwaIndex): string {
  if (!row.relation) return t('positions.symbolMatch');
  if (row.relation.depth === 1) return t('positions.direct');
  const via = row.relation.path.slice(1, -1).map((k) => index.symbolByKey.get(k) ?? k);
  return t('positions.via', { path: via.join(' → ') });
}

interface OpportunityTableProps {
  index: RwaIndex;
  rows: RelationRow[];
  showToken?: boolean;
  defaultSort?: Field;
  defaultRows?: number;
  emptyText?: string;
  caption?: string;
}

export default function OpportunityTable({
  index,
  rows,
  showToken = false,
  defaultSort = 'apy',
  defaultRows = 10,
  emptyText,
  caption
}: OpportunityTableProps) {
  const t = useT();
  const f = useFormat();
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
  const handleSort = (fl: Field) => {
    onSort(fl);
    setPage(1);
  };

  return (
    <Box>
      <DataTableContainer sx={[{ marginBottom: 2 }, stickyColumn(1)]}>
        {/* denser cells: with the token column the table must still fit the lg container so the header can stick */}
        <Table sx={{ minWidth: 650, '& .MuiTableCell-root': { paddingX: 1.5 } }}>
          <TableCaption>{caption ?? t('table.positions')}</TableCaption>
          <TableHead>
            <TableRow>
              <TableCell>{t('col.position')}</TableCell>
              {showToken && <TableCell>{t('col.token')}</TableCell>}
              <TableCell>{t('col.protocol')}</TableCell>
              <TableCell>{t('col.network')}</TableCell>
              <TableCell>{t('col.category')}</TableCell>
              <TableCell>{t('col.link')}</TableCell>
              <SortHeader field="liquidity" label={t('col.liquidity')} {...{ sortField, sortOrder }} onSort={handleSort} />
              <SortHeader
                field="apy"
                label={t('col.reportedApy')}
                hint={t('col.reportedApy.hint')}
                {...{ sortField, sortOrder }}
                onSort={handleSort}
              />
              <TableCell>{t('col.status')}</TableCell>
              <SortHeader field="updated" label={t('col.updated')} {...{ sortField, sortOrder }} onSort={handleSort} />
            </TableRow>
          </TableHead>
          <TableBody>
            {visible.length === 0 && (
              <TableRow>
                <TableCell colSpan={showToken ? 10 : 9} sx={{ color: 'text.secondary' }}>
                  {emptyText ?? t('positions.empty')}
                </TableCell>
              </TableRow>
            )}
            {visible.map((row) => {
              const o = row.opportunity;
              const name = o.symbol ?? o.key;
              return (
                <TableRow key={`${row.token.id}:${o.key}`} hover onClick={() => setSelected(row)} sx={{ cursor: 'pointer' }}>
                  <TableCell component="th" scope="row" sx={{ fontWeight: 'inherit' }}>
                    {/* the name opens the details: a real button for keyboard and screen-reader users; the row click stays for the mouse */}
                    <ButtonBase
                      onClick={(e) => {
                        e.stopPropagation();
                        setSelected(row);
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
                      <Typography variant="body2" sx={{ fontWeight: 500 }}>
                        {truncate(name, 26)}
                      </Typography>
                      <Typography variant="body2" color="text.secondary">
                        {truncate(o.name ?? '')}
                      </Typography>
                      <VisuallyHidden>. {t('common.details')}</VisuallyHidden>
                    </ButtonBase>
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
                  <TableCell>{categoryLabel(t, o.category)}</TableCell>
                  <TableCell>
                    <Tooltip title={linkLabel(t, row, index)} describeChild arrow>
                      <span>
                        {!row.relation
                          ? t('positions.symbolMatch')
                          : row.relation.depth === 1
                            ? t('positions.direct')
                            : t('positions.indirect', { depth: row.relation.depth })}
                      </span>
                    </Tooltip>
                    {row.relation && row.relation.depth > 1 && <VisuallyHidden>: {linkLabel(t, row, index)}</VisuallyHidden>}
                  </TableCell>
                  <TableCell>{f.usd(o.liquidity_usd)}</TableCell>
                  <TableCell>
                    {o.flags.includes('apy_anomaly') ? (
                      <Tooltip title={flagLabel(t, 'apy_anomaly')} describeChild arrow>
                        <Typography variant="body2" component="span" color="warning.main">
                          {f.pct(o.apy_percent)} <span aria-hidden="true">⚠</span>
                          <VisuallyHidden>. {flagLabel(t, 'apy_anomaly')}</VisuallyHidden>
                        </Typography>
                      </Tooltip>
                    ) : (
                      f.pct(o.apy_percent)
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
