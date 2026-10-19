import { ReactNode, useId } from 'react';
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
import { categoryLabel, DASH, eligibilityChip, flagLabel, networkName } from 'utils/rwa';
import { MessageId, useFormat, useT } from 'i18n';
import { EligibilityChip } from './StatusChip';

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <Box sx={{ display: 'flex', justifyContent: 'space-between', gap: 2, paddingY: 0.75 }}>
      <Typography variant="body2" color="text.secondary" component="dt" sx={{ flexShrink: 0 }}>
        {label}
      </Typography>
      <Box component="dd" sx={{ textAlign: 'right', minWidth: 0, wordBreak: 'break-word', margin: 0 }}>
        <Typography variant="body2" component="div">
          {children}
        </Typography>
      </Box>
    </Box>
  );
}

export default function OpportunityDialog({ index, row, onClose }: { index: RwaIndex; row: RelationRow | null; onClose: () => void }) {
  const t = useT();
  const f = useFormat();
  const titleId = useId();
  if (!row) return null;
  const o = row.opportunity;
  const label = (key: string) => index.symbolByKey.get(key) ?? key;
  const status = row.relation ? o.eligibility : 'unconfirmed';

  return (
    <Dialog open onClose={onClose} fullWidth maxWidth="sm" aria-labelledby={titleId}>
      <DialogTitle sx={{ paddingRight: 6 }} component="div">
        <Typography variant="h4" component="h2" id={titleId} sx={{ display: 'block', wordBreak: 'break-word' }}>
          {o.symbol ?? o.key}
        </Typography>
        <Typography variant="body2" color="text.secondary">
          {o.name}
        </Typography>
        <IconButton aria-label={t('dialog.close')} onClick={onClose} sx={{ position: 'absolute', right: 8, top: 8 }}>
          <CloseIcon />
        </IconButton>
      </DialogTitle>
      <DialogContent dividers>
        <Box component="dl" sx={{ margin: 0 }}>
          <Field label={t('dialog.status')}>
            <EligibilityChip eligibility={status} /> <span style={{ marginLeft: 8 }}>{eligibilityChip(t, status).hint}</span>
          </Field>
          <Field label={t('dialog.token')}>
            {row.token.symbol} ({row.token.ticker}, {row.token.issuer_name ?? t('dialog.issuerUnknown')})
          </Field>
          <Field label={t('dialog.linkToToken')}>
            {row.relation
              ? row.relation.path.map(label).join(' → ') +
                ' ' +
                (row.relation.depth > 1 ? t('dialog.indirect', { depth: row.relation.depth }) : t('dialog.direct'))
              : t('dialog.symbolMatchOnly')}
          </Field>
          <Divider sx={{ marginY: 1 }} />
          <Field label={t('dialog.protocol')}>{o.platform_name}</Field>
          <Field label={t('dialog.network')}>{networkName(o.network)}</Field>
          <Field label={t('dialog.category')}>{categoryLabel(t, o.category)}</Field>
          <Field label={t('dialog.composition')}>
            {o.components.length ? o.components.map((c) => c.symbol ?? c.key).join(' + ') : DASH}
          </Field>
          <Field label={t('dialog.positionId')}>
            <CopyableAddress address={o.address} />
          </Field>
          <Field label={t('dialog.kind')}>
            {o.position_kind === 'pool_id' ? t('dialog.kind.poolId') : t('dialog.kind.contract')}
            {o.is_portable === false ? t('dialog.kind.notPortable') : ''}
          </Field>
          <Divider sx={{ marginY: 1 }} />
          <Field label={t('dialog.liquidity')}>
            {f.usd(o.liquidity_usd)}{' '}
            <Typography component="span" variant="body2" color="text.secondary">
              ({t(`dialog.liquidity.${o.liquidity_semantics}` as MessageId)})
            </Typography>
          </Field>
          <Field label={t('dialog.reportedApy')}>{f.pct(o.apy_percent)}</Field>
          <Field label={t('dialog.baseReward')}>
            {f.pct(o.base_apy_percent)} / {f.pct(o.reward_apy_percent)}
          </Field>
          {o.borrow_apy_percent !== null && <Field label={t('dialog.borrow')}>{f.pct(o.borrow_apy_percent)}</Field>}
          <Field label={t('dialog.volume')}>
            {f.usd(o.volume_usd_1d)} / {f.usd(o.volume_usd_7d)}
          </Field>
          <Field label={t('dialog.sourceUpdated')}>{f.date(o.updated_at)}</Field>
          {o.flags.length > 0 && (
            <Field label={t('dialog.flags')}>
              <Box sx={{ display: 'flex', flexDirection: 'column', alignItems: 'flex-end', gap: 0.5 }}>
                {o.flags.map((fl) => (
                  <Chip key={fl} size="small" label={flagLabel(t, fl)} />
                ))}
              </Box>
            </Field>
          )}
        </Box>
        <Typography variant="caption" color="text.secondary" sx={{ display: 'block', marginTop: 1 }}>
          {t('dialog.source')}
        </Typography>
      </DialogContent>
    </Dialog>
  );
}
