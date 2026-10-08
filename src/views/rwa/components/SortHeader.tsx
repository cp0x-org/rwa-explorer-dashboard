import { useState } from 'react';
import TableSortLabel from '@mui/material/TableSortLabel';
import Tooltip from '@mui/material/Tooltip';
import { UnfoldMore } from '@mui/icons-material';

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
}

export default function SortHeader<F extends string>({ field, label, sortField, sortOrder, onSort, hint }: SortHeaderProps<F>) {
  const active = sortField === field;
  return (
    <Tooltip title={hint ?? `Click to sort by ${label.toLowerCase()}`} arrow>
      <TableSortLabel
        active={active}
        direction={active ? sortOrder : 'desc'}
        onClick={() => onSort(field)}
        IconComponent={active ? undefined : UnfoldMore}
        sx={{ '.MuiTableSortLabel-icon': { opacity: 1, visibility: 'visible' } }}
      >
        {label}
      </TableSortLabel>
    </Tooltip>
  );
}
