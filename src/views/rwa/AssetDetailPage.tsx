import { useParams } from 'react-router-dom';
import Box from '@mui/material/Box';
import Chip from '@mui/material/Chip';
import Link from '@mui/material/Link';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableHead from '@mui/material/TableHead';
import TableRow from '@mui/material/TableRow';
import Typography from '@mui/material/Typography';

import { isReportedYield, relationRows, RwaIndex } from 'api/rwa';
import { Token } from 'types/rwa';
import { excludedReason, underlyingStatusLabel } from 'utils/rwa';
import { LocaleLink, useFormat, useT } from 'i18n';
import DataTableContainer from './components/DataTableContainer';
import DataGate, { NotFound } from './components/DataGate';
import DetailHeader from './components/StatGrid';
import MethodNote from './components/MethodNote';
import OpportunityTable from './components/OpportunityTable';
import { CoverageChip } from './components/StatusChip';
import TableCaption from './components/TableCaption';
import VersionsTable from './components/VersionsTable';
import { stickyColumn } from './components/tableSx';

function AssetDetailView({ index, ticker }: { index: RwaIndex; ticker: string }) {
  const t = useT();
  const f = useFormat();
  const u = index.underlyingByTicker.get(ticker.toUpperCase());
  if (!u) return <NotFound id="notFound.asset" values={{ ticker }} />;

  const tokens = u.token_ids.map((id) => index.tokenById.get(id)).filter((tk): tk is Token => !!tk);
  const excluded = u.excluded_token_ids.map((id) => index.tokenById.get(id)).filter((tk): tk is Token => !!tk);
  const yieldRows = relationRows(index, tokens).filter(isReportedYield);

  return (
    <Box sx={{ width: '100%' }}>
      <DetailHeader
        crumbs={[{ label: t('nav.assets'), to: '/assets' }]}
        title={u.ticker}
        logo={u.logo}
        subtitle={
          <>
            {`${u.cmc_name ?? u.name} · ${t('asset.isin')} ${u.isin ?? '—'} · ${u.asset_type.toUpperCase()}`}
            {u.website && (
              <>
                {' · '}
                <Link href={u.website} target="_blank" rel="noopener noreferrer" underline="always">
                  {u.website.replace(/^https?:\/\/(www\.)?/, '')}
                </Link>
              </>
            )}
          </>
        }
        chips={
          u.status === 'ok' ? <CoverageChip coverage={u.coverage} /> : <Chip size="small" label={underlyingStatusLabel(t, u.status)} />
        }
        stats={[
          { label: t('stat.tokens'), value: f.num(u.versions_count) },
          { label: t('stat.issuers'), value: f.num(u.issuers_count) },
          { label: t('stat.eligible'), value: f.num(u.eligible_opportunities_count), hint: t('stat.eligible.hintDistinct') },
          { label: t('stat.rawConfirmed'), value: f.num(u.raw_confirmed_opportunities_count) },
          { label: t('stat.protocols'), value: f.num(u.eligible_protocols_count), hint: u.eligible_platforms.join(', ') || undefined },
          { label: t('stat.networks'), value: f.num(u.eligible_networks_count) },
          { label: t('stat.tokenizedMcap'), value: f.usd(u.tokenized_market_cap_usd) },
          { label: t('stat.avgPrice'), value: f.usd(u.average_tokenized_price_usd) }
        ]}
      />
      <MethodNote data={index.data} />

      <Typography variant="h3" component="h2" gutterBottom sx={{ marginBottom: 2 }}>
        {t('asset.tokenizedAssets')}
      </Typography>
      <VersionsTable index={index} tokens={tokens} />

      <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'baseline', flexWrap: 'wrap', gap: 1, marginBottom: 2 }}>
        <Typography variant="h3" component="h2">
          {t('asset.highestApy')}
        </Typography>
        <Link component={LocaleLink} to={`/opportunities?underlying=${u.ticker}`} underline="always">
          {t('asset.allPositions', { ticker: u.ticker })}
        </Link>
      </Box>
      <OpportunityTable
        index={index}
        rows={yieldRows}
        showToken
        emptyText={t('asset.noYield')}
        caption={`${t('asset.highestApy')}: ${u.ticker}`}
      />

      {excluded.length > 0 && (
        <Box sx={{ marginTop: 4 }}>
          <Typography variant="h3" component="h2" gutterBottom sx={{ marginBottom: 2 }}>
            {t('asset.excluded')}
          </Typography>
          <DataTableContainer sx={stickyColumn(1)}>
            <Table sx={{ minWidth: 650 }} size="small">
              <TableCaption>{t('table.excluded')}</TableCaption>
              <TableHead>
                <TableRow>
                  <TableCell>{t('col.symbol')}</TableCell>
                  <TableCell>{t('col.name')}</TableCell>
                  <TableCell>{t('col.cmcIssuerLabel')}</TableCell>
                  <TableCell>{t('col.reason')}</TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {excluded.map((tk) => (
                  <TableRow key={tk.id}>
                    <TableCell component="th" scope="row" sx={{ fontWeight: 'inherit' }}>
                      <Link component={LocaleLink} to={`/tokens/${tk.id}`} underline="hover">
                        {tk.symbol ?? '—'}
                      </Link>
                    </TableCell>
                    <TableCell>{tk.name ?? '—'}</TableCell>
                    <TableCell>{tk.issuer_name ?? '—'}</TableCell>
                    <TableCell>{excludedReason(t, tk.excluded_reason)}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </DataTableContainer>
        </Box>
      )}
    </Box>
  );
}

export default function AssetDetailPage() {
  const { ticker = '' } = useParams<{ ticker: string }>();
  return <DataGate>{(index) => <AssetDetailView index={index} ticker={ticker} />}</DataGate>;
}
