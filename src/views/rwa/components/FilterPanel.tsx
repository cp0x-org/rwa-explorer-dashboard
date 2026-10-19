import { ReactNode, useId, useState } from 'react';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import { IconAdjustmentsHorizontal } from '@tabler/icons-react';

import { useT } from 'i18n';

// Filters stay open on wider screens; on phones they fold behind one button so the data starts on the first screen.
// Breakpoints are CSS, not a JS media query, so the prerendered page already has the right layout before React loads.
export default function FilterPanel({ active, children }: { active: number; children: ReactNode }) {
  const t = useT();
  const [open, setOpen] = useState(false);
  const id = useId();

  return (
    <>
      <Button
        variant="outlined"
        color="secondary"
        fullWidth
        startIcon={<IconAdjustmentsHorizontal size={18} aria-hidden="true" />}
        onClick={() => setOpen(!open)}
        aria-expanded={open}
        aria-controls={id}
        sx={{ display: { xs: 'flex', sm: 'none' }, justifyContent: 'flex-start', textTransform: 'none', marginBottom: 2 }}
      >
        {open ? t('filter.hide') : t('filter.show')}
        {active > 0 && ` · ${t('filter.active', { n: active })}`}
      </Button>
      <Box id={id} sx={{ display: { xs: open ? 'block' : 'none', sm: 'block' } }}>
        {children}
      </Box>
    </>
  );
}
