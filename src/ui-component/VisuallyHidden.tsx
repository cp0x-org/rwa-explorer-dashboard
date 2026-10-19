import { ElementType, ReactNode } from 'react';
import Box from '@mui/material/Box';

// Text for assistive technology only (table captions, hints, chart summaries): in the accessibility tree and in the
// prerendered HTML, not on screen.
export const visuallyHidden = {
  position: 'absolute',
  width: '1px', // strings: in MUI's sx a bare 1 would mean 100%
  height: '1px',
  padding: 0,
  margin: '-1px',
  overflow: 'hidden',
  clip: 'rect(0 0 0 0)',
  whiteSpace: 'nowrap',
  border: 0
} as const;

export default function VisuallyHidden({ children, component = 'span' }: { children: ReactNode; component?: ElementType }) {
  return (
    <Box component={component} sx={visuallyHidden}>
      {children}
    </Box>
  );
}
