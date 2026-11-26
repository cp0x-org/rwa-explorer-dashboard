import Box from '@mui/material/Box';
import Link from '@mui/material/Link';
import Typography from '@mui/material/Typography';

import type { RwaIndex } from 'api/rwa';
import { LocaleLink, useT } from 'i18n';
import type { Token } from 'types/rwa';
import { tokenRights } from 'utils/rights';
import { KeyFacts, RightsReview, ToneLegend } from './HolderRights';

export default function RightsSummary({ index, token }: { index: RwaIndex; token: Token }) {
  const t = useT();
  const profile = tokenRights(index, token);
  const issuer = token.issuer_id ? index.issuerById.get(token.issuer_id) : undefined;
  const otherProfiles = issuer?.rights_profile_ids?.some((id) => index.rightsById.has(id));
  return (
    <Box component="section" sx={{ marginY: 3 }}>
      <Typography variant="h3" component="h2" gutterBottom>
        {t('rights.title')}
      </Typography>
      {profile ? (
        <>
          <KeyFacts profile={profile} />
          <ToneLegend sx={{ marginTop: 1.5 }} />
          <RightsReview index={index} profile={profile} />
          <Link component={LocaleLink} to={`/issuers/${profile.issuer_id}#rights-${profile.id}`} underline="always">
            {t('rights.fullProfile')}
          </Link>
        </>
      ) : (
        <>
          <Typography>{t('rights.notReviewed.token')}</Typography>
          {token.kind === 'wrapper' && <Typography sx={{ marginTop: 1 }}>{t('rights.wrapper')}</Typography>}
          {issuer && otherProfiles && (
            <Link component={LocaleLink} to={`/issuers/${issuer.id}#rights`} underline="always">
              {t('rights.reviewedOther', { issuer: issuer.name ?? t('common.unknownIssuer') })}
            </Link>
          )}
        </>
      )}
    </Box>
  );
}
