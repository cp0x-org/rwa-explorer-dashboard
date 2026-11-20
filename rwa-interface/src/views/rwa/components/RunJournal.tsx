import { memo, useEffect, useMemo, useRef, useState } from 'react';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import Chip from '@mui/material/Chip';
import FormControlLabel from '@mui/material/FormControlLabel';
import Link from '@mui/material/Link';
import Switch from '@mui/material/Switch';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableContainer from '@mui/material/TableContainer';
import TableHead from '@mui/material/TableHead';
import TableRow from '@mui/material/TableRow';
import ToggleButton from '@mui/material/ToggleButton';
import ToggleButtonGroup from '@mui/material/ToggleButtonGroup';
import Paper from '@mui/material/Paper';
import Typography from '@mui/material/Typography';
import { alpha } from '@mui/material/styles';

import { useSWRConfig } from 'swr';

import { ApiError, RwaIndex } from 'api/rwa';
import { runKey, useRunEvents } from 'api/runs';
import { Json, RunEvent, RunEventLevel } from 'types/api';
import { LocaleLink, MessageId, useT } from 'i18n';
import TableCaption from './TableCaption';

const LEVELS: RunEventLevel[] = ['debug', 'info', 'warn', 'error']; // the first one is "all"
const LEVEL_COLOR: Record<RunEventLevel, 'default' | 'info' | 'warning' | 'error'> = {
  debug: 'default',
  info: 'info',
  warn: 'warning',
  error: 'error'
};
const PLACEHOLDER_HEIGHT = 240;
const MAX_VALUE_CHARS = 200; // a value of `attrs` is clamped to this, the full text is in its title
const MAX_LIST_ITEMS = 5; // an array shows this many items and "+N"
const MESSAGE_LINES = 3;

// HH:MM:SS in UTC, whatever the viewer's time zone.
export const utcTime = (iso: string): string => {
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? iso : d.toISOString().slice(11, 19);
};

// MM-DD in UTC: the prefix of the first row of a new day, when a run crosses midnight.
export const utcDay = (iso: string): string => {
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? '' : d.toISOString().slice(5, 10);
};

const scalar = (v: Json): string => (typeof v === 'string' ? v : JSON.stringify(v));

// An attribute value as text: strings as they are, arrays `a, b, …` (first 5 and "+N"), objects as JSON; clamped to 200
// characters. `full` is the unclamped text (the title of the cell) when anything was cut.
export function attrText(v: Json): { text: string; full: string | null } {
  let full: string;
  let text: string;
  if (Array.isArray(v)) {
    full = v.map(scalar).join(', ');
    text = v.slice(0, MAX_LIST_ITEMS).map(scalar).join(', ') + (v.length > MAX_LIST_ITEMS ? ` +${v.length - MAX_LIST_ITEMS}` : '');
  } else {
    full = text = scalar(v);
  }
  if (text.length > MAX_VALUE_CHARS) text = `${text.slice(0, MAX_VALUE_CHARS - 1)}…`;
  return { text, full: text === full ? null : full };
}

// The asset a line is about: attr `underlying` holds its ticker (an ISIN is resolved to the ticker too).
const assetTicker = (v: Json, index: RwaIndex): string | null => {
  if (typeof v !== 'string') return null;
  if (index.underlyingByTicker.has(v)) return v;
  return index.underlyingById.get(v)?.ticker ?? null;
};

function Attrs({ attrs, index }: { attrs: RunEvent['attrs']; index: RwaIndex }) {
  const entries = Object.entries(attrs ?? {});
  if (entries.length === 0) return null;
  return (
    <Box component="span" sx={{ fontFamily: 'monospace', fontSize: '12px', wordBreak: 'break-word' }}>
      {entries.map(([k, v], i) => {
        const ticker = k === 'underlying' ? assetTicker(v, index) : null;
        const { text, full } = attrText(v);
        return (
          <span key={k} title={full ?? undefined}>
            {i > 0 ? ' ' : ''}
            {k}=
            {ticker ? (
              <Link component={LocaleLink} to={`/assets/${encodeURIComponent(ticker)}`} underline="hover">
                {text}
              </Link>
            ) : (
              text
            )}
          </span>
        );
      })}
    </Box>
  );
}

