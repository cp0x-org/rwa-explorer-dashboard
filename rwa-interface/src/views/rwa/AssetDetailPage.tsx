import { SyntheticEvent, useEffect, useState } from 'react';
import { useParams } from 'react-router-dom';
import Box from '@mui/material/Box';
import Chip from '@mui/material/Chip';
import Link from '@mui/material/Link';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableHead from '@mui/material/TableHead';
import TableRow from '@mui/material/TableRow';
import Tab from '@mui/material/Tab';
import Tabs from '@mui/material/Tabs';
import Typography from '@mui/material/Typography';

import { RwaIndex } from 'api/rwa';
import { Token } from 'types/rwa';
import { excludedReason, underlyingStatusLabel } from 'utils/rwa';
import { LocaleLink, useFormat, useT } from 'i18n';
import DataTableContainer from './components/DataTableContainer';
import DataGate, { NotFound } from './components/DataGate';
import DetailHeader from './components/StatGrid';
import HistoryChart from './components/HistoryChart';
import MethodNote from './components/MethodNote';
import { CoverageChip } from './components/StatusChip';
import TableCaption from './components/TableCaption';
import VersionsTable from './components/VersionsTable';
import RightsCompareTable from './components/RightsCompareTable';
import { stickyColumn } from './components/tableSx';

const ASSET_HISTORY_METRICS = [
  'tokenized_market_cap_usd',
  'eligible_opportunities_count',
  'tokens_in_defi_count',
  'versions_count',
  'issuers_count'
];

// The tokens table and the rights comparison share one slot; #rights opens the second tab, so it can be linked.
type TabId = 'tokens' | 'rights';
const tabFromHash = (): TabId => (typeof window !== 'undefined' && window.location.hash === '#rights' ? 'rights' : 'tokens');

function useHashTab(): [TabId, (tab: TabId) => void] {
  const [tab, setTab] = useState<TabId>('tokens'); // the server render and hydration always start on the first tab
  useEffect(() => {
    const sync = () => setTab(tabFromHash());
    sync();
    window.addEventListener('hashchange', sync);
    return () => window.removeEventListener('hashchange', sync);
  }, []);
  const select = (next: TabId) => {
    setTab(next);
    const { pathname, search } = window.location;
    window.history.replaceState(window.history.state, '', next === 'rights' ? `${pathname}${search}#rights` : `${pathname}${search}`);
  };
  return [tab, select];
}

function AssetDetailView({ index, ticker }: { index: RwaIndex; ticker: string }) {
  const t = useT();
  const f = useFormat();
  const [tab, setTab] = useHashTab();
  const u = index.underlyingByTicker.get(ticker.toUpperCase());
  if (!u) return <NotFound id="notFound.asset" values={{ ticker }} />;

  const tokens = u.token_ids.map((id) => index.tokenById.get(id)).filter((tk): tk is Token => !!tk);
  const excluded = u.excluded_token_ids.map((id) => index.tokenById.get(id)).filter((tk): tk is Token => !!tk);

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

      <HistoryChart
        key={`underlyings:${u.id}`}
        entity="underlyings"
        id={u.ticker}
        metrics={ASSET_HISTORY_METRICS}
        title={t('history.asset.title')}
        subtitle={t('history.subtitle')}
        gutterBottom
      />

      <Box component="section" aria-label={t('asset.sections')} sx={{ marginBottom: 4 }}>
        <Tabs
          value={tab}
          onChange={(_: SyntheticEvent, next: TabId) => setTab(next)}
          aria-label={t('asset.sections')}
          variant="scrollable"
          allowScrollButtonsMobile
          sx={{ marginBottom: 2, borderBottom: 1, borderColor: 'divider' }}
        >
          <Tab value="tokens" id="asset-tab-tokens" aria-controls="asset-panel-tokens" label={t('asset.tokenizedAssets')} />
          <Tab value="rights" id="asset-tab-rights" aria-controls="asset-panel-rights" label={t('rights.compare.title')} />
        </Tabs>
        {/* both panels stay rendered (and in the static HTML); the inactive one is only hidden, so it keeps its sort and switch */}
        <div role="tabpanel" id="asset-panel-tokens" aria-labelledby="asset-tab-tokens" hidden={tab !== 'tokens'}>
          <VersionsTable
            index={index}
            tokens={tokens}
            positionColumns
            toolbarEnd={
              <Link component={LocaleLink} to={`/opportunities?underlying=${u.ticker}`} underline="always">
                {t('asset.allPositions', { ticker: u.ticker })}
              </Link>
            }
          />
        </div>
        <div role="tabpanel" id="asset-panel-rights" aria-labelledby="asset-tab-rights" hidden={tab !== 'rights'}>
          <RightsCompareTable index={index} tokens={tokens} />
        </div>
      </Box>

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
