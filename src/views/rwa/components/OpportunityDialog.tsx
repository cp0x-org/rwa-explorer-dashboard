import { ReactNode } from 'react';
import Box from '@mui/material/Box';
import Chip from '@mui/material/Chip';
import Dialog from '@mui/material/Dialog';
import DialogContent from '@mui/material/DialogContent';
import DialogTitle from '@mui/material/DialogTitle';
import Divider from '@mui/material/Divider';
import IconButton from '@mui/material/IconButton';
import Typography from '@mui/material/Typography';
import CloseIcon from '@mui/icons-material/Close';

import { RwaIndex } from 'api/rwa';
import { CopyableAddress } from 'components/CopyableAddress';
import { RelationRow } from 'types/rwa';
import { categoryLabel, DASH, ELIGIBILITY, FLAG_LABELS, fmtDate, fmtPct, fmtUsd, networkName } from 'utils/rwa';
import { EligibilityChip } from './StatusChip';

const LIQUIDITY_SEMANTICS: Record<string, string> = {
  pool_tvl: 'pool TVL',
  vault_tvl: 'deposits (TVL)',
  token_metric: 'token metric, not market depth',
  unknown: 'semantics unknown'
};

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <Box sx={{ display: 'flex', justifyContent: 'space-between', gap: 2, paddingY: 0.75 }}>
      <Typography variant="body2" color="text.secondary" sx={{ flexShrink: 0 }}>
        {label}
      </Typography>
      <Box sx={{ textAlign: 'right', minWidth: 0, wordBreak: 'break-word' }}>
        <Typography variant="body2" component="div">
          {children}
        </Typography>
      </Box>
    </Box>
  );
}

export default function OpportunityDialog({ index, row, onClose }: { index: RwaIndex; row: RelationRow | null; onClose: () => void }) {
  if (!row) return null;
  const o = row.opportunity;
  const label = (key: string) => index.symbolByKey.get(key) ?? key;
  const status = row.relation ? o.eligibility : 'unconfirmed';

  return (
    <Dialog open onClose={onClose} fullWidth maxWidth="sm">
      <DialogTitle sx={{ paddingRight: 6 }}>
        <Typography variant="h4" component="span" sx={{ display: 'block', wordBreak: 'break-word' }}>
          {o.symbol ?? o.key}
        </Typography>
        <Typography variant="body2" color="text.secondary">
          {o.name}
        </Typography>
        <IconButton aria-label="close" onClick={onClose} sx={{ position: 'absolute', right: 8, top: 8 }}>
          <CloseIcon />
        </IconButton>
      </DialogTitle>
      <DialogContent dividers>
        <Field label="Status">
          <EligibilityChip eligibility={status} /> <span style={{ marginLeft: 8 }}>{ELIGIBILITY[status]?.hint}</span>
        </Field>
        <Field label="Token">
          {row.token.symbol} ({row.token.ticker}, {row.token.issuer_name ?? 'issuer unknown'})
        </Field>
        <Field label="Link to token">
          {row.relation
            ? row.relation.path.map(label).join(' → ') + (row.relation.depth > 1 ? ` (indirect, depth ${row.relation.depth})` : ' (direct)')
            : 'Symbol match only: composition does not contain the token address'}
        </Field>
        <Divider sx={{ marginY: 1 }} />
        <Field label="Protocol">{o.platform_name}</Field>
        <Field label="Network">{networkName(o.network)}</Field>
        <Field label="Category">{categoryLabel(o.category)}</Field>
        <Field label="Composition">{o.components.length ? o.components.map((c) => c.symbol ?? c.key).join(' + ') : DASH}</Field>
        <Field label="Position id">
          <CopyableAddress address={o.address} />
        </Field>
        <Field label="Kind">
          {o.position_kind === 'pool_id' ? 'Pool id (not an ERC-20 share)' : 'Contract'}
          {o.is_portable === false ? ', not zappable via Portals' : ''}
        </Field>
        <Divider sx={{ marginY: 1 }} />
        <Field label="Liquidity">
          {fmtUsd(o.liquidity_usd)}{' '}
          <Typography component="span" variant="body2" color="text.secondary">
            ({LIQUIDITY_SEMANTICS[o.liquidity_semantics]})
          </Typography>
        </Field>
        <Field label="Reported APY">{fmtPct(o.apy_percent)}</Field>
        <Field label="Base / reward APY">
          {fmtPct(o.base_apy_percent)} / {fmtPct(o.reward_apy_percent)}
        </Field>
        {o.borrow_apy_percent !== null && <Field label="Borrow APY (cost, not yield)">{fmtPct(o.borrow_apy_percent)}</Field>}
        <Field label="Volume 1d / 7d">
          {fmtUsd(o.volume_usd_1d)} / {fmtUsd(o.volume_usd_7d)}
        </Field>
        <Field label="Source updated">{fmtDate(o.updated_at)}</Field>
        {o.flags.length > 0 && (
          <Field label="Flags">
            <Box sx={{ display: 'flex', flexDirection: 'column', alignItems: 'flex-end', gap: 0.5 }}>
              {o.flags.map((f) => (
                <Chip key={f} size="small" label={FLAG_LABELS[f] ?? f} />
              ))}
            </Box>
          </Field>
        )}
        <Typography variant="caption" color="text.secondary" sx={{ display: 'block', marginTop: 1 }}>
          Source: Portals token index. Reported APY is a snapshot of this position, not an expected return.
        </Typography>
      </DialogContent>
    </Dialog>
  );
}
