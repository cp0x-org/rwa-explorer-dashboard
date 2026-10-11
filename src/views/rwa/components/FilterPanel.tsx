import { ReactNode, useState } from 'react';
import { useTheme } from '@mui/material/styles';
import useMediaQuery from '@mui/material/useMediaQuery';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import Collapse from '@mui/material/Collapse';
import { IconAdjustmentsHorizontal } from '@tabler/icons-react';

// Filters stay open on wider screens; on phones they fold behind one button so the data starts on the first screen.
export default function FilterPanel({ active, children }: { active: number; children: ReactNode }) {
  const theme = useTheme();
  const phone = useMediaQuery(theme.breakpoints.down('sm'));
  const [open, setOpen] = useState(false);

  if (!phone) return <>{children}</>;

  return (
    <Box sx={{ marginBottom: 2 }}>
      <Button
        variant="outlined"
        color="secondary"
        fullWidth
        startIcon={<IconAdjustmentsHorizontal size={18} />}
        onClick={() => setOpen(!open)}
        aria-expanded={open}
        sx={{ justifyContent: 'flex-start', textTransform: 'none' }}
      >
        {open ? 'Hide filters' : 'Filters'}
        {active > 0 && ` · ${active} active`}
      </Button>
      <Collapse in={open}>
        <Box sx={{ paddingTop: 2 }}>{children}</Box>
      </Collapse>
    </Box>
  );
}
