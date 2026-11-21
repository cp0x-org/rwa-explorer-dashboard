import { Fragment } from 'react';
import Box from '@mui/material/Box';
import Link from '@mui/material/Link';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableHead from '@mui/material/TableHead';
import TableRow from '@mui/material/TableRow';
import Typography from '@mui/material/Typography';

import type { RwaIndex } from 'api/rwa';
import { LocaleLink, useT } from 'i18n';
import type { Token } from 'types/rwa';
import { COMPARE_FACTS, tokenRights } from 'utils/rights';
import DataTableContainer from './DataTableContainer';
import { RightsFactValue, RightsReview, SuggestCorrection } from './HolderRights';
import TableCaption from './TableCaption';
import { compareTokens } from './VersionsTable';
import { stickyColumn } from './tableSx';

export default function RightsCompareTable({ index, tokens }: { index: RwaIndex; tokens: Token[] }) {
  const t = useT();
  const reviewed = tokens.filter((token) => tokenRights(index, token)).sort((a, b) => compareTokens(a, b, 'score', 'desc') || a.id - b.id);
  const unreviewed = tokens.filter((token) => !tokenRights(index, token));
  return (
    <Box component="section" sx={{ marginBottom: 4, '& a': { display: 'inline-flex', minHeight: 24, alignItems: 'center' } }}>
      <Typography variant="h3" component="h2" gutterBottom>
        {t('rights.compare.title')}
      </Typography>
      {reviewed.length ? (
        <DataTableContainer sx={stickyColumn(1)}>
          <Table sx={{ minWidth: 1000 }}>
            <TableCaption>{t('rights.compare.caption')}</TableCaption>
            <TableHead>
              <TableRow>
                <TableCell>{t('col.token')}</TableCell>
                {COMPARE_FACTS.map((key) => (
                  <TableCell key={key}>{t(`rights.fact.${key}`)}</TableCell>
                ))}
                <TableCell>{t('rights.col.reviewed')}</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {reviewed.map((token) => {
                const profile = tokenRights(index, token)!;
                return (
                  <TableRow key={token.id}>
                    <TableCell component="th" scope="row">
                      <Link component={LocaleLink} to={`/issuers/${profile.issuer_id}#rights-${profile.id}`} underline="always">
                        {token.symbol ?? token.name ?? String(token.id)}
                      </Link>
                      <Typography variant="body2">{token.issuer_name ?? t('common.unknownIssuer')}</Typography>
                    </TableCell>
                    {COMPARE_FACTS.map((key) => (
                      <TableCell key={key} sx={{ verticalAlign: 'top' }}>
                        <RightsFactValue factKey={key} fact={profile.facts[key]} compact />
                      </TableCell>
                    ))}
                    <TableCell>
                      <RightsReview index={index} profile={profile} />
                    </TableCell>
                  </TableRow>
                );
              })}
            </TableBody>
          </Table>
        </DataTableContainer>
      ) : (
        <Box>
          <Typography>{t('rights.compare.none')}</Typography>
          <SuggestCorrection />
        </Box>
      )}
      {unreviewed.length > 0 && (
        <Typography component="p" id="rights-unreviewed" sx={{ marginTop: 2 }}>
          {t('rights.compare.unreviewed')}{' '}
          {unreviewed.map((token, i) => (
            <Fragment key={token.id}>
              {i > 0 && ', '}
              <Link component={LocaleLink} to={`/tokens/${token.id}`} underline="always">
                {token.symbol ?? token.name ?? String(token.id)} ({token.issuer_name ?? t('common.unknownIssuer')})
              </Link>
            </Fragment>
          ))}
        </Typography>
      )}
    </Box>
  );
}
