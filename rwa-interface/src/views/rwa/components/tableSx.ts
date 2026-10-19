import { Theme } from '@mui/material/styles';

// Phones: keeps the column that names the row (asset, token, position) in view while a wide table scrolls sideways.
export const stickyColumn =
  (column = 1) =>
  (theme: Theme) => ({
    [theme.breakpoints.down('sm')]: {
      [`& .MuiTableCell-root:nth-of-type(${column})`]: {
        position: 'sticky' as const,
        left: 0,
        zIndex: 1,
        backgroundColor: theme.palette.background.paper,
        boxShadow: `inset -1px 0 0 ${theme.palette.divider}`
      }
    }
  });
