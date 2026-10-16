import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableHead from '@mui/material/TableHead';
import TableRow from '@mui/material/TableRow';

import DataTableContainer from './DataTableContainer';
import { Breakdown } from 'types/rwa';

export default function BreakdownTable({
  title,
  data,
  label = (k) => k
}: {
  title: string;
  data: Breakdown;
  label?: (k: string) => string;
}) {
  const entries = Object.entries(data);
  return (
    <DataTableContainer>
      <Table size="small" aria-label={`${title} breakdown`}>
        <TableHead>
          <TableRow>
            <TableCell>{title}</TableCell>
            <TableCell align="right">Eligible</TableCell>
            <TableCell align="right">Raw</TableCell>
          </TableRow>
        </TableHead>
        <TableBody>
          {entries.length === 0 && (
            <TableRow>
              <TableCell colSpan={3} sx={{ color: 'text.secondary' }}>
                No confirmed positions
              </TableCell>
            </TableRow>
          )}
          {entries.map(([key, v]) => (
            <TableRow key={key}>
              <TableCell>{label(key)}</TableCell>
              <TableCell align="right">{v.eligible}</TableCell>
              <TableCell align="right">{v.raw}</TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </DataTableContainer>
  );
}
