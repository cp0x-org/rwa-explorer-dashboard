import { ReactNode, useState } from 'react';
import { useParams } from 'react-router-dom';
import Box from '@mui/material/Box';
import Chip from '@mui/material/Chip';
import FormControlLabel from '@mui/material/FormControlLabel';
import Link from '@mui/material/Link';
import Switch from '@mui/material/Switch';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableHead from '@mui/material/TableHead';
import TableRow from '@mui/material/TableRow';
import Typography from '@mui/material/Typography';

import { ApiError, RwaIndex } from 'api/rwa';
import { useRun } from 'api/runs';
import useHydrated from 'hooks/useHydrated';
import { RunChangesDetail, RunDetail } from 'types/api';
import { networkName, runKind } from 'utils/rwa';
import { DASH, LocaleLink, MessageId, T, useFormat, useT } from 'i18n';
import DataGate, { NotFound } from './components/DataGate';
import DataTableContainer from './components/DataTableContainer';
import { StatGrid } from './components/StatGrid';
import { RunStatusChip } from './components/StatusChip';
import TableCaption from './components/TableCaption';

const SHELL_HEIGHT = 320;

function Crumbs({ id }: { id: string }) {
  const t = useT();
  return (
    <Box component="nav" aria-label={t('common.breadcrumb')} sx={{ marginBottom: 1 }}>
      <Box
        component="ol"
        sx={{ display: 'flex', flexWrap: 'wrap', gap: 0.75, listStyle: 'none', margin: 0, padding: 0, color: 'grey.500' }}
      >
        <Typography component="li" variant="h4" sx={{ fontSize: '16px', fontWeight: 600 }}>
          <Link component={LocaleLink} to="/updates" color="inherit" underline="hover">
            {t('update.crumb')}
          </Link>
        </Typography>
        <Typography component="li" aria-hidden="true" sx={{ fontSize: '16px' }}>
          ›
        </Typography>
        <Typography component="li" variant="h4" aria-current="page" sx={{ fontSize: '16px', fontWeight: 600 }}>
          #{id}
        </Typography>
      </Box>
    </Box>
  );
}

// What the backend writes into `details` (compute/changes.go): signed changes of eligible positions per network and per
// protocol, CMC ids of reclassified tokens, and `first_build` on the very first build. Anything else is not shown.
interface ChangeDetails {
  eligible_by_network?: Record<string, number>;
  eligible_by_platform?: Record<string, number>;
  reclassified?: number[];
  first_build?: boolean;
}

const asRecord = (v: unknown): Record<string, number> =>
  v && typeof v === 'object' && !Array.isArray(v)
    ? Object.fromEntries(Object.entries(v).filter(([, n]) => typeof n === 'number') as [string, number][])
    : {};

function DetailsLists({ index, changes, tokenNode }: { index: RwaIndex; changes: RunChangesDetail; tokenNode: (id: number) => ReactNode }) {
  const t = useT();
  const f = useFormat();
  const details = (changes.details ?? {}) as ChangeDetails;
  const signed = (n: number) => (n > 0 ? `+${f.num(n)}` : f.num(n));
  const groups: { title: MessageId; rows: [string, number][] }[] = [
    {
      title: 'update.details.byNetwork' as MessageId,
      rows: Object.entries(asRecord(details.eligible_by_network)).map(([k, n]): [string, number] => [networkName(k), n])
    },
    {
      title: 'update.details.byPlatform' as MessageId,
      rows: Object.entries(asRecord(details.eligible_by_platform)).map(([k, n]): [string, number] => [
        index.platformNameBySlug.get(k) ?? k,
        n
      ])
    }
  ].filter((g) => g.rows.length > 0);
  const reclassified = Array.isArray(details.reclassified) ? details.reclassified.filter((id) => typeof id === 'number') : [];

  return (
    <>
      {details.first_build === true && (
        <Typography variant="body2" color="text.secondary" sx={{ marginTop: 2 }}>
          {t('update.details.firstBuild')}
        </Typography>
      )}
      {groups.length > 0 && (
        <Box sx={{ display: 'flex', gap: 4, flexWrap: 'wrap', marginTop: 2 }}>
          {groups.map((g) => (
            <Box key={g.title} sx={{ minWidth: 220 }}>
              <Typography variant="h5" component="h3" gutterBottom>
                {t(g.title)}
              </Typography>
              <Box component="ul" sx={{ listStyle: 'none', margin: 0, padding: 0 }}>
                {g.rows.map(([name, n]) => (
                  <Typography key={name} component="li" variant="body2" sx={{ paddingY: 0.25, fontVariantNumeric: 'tabular-nums' }}>
                    <strong>{name}</strong>: {signed(n)}
                  </Typography>
                ))}
              </Box>
            </Box>
          ))}
        </Box>
      )}
      {reclassified.length > 0 && (
        <LinkList title={t('update.details.reclassified')} items={reclassified.map((id) => ({ key: String(id), node: tokenNode(id) }))} />
      )}
    </>
  );
}

