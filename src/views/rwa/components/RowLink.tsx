import { ReactNode } from 'react';
import { Link as RouterLink } from 'react-router-dom';
import Link from '@mui/material/Link';

// The name cell of a clickable row as a real link: it works without JS, by keyboard and in a new tab, and crawlers follow
// it. The row keeps its onClick for clicks elsewhere; a click on the link stops here, so the row does not navigate again.
export default function RowLink({ to, children }: { to: string; children: ReactNode }) {
  return (
    <Link
      component={RouterLink}
      to={to}
      color="inherit"
      underline="none"
      onClick={(e) => e.stopPropagation()}
      sx={{ display: 'block', borderRadius: '4px' }}
    >
      {children}
    </Link>
  );
}
