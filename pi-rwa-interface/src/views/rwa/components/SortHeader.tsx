import { useState } from 'react';
import TableCell from '@mui/material/TableCell';
import TableSortLabel from '@mui/material/TableSortLabel';
import { UnfoldMore } from '@mui/icons-material';

import VisuallyHidden from 'ui-component/VisuallyHidden';
import { useT } from 'i18n';
import HintTooltip from './HintTooltip';

export type SortOrder = 'asc' | 'desc';

export function useSort<F extends string>(defaultField: F, defaultOrder: SortOrder = 'desc') {
  const [sortField, setSortField] = useState<F>(defaultField);
  const [sortOrder, setSortOrder] = useState<SortOrder>(defaultOrder);
  const onSort = (field: F) => {
    // numeric columns: first click sorts descending
    setSortOrder(sortField === field && sortOrder === 'desc' ? 'asc' : 'desc');
    setSortField(field);
  };
  return { sortField, sortOrder, onSort };
}

interface SortHeaderProps<F extends string> {
  field: F;
  label: string;
  sortField: F;
  sortOrder: SortOrder;
  onSort: (field: F) => void;
  hint?: string;
  align?: 'left' | 'right';
}

// A sortable column header cell: <th aria-sort> with the sort button inside; the hint describes the column (tooltip
// and visually hidden text), the visible label stays the accessible name.
export default function SortHeader<F extends string>({ field, label, sortField, sortOrder, onSort, hint, align }: SortHeaderProps<F>) {
  const t = useT();
  const active = sortField === field;
  return (
    <TableCell align={align} aria-sort={active ? (sortOrder === 'asc' ? 'ascending' : 'descending') : undefined}>
      <HintTooltip title={hint ?? t('sort.hint', { label })}>
        <TableSortLabel
          active={active}
          direction={active ? sortOrder : 'desc'}
          onClick={() => onSort(field)}
          IconComponent={active ? undefined : UnfoldMore}
          sx={{ '.MuiTableSortLabel-icon': { opacity: 1, visibility: 'visible' } }}
        >
          {label}
          {hint && <VisuallyHidden>. {hint}</VisuallyHidden>}
        </TableSortLabel>
      </HintTooltip>
    </TableCell>
  );
}

// A plain column header cell with an optional hint (tooltip + visually hidden text).
export function HeaderCell({ label, hint, align }: { label: string; hint?: string; align?: 'left' | 'right' }) {
  return (
    <TableCell align={align}>
      <HintTooltip title={hint}>
        <span>{label}</span>
      </HintTooltip>
      {hint && <VisuallyHidden>. {hint}</VisuallyHidden>}
    </TableCell>
  );
}
