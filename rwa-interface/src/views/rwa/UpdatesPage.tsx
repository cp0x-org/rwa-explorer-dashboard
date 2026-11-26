import { useId, useState } from 'react';
import Box from '@mui/material/Box';
import FormControlLabel from '@mui/material/FormControlLabel';
import Switch from '@mui/material/Switch';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableHead from '@mui/material/TableHead';
import TableRow from '@mui/material/TableRow';
import Typography from '@mui/material/Typography';

import { useRuns } from 'api/runs';
import useHydrated from 'hooks/useHydrated';
import { runKind } from 'utils/rwa';
import { DASH, useFormat, useLocaleNavigate, useT } from 'i18n';
import DataGate from './components/DataGate';
import DataTableContainer from './components/DataTableContainer';
import RowLink from './components/RowLink';
import { RunStatusChip } from './components/StatusChip';
import TableCaption from './components/TableCaption';
import { stickyColumn } from './components/tableSx';

const RUNS_LIMIT = 50;
const PLACEHOLDER_HEIGHT = 360; // the runs table's room before it is there: the same on the server and in the first client render

function signed(value: number, format: (n: number) => string): string {
  return value > 0 ? `+${format(value)}` : format(value);
}

// The runs come from the API, not from the export. The server fetches them and renders the table (crawlers and agents read it
// without JavaScript); the client hydrates from the same list, embedded in the page as an SWR fallback. When the server could
// not get them, the server markup and the first client render are the same busy placeholder; the table replaces it afterwards.
function RunsTable({ showRebuilds }: { showRebuilds: boolean }) {
  const t = useT();
  const f = useFormat();
  const navigate = useLocaleNavigate();
  const hydrated = useHydrated();
  const { data, error } = useRuns(RUNS_LIMIT);

  if (!data && (!hydrated || !error)) {
    return (
      <Box aria-busy="true" sx={{ minHeight: PLACEHOLDER_HEIGHT }}>
        <Typography variant="body2" color="text.secondary" role="status" aria-live="polite">
          {t('updates.loading')}
        </Typography>
      </Box>
    );
  }
  if (!data) {
    return (
      <Box role="alert" sx={{ minHeight: PLACEHOLDER_HEIGHT }}>
        <Typography variant="body1" color="error">
          {t('updates.error')}
        </Typography>
      </Box>
    );
  }

  const runs = data.items.filter((r) => showRebuilds || r.mode !== 'rebuild');
  if (runs.length === 0) {
    const hidden = data.items.length;
    return (
      <Typography variant="body1" color="text.secondary" sx={{ minHeight: PLACEHOLDER_HEIGHT }}>
        {hidden > 0 ? t('updates.onlyRebuilds', { n: hidden }) : t('updates.empty')}
      </Typography>
    );
  }

  const num = (n: number) => f.num(n);
  return (
    <DataTableContainer sx={stickyColumn(1)}>
      <Table sx={{ minWidth: 900, '& .MuiTableCell-root': { paddingX: 1.5, fontVariantNumeric: 'tabular-nums' } }} size="small">
        <TableCaption>{t('updates.table')}</TableCaption>
        <TableHead>
          <TableRow>
            <TableCell>{t('updates.col.date')}</TableCell>
            <TableCell>{t('updates.col.status')}</TableCell>
            <TableCell>{t('updates.col.kind')}</TableCell>
            <TableCell align="right">{t('updates.col.duration')}</TableCell>
            <TableCell align="right">{t('updates.col.processed')}</TableCell>
            <TableCell align="right">{t('updates.col.cmc')}</TableCell>
            <TableCell align="right">{t('updates.col.portals')}</TableCell>
            <TableCell align="right">{t('updates.col.credits')}</TableCell>
            <TableCell align="right">{t('updates.col.newTokens')}</TableCell>
            <TableCell align="right">{t('updates.col.newPositions')}</TableCell>
            <TableCell align="right">{t('updates.col.eligibleDelta')}</TableCell>
          </TableRow>
        </TableHead>
        <TableBody>
          {runs.map((r) => (
            <TableRow key={r.id} hover onClick={() => navigate(`/updates/${r.id}`)} sx={{ cursor: 'pointer' }}>
              <TableCell component="th" scope="row" sx={{ fontWeight: 'inherit', whiteSpace: 'nowrap' }}>
                <RowLink to={`/updates/${r.id}`}>
                  <time dateTime={r.started_at}>{f.date(r.started_at)}</time>
                </RowLink>
              </TableCell>
              <TableCell>
                <RunStatusChip status={r.status} />
              </TableCell>
              <TableCell>{runKind(t, r)}</TableCell>
              <TableCell align="right">{f.duration(r.duration_seconds)}</TableCell>
              <TableCell align="right" sx={{ whiteSpace: 'nowrap' }}>
                {num(r.completed_count)} / {num(r.failed_count)}
              </TableCell>
              <TableCell align="right">{num(r.cmc_requests)}</TableCell>
              <TableCell align="right">{num(r.portals_requests)}</TableCell>
              <TableCell align="right">{num(r.cmc_credits)}</TableCell>
              <TableCell align="right">{r.changes ? num(r.changes.new_tokens_count) : DASH}</TableCell>
              <TableCell align="right">{r.changes ? num(r.changes.new_positions_count) : DASH}</TableCell>
              <TableCell align="right">{r.changes ? signed(r.changes.eligible_delta, num) : DASH}</TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </DataTableContainer>
  );
}

function UpdatesView() {
  const t = useT();
  const [showRebuilds, setShowRebuilds] = useState(false); // rebuilds collect nothing: hidden unless asked for
  const hintId = useId();

  return (
    <Box sx={{ width: '100%' }}>
      <Typography variant="h3" component="h1" gutterBottom>
        {t('updates.title')}
      </Typography>
      <Typography variant="body2" color="text.secondary" sx={{ marginBottom: 2, maxWidth: 900 }}>
        {t('updates.intro')}
      </Typography>
      <Box sx={{ marginBottom: 2 }}>
        <FormControlLabel
          control={
            <Switch
              checked={showRebuilds}
              onChange={(e) => setShowRebuilds(e.target.checked)}
              slotProps={{ input: { role: 'switch', 'aria-describedby': hintId } }}
            />
          }
          label={t('updates.showRebuilds')}
        />
        <Typography id={hintId} variant="caption" color="text.secondary" component="p" sx={{ marginTop: 0 }}>
          {t('updates.showRebuilds.hint')}
        </Typography>
      </Box>
      <RunsTable showRebuilds={showRebuilds} />
    </Box>
  );
}

export default function UpdatesPage() {
  return <DataGate>{() => <UpdatesView />}</DataGate>;
}
