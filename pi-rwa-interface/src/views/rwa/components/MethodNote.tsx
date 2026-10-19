import { useId, useState } from 'react';
import Box from '@mui/material/Box';
import Collapse from '@mui/material/Collapse';
import Link from '@mui/material/Link';
import Typography from '@mui/material/Typography';

import useHydrated from 'hooks/useHydrated';
import { RwaExport } from 'types/rwa';
import { useFormat, useT } from 'i18n';

// Update time and counting method, always next to the data. The method text is a disclosure (a real button, in the
// page for crawlers and screen readers), not a hover-only tooltip.
export default function MethodNote({ data }: { data: RwaExport }) {
  const hydrated = useHydrated(); // the age needs the viewer's clock, so the prerendered HTML has only the date
  const t = useT();
  const f = useFormat();
  const [open, setOpen] = useState(false);
  const id = useId();
  const { oldest_fetched_at: oldest, newest_fetched_at: newest } = data.summary;
  const s = data.settings;
  const method = t('method.text', {
    depth: s.max_graph_depth,
    liquidity: f.usd(s.min_eligible_liquidity_usd),
    hours: s.max_position_age_hours
  });

  return (
    <Box sx={{ marginBottom: 2 }}>
      <Typography variant="body2" color="text.secondary">
        {t('method.collected')} {oldest && oldest !== newest ? `${f.date(oldest)} – ` : ''}
        <time dateTime={newest ?? undefined}>{f.date(newest)}</time>
        {hydrated && ` (${f.age(newest)})`} · {t('method.sources')} · {t('method.coverage')} ·{' '}
        <Link
          component="button"
          type="button"
          variant="body2"
          aria-expanded={open}
          aria-controls={id}
          onClick={() => setOpen(!open)}
          sx={{ verticalAlign: 'baseline', textDecoration: 'underline dotted', color: 'inherit' }}
        >
          {t('method.how')}
        </Link>
      </Typography>
      <Collapse in={open}>
        <Typography id={id} variant="body2" color="text.secondary" sx={{ marginTop: 1, maxWidth: 900 }}>
          {method}
        </Typography>
      </Collapse>
    </Box>
  );
}
