import { ReactNode, useState } from 'react';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import { IconAdjustmentsHorizontal } from '@tabler/icons-react';

// Filters stay open on wider screens; on phones they fold behind one button so the data starts on the first screen.
// Breakpoints are CSS, not a JS media query, so the prerendered page already has the right layout before React loads.
export default function FilterPanel({ active, children }: { active: number; children: ReactNode }) {
  const [open, setOpen] = useState(false);

  return (
    <>
      <Button
        variant="outlined"
        color="secondary"
        fullWidth
        startIcon={<IconAdjustmentsHorizontal size={18} />}
        onClick={() => setOpen(!open)}
        aria-expanded={open}
        sx={{ display: { xs: 'flex', sm: 'none' }, justifyContent: 'flex-start', textTransform: 'none', marginBottom: 2 }}
      >
        {open ? 'Hide filters' : 'Filters'}
        {active > 0 && ` · ${active} active`}
      </Button>
      <Box sx={{ display: { xs: open ? 'block' : 'none', sm: 'block' } }}>{children}</Box>
    </>
  );
}
