import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import IconButton from '@mui/material/IconButton';
import Link from '@mui/material/Link';
import Typography from '@mui/material/Typography';
import OpenInNewIcon from '@mui/icons-material/OpenInNew';

import useExpiry from 'hooks/useExpiry';
import { useFormat, useT } from 'i18n';
import { Opportunity, Referral } from 'types/rwa';
import VisuallyHidden from 'ui-component/VisuallyHidden';
import { dealPageUrl, referralIsFresh, referralRecheckDelay, safeUrl } from 'utils/links';

export function OutboundLink({ url, label, iconOnly = false }: { url: unknown; label: string; iconOnly?: boolean }) {
  const t = useT();
  const href = safeUrl(url);
  if (!href) return null;
  const newTab = <VisuallyHidden> {t('link.newTab')}</VisuallyHidden>;
  if (iconOnly) {
    return (
      <IconButton component="a" href={href} target="_blank" rel="noopener noreferrer" onClick={(event) => event.stopPropagation()}>
        <OpenInNewIcon fontSize="small" />
        <VisuallyHidden>{label}</VisuallyHidden>
        {newTab}
      </IconButton>
    );
  }
  return (
    <Button
      component="a"
      href={href}
      target="_blank"
      rel="noopener noreferrer"
      variant="outlined"
      endIcon={<OpenInNewIcon />}
      onClick={(event) => event.stopPropagation()}
      sx={{ maxWidth: '100%', overflowWrap: 'anywhere', textAlign: 'center' }}
    >
      {label}
      {newTab}
    </Button>
  );
}

export function PositionLink({ opportunity, iconOnly = false }: { opportunity: Opportunity; iconOnly?: boolean }) {
  const t = useT();
  return (
    <OutboundLink
      url={opportunity.link?.url}
      label={t(opportunity.link?.kind === 'position' ? 'link.openPosition' : 'link.openProtocol', { protocol: opportunity.platform_name })}
      iconOnly={iconOnly}
    />
  );
}

export function ReferralLink({
  referral,
  maxAgeDays,
  direct
}: {
  referral?: Referral | null;
  maxAgeDays?: number | null;
  direct: boolean;
}) {
  const t = useT();
  const f = useFormat();
  const now = useExpiry(referral?.as_of, maxAgeDays, referralRecheckDelay);
  const href = safeUrl(referral?.url);
  if (!referral || !href || now === null || !referralIsFresh(referral.as_of, maxAgeDays, now)) return null;
  const detailsUrl = dealPageUrl(referral.page_url);
  return (
    <Box sx={{ minWidth: 0, width: '100%', overflowWrap: 'anywhere' }}>
      <Link href={href} target="_blank" rel="sponsored noopener noreferrer" underline="always">
        {referral.bonus ? t('link.referral', { bonus: referral.bonus }) : t('link.referral.plain')}
        <OpenInNewIcon sx={{ fontSize: 'inherit', marginLeft: 0.5 }} />
        <VisuallyHidden> {t('link.newTab')}</VisuallyHidden>
      </Link>
      <Typography variant="caption" component="p" color="text.secondary" sx={{ margin: 0 }}>
        {t(direct ? 'link.referral.note' : 'link.referral.note.only')}
      </Typography>
      <Typography variant="caption" component="p" color="text.secondary" sx={{ margin: 0 }}>
        {detailsUrl && (
          <>
            <Link href={detailsUrl} target="_blank" rel="noopener noreferrer" underline="always">
              {t('link.referral.details')}
              <VisuallyHidden> {t('link.newTab')}</VisuallyHidden>
            </Link>{' '}
          </>
        )}
        {t('link.referral.asOf', { date: f.date(referral.as_of) })}
      </Typography>
    </Box>
  );
}
