import { useId } from 'react';
import Box from '@mui/material/Box';
import FormControl from '@mui/material/FormControl';
import InputLabel from '@mui/material/InputLabel';
import MenuItem from '@mui/material/MenuItem';
import Pagination from '@mui/material/Pagination';
import Select from '@mui/material/Select';

import { useT } from 'i18n';

interface TablePagerProps {
  count: number;
  page: number;
  rowsPerPage: number;
  onPage: (page: number) => void;
  onRowsPerPage: (rows: number) => void;
}

// Rows selector + pagination, as on the template's vault list.
export default function TablePager({ count, page, rowsPerPage, onPage, onRowsPerPage }: TablePagerProps) {
  const t = useT();
  const labelId = useId();
  const pageCount = Math.ceil(count / rowsPerPage);
  if (count <= 10) return null;
  return (
    <Box sx={{ display: 'flex', justifyContent: 'center', alignItems: 'center', gap: 2, flexWrap: 'wrap', marginBottom: 2 }}>
      <FormControl variant="outlined" size="small">
        <InputLabel id={labelId}>{t('pager.rows')}</InputLabel>
        <Select
          labelId={labelId}
          value={rowsPerPage}
          onChange={(e) => {
            onRowsPerPage(Number(e.target.value));
            onPage(1);
          }}
          label={t('pager.rows')}
          sx={{ minWidth: 80 }}
        >
          {[10, 25, 50, 100].map((n) => (
            <MenuItem key={n} value={n}>
              {n}
            </MenuItem>
          ))}
        </Select>
      </FormControl>
      <Pagination
        count={pageCount}
        page={Math.min(page, pageCount)}
        onChange={(_, p) => onPage(p)}
        color="primary"
        aria-label={t('pager.label')}
        getItemAriaLabel={(type, p, selected) => {
          if (type === 'page') return selected ? t('pager.current', { page: p }) : t('pager.page', { page: p });
          if (type === 'next') return t('pager.next');
          if (type === 'previous') return t('pager.previous');
          if (type === 'first') return t('pager.first');
          return t('pager.last');
        }}
      />
    </Box>
  );
}
