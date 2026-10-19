import { ReactElement } from 'react';
import Tooltip from '@mui/material/Tooltip';

// A tooltip that explains its child without renaming it: MUI's default puts the title into aria-label, which would
// replace the visible label as the accessible name. Here the title describes the child (aria-describedby when shown).
// Hints that must also reach screen readers without hover are rendered as visually hidden text by the caller.
export default function HintTooltip({ title, children }: { title?: string; children: ReactElement<any> }) {
  if (!title) return children;
  return (
    <Tooltip title={title} describeChild arrow enterTouchDelay={0}>
      {children}
    </Tooltip>
  );
}