function LinkList({ title, items }: { title: string; items: { key: string; node: ReactNode }[] }) {
  const t = useT();
  return (
    <Box sx={{ marginTop: 2 }}>
      <Typography variant="h5" component="h3" gutterBottom>
        {title}
      </Typography>
      {items.length === 0 ? (
        <Typography variant="body2" color="text.secondary">
          {t('update.changes.nothing')}
        </Typography>
      ) : (
        <Box component="ul" sx={{ display: 'flex', flexWrap: 'wrap', gap: 1, listStyle: 'none', margin: 0, padding: 0 }}>
          {items.map((i) => (
            <Typography key={i.key} component="li" variant="body2">
              {i.node}
            </Typography>
          ))}
        </Box>
      )}
    </Box>
  );
}

function Changes({ index, run }: { index: RwaIndex; run: RunDetail }) {
  const t = useT();
  const f = useFormat();
  const c = run.changes;
  if (!c) {
    return (
      <Typography variant="body2" color="text.secondary">
        {t('update.changes.none')}
      </Typography>
    );
  }
  const tokenNode = (id: number) => {
    const tk = index.tokenById.get(id);
    if (!tk) return <span>{t('update.changes.tokenUnknown', { id })}</span>;
    return (
      <Link component={LocaleLink} to={`/tokens/${id}`} underline="hover">
        {tk.symbol ?? t('update.changes.tokenUnknown', { id })}
        {tk.name ? ` · ${tk.name}` : ''}
      </Link>
    );
  };
  const delta = c.eligible_delta;
  return (
    <>
      <StatGrid
        stats={[
          { label: t('update.changes.newPositions'), value: f.num(c.new_positions_count) },
          { label: t('update.changes.gonePositions'), value: f.num(c.gone_positions_count) },
          { label: t('update.changes.eligibleDelta'), value: delta > 0 ? `+${f.num(delta)}` : f.num(delta) }
        ]}
      />
      <LinkList title={t('update.changes.newTokens')} items={c.new_tokens.map((id) => ({ key: String(id), node: tokenNode(id) }))} />
      <LinkList
        title={t('update.changes.removedTokens')}
        items={c.removed_tokens.map((id) => ({ key: String(id), node: tokenNode(id) }))}
      />
      <LinkList
        title={t('update.changes.newIssuers')}
        items={c.new_issuers.map((id) => {
          const issuer = index.issuerById.get(id);
          return {
            key: id,
            node: issuer ? (
              <Link component={LocaleLink} to={`/issuers/${encodeURIComponent(id)}`} underline="hover">
                {issuer.name ?? id}
              </Link>
            ) : (
              <span>{id}</span>
            )
          };
        })}
      />
      <DetailsLists index={index} changes={c} tokenNode={tokenNode} />
    </>
  );
}

function underlyingStatus(t: T, status: string): { label: string; color: 'success' | 'error' | 'default' } {
  if (status === 'ok') return { label: t('update.underlying.ok'), color: 'success' };
  if (status === 'failed') return { label: t('update.underlying.failed'), color: 'error' };
  if (status === 'skipped') return { label: t('update.underlying.skipped'), color: 'default' };
  return { label: status, color: 'default' };
}

function Underlyings({ index, run }: { index: RwaIndex; run: RunDetail }) {
  const t = useT();
  const f = useFormat();
  const [onlyIssues, setOnlyIssues] = useState(false);
  const rows = onlyIssues ? run.underlyings.filter((u) => u.status !== 'ok') : run.underlyings;

  return (
    <>
      <FormControlLabel
        sx={{ marginBottom: 1 }}
        control={
          <Switch checked={onlyIssues} onChange={(e) => setOnlyIssues(e.target.checked)} slotProps={{ input: { role: 'switch' } }} />
        }
        label={t('update.underlyings.onlyIssues')}
      />
      {rows.length === 0 ? (
        <Typography variant="body2" color="text.secondary">
          {run.underlyings.length === 0 ? t('update.underlyings.none') : t('update.underlyings.noIssues')}
        </Typography>
      ) : (
        <DataTableContainer>
          <Table size="small" sx={{ minWidth: 600 }}>
            <TableCaption>{t('update.underlyings.table')}</TableCaption>
            <TableHead>
              <TableRow>
                <TableCell>{t('update.col.underlying')}</TableCell>
                <TableCell>{t('update.col.status')}</TableCell>
                <TableCell>{t('update.col.error')}</TableCell>
                <TableCell align="right">{t('update.col.duration')}</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {rows.map((u) => {
                const status = underlyingStatus(t, u.status);
                const known = index.underlyingByTicker.has(u.ticker);
                return (
                  <TableRow key={u.underlying_id}>
                    <TableCell component="th" scope="row" sx={{ fontWeight: 'inherit' }}>
                      {known ? (
                        <Link component={LocaleLink} to={`/assets/${encodeURIComponent(u.ticker)}`} underline="hover">
                          {u.ticker}
                        </Link>
                      ) : (
                        u.ticker
                      )}
                    </TableCell>
                    <TableCell>
                      <Chip
                        size="small"
                        variant={status.color === 'error' ? 'outlined' : 'filled'}
                        color={status.color}
                        label={status.label}
                      />
                    </TableCell>
                    <TableCell sx={{ wordBreak: 'break-word' }}>{u.error ?? DASH}</TableCell>
                    <TableCell align="right" sx={{ fontVariantNumeric: 'tabular-nums' }}>
                      {f.duration(u.duration_seconds)}
                    </TableCell>
                  </TableRow>
                );
              })}
            </TableBody>
          </Table>
        </DataTableContainer>
      )}
    </>
  );
}

