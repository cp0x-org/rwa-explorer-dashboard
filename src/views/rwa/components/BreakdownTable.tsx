import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableHead from '@mui/material/TableHead';
import TableRow from '@mui/material/TableRow';

import DataTableContainer from './DataTableContainer';
import TableCaption from './TableCaption';
import { Breakdown } from 'types/rwa';
import { useT } from 'i18n';

export default function BreakdownTable({
  title,
  data,
  label = (k) => k
}: {
  title: string;
  data: Breakdown;
  label?: (k: string) => string;
}) {
  const t = useT();
  const entries = Object.entries(data);
  return (
    <DataTableContainer>
      <Table size="small">
        <TableCaption>{t('table.breakdown', { title })}</TableCaption>
        <TableHead>
          <TableRow>
            <TableCell>{title}</TableCell>
            <TableCell align="right">{t('col.eligible')}</TableCell>
            <TableCell align="right">{t('col.raw')}</TableCell>
          </TableRow>
        </TableHead>
        <TableBody>
          {entries.length === 0 && (
            <TableRow>
              <TableCell colSpan={3} sx={{ color: 'text.secondary' }}>
                {t('breakdown.noPositions')}
              </TableCell>
            </TableRow>
          )}
          {entries.map(([key, v]) => (
            <TableRow key={key}>
              <TableCell component="th" scope="row" sx={{ fontWeight: 'inherit' }}>
                {label(key)}
              </TableCell>
              <TableCell align="right">{v.eligible}</TableCell>
              <TableCell align="right">{v.raw}</TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </DataTableContainer>
  );
}
