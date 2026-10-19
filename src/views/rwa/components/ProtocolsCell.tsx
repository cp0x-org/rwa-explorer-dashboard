import Box from '@mui/material/Box';
import Typography from '@mui/material/Typography';

import VisuallyHidden from 'ui-component/VisuallyHidden';
import { useT } from 'i18n';
import HintTooltip from './HintTooltip';

const SHOWN = 3;

// "Protocols" cell: the count (the sortable value) with the protocol names under it. Long lists show the first
// names plus "+n more"; the full list is in the tooltip and in hidden text for assistive technology.
export default function ProtocolsCell({ count, names }: { count: number; names: string[] }) {
  const t = useT();
  const shown = names.slice(0, SHOWN).join(', ');
  const rest = names.length - SHOWN;
  const full = names.join(', ');
  return (
    <HintTooltip title={rest > 0 ? full : undefined}>
      <Box sx={{ minWidth: 0 }}>
        <Typography variant="body2" component="div">
          {count}
        </Typography>
        {names.length > 0 && (
          <Typography variant="caption" color="text.secondary" component="div" sx={{ lineHeight: 1.3, maxWidth: 190 }}>
            <span aria-hidden={rest > 0 ? 'true' : undefined}>
              {shown}
              {rest > 0 && ` ${t('protocols.more', { n: rest })}`}
            </span>
            {rest > 0 && <VisuallyHidden>{full}</VisuallyHidden>}
          </Typography>
        )}
      </Box>
    </HintTooltip>
  );
}