function RunView({ index, run, id }: { index: RwaIndex; run: RunDetail; id: string }) {
  const t = useT();
  const f = useFormat();
  const num = (n: number) => f.num(n);
  return (
    <>
      <Typography variant="body2" color="text.secondary" component="div" sx={{ marginBottom: 2 }}>
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, flexWrap: 'wrap' }}>
          <RunStatusChip status={run.status} />
          <span>
            {t('update.runKey', { key: run.run_key })}
            {' · '}
            {t('update.startedFinished', {
              started: f.date(run.started_at),
              finished: run.finished_at ? f.date(run.finished_at) : t('update.running')
            })}
          </span>
        </Box>
      </Typography>
      {run.error && (
        <Typography variant="body1" color="error" role="alert" sx={{ marginBottom: 2 }}>
          {t('update.runError', { message: run.error })}
        </Typography>
      )}
      <Box sx={{ marginBottom: 4 }}>
        <StatGrid
          stats={[
            { label: t('update.stat.mode'), value: runKind(t, run) },
            { label: t('update.stat.duration'), value: f.duration(run.duration_seconds) },
            { label: t('update.stat.selected'), value: num(run.selected_count) },
            { label: t('update.stat.completed'), value: num(run.completed_count) },
            { label: t('update.stat.failed'), value: num(run.failed_count) },
            { label: t('update.stat.cmc'), value: num(run.cmc_requests) },
            { label: t('update.stat.portals'), value: num(run.portals_requests) },
            { label: t('update.stat.credits'), value: num(run.cmc_credits) }
          ]}
        />
      </Box>

      <Typography variant="h3" component="h2" gutterBottom sx={{ marginBottom: 2 }}>
        {t('update.warnings')} ({num(run.warnings.length)})
      </Typography>
      {run.warnings.length === 0 ? (
        <Typography variant="body2" color="text.secondary" sx={{ marginBottom: 4 }}>
          {t('update.warnings.none')}
        </Typography>
      ) : (
        <Box component="ul" sx={{ marginTop: 0, marginBottom: 4, paddingLeft: 3 }}>
          {run.warnings.map((w, i) => (
            <Typography key={i} component="li" variant="body2" sx={{ paddingY: 0.25, wordBreak: 'break-word' }}>
              {w}
            </Typography>
          ))}
        </Box>
      )}

      <Typography variant="h3" component="h2" gutterBottom sx={{ marginBottom: 2 }}>
        {t('update.changes')}
      </Typography>
      <Box sx={{ marginBottom: 4 }}>
        <Changes index={index} run={run} />
      </Box>

      <Typography variant="h3" component="h2" gutterBottom sx={{ marginBottom: 2 }}>
        {t('update.underlyings')}
      </Typography>
      <Underlyings key={id} index={index} run={run} />
    </>
  );
}

// One run. The server renders the page shell (heading, breadcrumb, a busy placeholder) for any numeric id, with status
// 200 and `noindex`: it does not know the runs (the export has none), the browser asks the API after hydration and the
// shell and the first client render are the same. A run the API does not know ends in the "not found" state in the
// browser; such pages are never in the sitemap, and crawlers are told not to index any /updates/:id.
function UpdateDetailView({ index, id }: { index: RwaIndex; id: string }) {
  const t = useT();
  const hydrated = useHydrated();
  const valid = /^\d+$/.test(id);
  const { data, error } = useRun(valid ? Number(id) : undefined);

  if (!valid) return <NotFound id="update.notFound" values={{ id }} />;
  if (hydrated && !data && error instanceof ApiError && error.status === 404) return <NotFound id="update.notFound" values={{ id }} />;

  return (
    <Box sx={{ width: '100%' }}>
      <Crumbs id={id} />
      <Typography variant="h3" component="h1" gutterBottom>
        {t('update.heading', { id })}
      </Typography>
      {!hydrated || !data ? (
        <Box aria-busy={!error} sx={{ minHeight: SHELL_HEIGHT }}>
          {error ? (
            <Typography variant="body1" color="error" role="alert">
              {t('update.error')}
            </Typography>
          ) : (
            <Typography variant="body2" color="text.secondary" role="status" aria-live="polite">
              {t('update.loading')}
            </Typography>
          )}
        </Box>
      ) : (
        <RunView index={index} run={data} id={id} />
      )}
    </Box>
  );
}

export default function UpdateDetailPage() {
  const { runId = '' } = useParams<{ runId: string }>();
  return <DataGate>{(index) => <UpdateDetailView key={runId} index={index} id={runId} />}</DataGate>;
}