// The message, clamped to 3 lines; "more" appears only when something is hidden.
function Message({ text }: { text: string }) {
  const t = useT();
  const [open, setOpen] = useState(false);
  const [clipped, setClipped] = useState(false);
  const el = useRef<HTMLSpanElement>(null);
  const maybeLong = text.length > 100 || text.includes('\n'); // short one-liners are not measured
  useEffect(() => {
    const node = el.current;
    if (maybeLong && node && !open) setClipped(node.scrollHeight > node.clientHeight + 1);
  }, [maybeLong, open, text]);
  return (
    <>
      <Box
        component="span"
        ref={el}
        sx={
          open
            ? { display: 'block', whiteSpace: 'pre-wrap', wordBreak: 'break-word' }
            : {
                display: '-webkit-box',
                WebkitBoxOrient: 'vertical',
                WebkitLineClamp: MESSAGE_LINES,
                overflow: 'hidden',
                whiteSpace: 'pre-wrap',
                wordBreak: 'break-word'
              }
        }
      >
        {text}
      </Box>
      {(clipped || open) && (
        <Link component="button" type="button" variant="body2" underline="hover" aria-expanded={open} onClick={() => setOpen(!open)}>
          {open ? t('update.journal.less') : t('update.journal.more')}
        </Link>
      )}
    </>
  );
}

// One row of the table. Memoised: a poll appends rows, the ones already shown keep their props and are not rendered again.
const EventRow = memo(function EventRow({ e, index, day }: { e: RunEvent; index: RwaIndex; day: string | null }) {
  const t = useT();
  return (
    <TableRow
      data-level={e.level}
      sx={e.level === 'error' ? { backgroundColor: (theme) => alpha(theme.palette.error.main, 0.06) } : undefined}
    >
      <TableCell sx={{ whiteSpace: 'nowrap', fontVariantNumeric: 'tabular-nums' }}>
        {day && <span>{day} </span>}
        {utcTime(e.ts)}
      </TableCell>
      <TableCell>
        <Chip size="small" variant="outlined" color={LEVEL_COLOR[e.level]} label={t(`update.journal.levelName.${e.level}` as MessageId)} />
      </TableCell>
      <TableCell sx={{ whiteSpace: 'nowrap' }}>{e.step}</TableCell>
      <TableCell sx={{ minWidth: 200 }}>
        <Message text={e.message} />
      </TableCell>
      <TableCell>
        <Attrs attrs={e.attrs} index={index} />
      </TableCell>
    </TableRow>
  );
});

export default function RunJournal({ runId, index }: { runId: number; index: RwaIndex }) {
  const t = useT();
  const { mutate } = useSWRConfig();
  const [level, setLevel] = useState<RunEventLevel>('debug');
  const [step, setStep] = useState<string | null>(null);
  const [follow, setFollow] = useState(false);
  const { data, error, isValidating, loadMore, stale } = useRunEvents(runId, { level });
  const scroller = useRef<HTMLDivElement>(null);

  const items = useMemo(() => data?.items ?? [], [data]);
  const steps = useMemo(() => [...new Set(items.map((e) => e.step))].sort(), [items]);
  const activeStep = step !== null && steps.includes(step) ? step : null;
  const rows = useMemo(() => (activeStep ? items.filter((e) => e.step === activeStep) : items), [items, activeStep]);
  const open = data?.journalOpen === true;
  const finishing = open && data?.runStatus !== 'running'; // the job has finished its work and is still writing the last events

  // The run detail is cached and polled by the page: when the journal closes, fetch it once more (final status, counts).
  const wasOpen = useRef(false);
  useEffect(() => {
    if (wasOpen.current && data && !data.journalOpen) void mutate(runKey(runId));
    wasOpen.current = open;
  }, [open, data, mutate, runId]);

  // Follow: keep the newest row in view inside the table's own scroll box (the page itself never scrolls).
  useEffect(() => {
    const el = scroller.current;
    if (follow && el) el.scrollTop = el.scrollHeight;
  }, [follow, rows.length]);

  if (!data) {
    return (
      <Box aria-busy={!error} sx={{ minHeight: PLACEHOLDER_HEIGHT }}>
        {error ? (
          <Typography variant="body1" color="error" role="alert">
            {t('update.journal.error')}
          </Typography>
        ) : (
          <Typography variant="body2" color="text.secondary" role="status" aria-live="polite">
            {t('update.journal.loading')}
          </Typography>
        )}
      </Box>
    );
  }

  const firstDay = items.length ? utcDay(items[0].ts) : '';
  const paused = error ? (error instanceof ApiError ? `HTTP ${error.status}` : t('update.journal.offline')) : null;

  return (
    <Box>
      <Box sx={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: 2, marginBottom: 1.5 }}>
        <ToggleButtonGroup
          exclusive
          size="small"
          value={level}
          onChange={(_e, value: RunEventLevel | null) => value !== null && setLevel(value)}
          aria-label={t('update.journal.level')}
        >
          {LEVELS.map((l) => (
            <ToggleButton key={l} value={l} sx={{ textTransform: 'none', px: 1.5 }}>
              {t(`update.journal.level.${l}` as MessageId)}
            </ToggleButton>
          ))}
        </ToggleButtonGroup>
        {open && (
          <FormControlLabel
            control={<Switch checked={follow} onChange={(e) => setFollow(e.target.checked)} slotProps={{ input: { role: 'switch' } }} />}
            label={t('update.journal.follow')}
          />
        )}
      </Box>

      {steps.length > 1 && (
        <Box role="group" aria-label={t('update.journal.steps')} sx={{ display: 'flex', flexWrap: 'wrap', gap: 0.75, marginBottom: 1.5 }}>
          {steps.map((s) => (
            <Chip
              key={s}
              size="small"
              label={s}
              clickable
              aria-pressed={activeStep === s}
              color={activeStep === s ? 'primary' : 'default'}
              variant={activeStep === s ? 'filled' : 'outlined'}
              onClick={() => setStep(activeStep === s ? null : s)}
            />
          ))}
        </Box>
      )}

      {/* The live region holds only the state (and errors), which change rarely; the counts, which change on every poll, are plain text next to it. */}
      <Box sx={{ display: 'flex', flexWrap: 'wrap', columnGap: 1.5, marginBottom: 1 }}>
        <Typography variant="body2" color="text.secondary" role="status">
          {open ? (finishing ? t('update.journal.finishing') : t('update.journal.running')) : t('update.journal.finished')}
          {paused && ` ${t('update.journal.paused', { reason: paused })}`}
        </Typography>
        <Typography variant="body2" color="text.secondary">
          {activeStep !== null
            ? t('update.journal.loadedFiltered', { n: items.length, m: rows.length })
            : t('update.journal.loaded', { n: items.length })}
        </Typography>
      </Box>

      {stale ? (
        <Box aria-busy="true" sx={{ minHeight: PLACEHOLDER_HEIGHT }}>
          <Typography variant="body2" color="text.secondary">
            {t('update.journal.loading')}
          </Typography>
        </Box>
      ) : rows.length === 0 ? (
        <Typography variant="body2" color="text.secondary" sx={{ minHeight: 48 }}>
          {t('update.journal.empty')}
        </Typography>
      ) : (
        <TableContainer
          component={Paper}
          ref={scroller}
          tabIndex={0}
          role="region"
          aria-label={t('update.journal.region')}
          sx={{ maxHeight: 480, overflow: 'auto' }}
        >
          <Table size="small" stickyHeader sx={{ minWidth: 720 }}>
            <TableCaption>{t('update.journal.table')}</TableCaption>
            <TableHead>
              <TableRow>
                <TableCell>{t('update.journal.col.time')}</TableCell>
                <TableCell>{t('update.journal.col.level')}</TableCell>
                <TableCell>{t('update.journal.col.step')}</TableCell>
                <TableCell>{t('update.journal.col.message')}</TableCell>
                <TableCell>{t('update.journal.col.attrs')}</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {rows.map((e, i) => {
                const day = utcDay(e.ts);
                const before = i === 0 ? firstDay : utcDay(rows[i - 1].ts);
                return <EventRow key={e.id} e={e} index={index} day={day && day !== before ? day : null} />;
              })}
            </TableBody>
          </Table>
        </TableContainer>
      )}

      {data.nextAfter !== null && !stale && (
        <Button variant="outlined" size="small" onClick={() => void loadMore()} disabled={isValidating} sx={{ marginTop: 1.5 }}>
          {t('update.journal.loadMore')}
        </Button>
      )}
    </Box>
  );
}
