import { useId, useState } from 'react';
import { useParams } from 'react-router-dom';
import Box from '@mui/material/Box';
import Chip from '@mui/material/Chip';
import FormControl from '@mui/material/FormControl';
import Grid from '@mui/material/Grid';
import InputLabel from '@mui/material/InputLabel';
import Link from '@mui/material/Link';
import MenuItem from '@mui/material/MenuItem';
import Select from '@mui/material/Select';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableHead from '@mui/material/TableHead';
import TableRow from '@mui/material/TableRow';
import Typography from '@mui/material/Typography';

import { relationRows, RwaIndex, tokenProtocols } from 'api/rwa';
import { CopyableAddress } from 'components/CopyableAddress';
import { Token } from 'types/rwa';
import { categoryLabel, excludedReason, networkName } from 'utils/rwa';
import { LocaleLink, useFormat, useT } from 'i18n';
import DataTableContainer from './components/DataTableContainer';
import BreakdownTable from './components/BreakdownTable';
import DataGate, { NotFound } from './components/DataGate';
import DetailHeader from './components/StatGrid';
import Legend from './components/Legend';
import MethodNote from './components/MethodNote';
import MultiFilter from './components/MultiFilter';
import OpportunityTable from './components/OpportunityTable';
import { CoverageChip, DeploymentChip } from './components/StatusChip';
import TableCaption from './components/TableCaption';
import { stickyColumn } from './components/tableSx';
import FilterPanel from './components/FilterPanel';

function TokenDetailView({ index, token }: { index: RwaIndex; token: Token }) {
  const t = useT();
  const f = useFormat();
  const statusId = useId();
  const linkId = useId();
  const [networkFilter, setNetworkFilter] = useState<string[]>([]);
  const [categoryFilter, setCategoryFilter] = useState<string[]>([]);
  const [status, setStatus] = useState<'eligible' | 'all'>('all');
  const [link, setLink] = useState<'all' | 'direct' | 'indirect'>('all');

  const u = index.underlyingById.get(token.underlying_id);
  const c = token.counts;
  const allRows = relationRows(index, [token]);
  const rows = allRows.filter(
    (r) =>
      (networkFilter.length === 0 || networkFilter.includes(r.opportunity.network)) &&
      (categoryFilter.length === 0 || categoryFilter.includes(r.opportunity.category)) &&
      (status === 'all' || r.opportunity.eligibility === 'eligible') &&
      (link === 'all' || (link === 'direct' ? r.relation?.depth === 1 : (r.relation?.depth ?? 0) > 1))
  );
  const unconfirmed = relationRows(index, [token], true).filter((r) => r.relation === null);
  const networks = Array.from(new Set(allRows.map((r) => r.opportunity.network))).sort();
  const categories = Array.from(new Set(allRows.map((r) => r.opportunity.category))).sort();
  const issuer = token.issuer_id ? index.issuerById.get(token.issuer_id) : undefined;
  const s = index.data.settings;
  const thresholds = {
    liquidity: f.usd(s.min_eligible_liquidity_usd),
    hours: s.max_position_age_hours,
    threshold: f.pct(s.yield_apy_review_threshold_percent, 0)
  };
  const title = token.symbol ?? `CMC #${token.id}`;

  return (
    <Box sx={{ width: '100%' }}>
      <DetailHeader
        crumbs={[
          { label: t('nav.assets'), to: '/assets' },
          { label: t('token.caption', { ticker: token.ticker }), to: `/assets/${token.ticker}` }
        ]}
        title={title}
        logo={token.logo}
        subtitle={
          <>
            {token.name} · {t('token.issuer')}{' '}
            {issuer ? (
              <Link component={LocaleLink} to={`/issuers/${issuer.id}`} underline="always">
                {issuer.name}
              </Link>
            ) : (
              (token.issuer_name ?? t('token.issuerUnknown'))
            )}
            {token.cmc_slug && (
              <>
                {' · '}
                <Link
                  href={`https://coinmarketcap.com/currencies/${token.cmc_slug}/`}
                  target="_blank"
                  rel="noopener noreferrer"
                  underline="always"
                >
                  {t('token.cmc')}
                </Link>
              </>
            )}
          </>
        }
        chips={
          <>
            {token.rank_in_underlying && u && (
              <Chip size="small" label={t('token.rank', { rank: token.rank_in_underlying, count: u.versions_count, ticker: u.ticker })} />
            )}
            {token.kind === 'wrapper' && <Chip size="small" label={t('common.wrapper')} />}
            <CoverageChip coverage={token.coverage} rawCount={c.raw_confirmed_opportunities_count} />
          </>
        }
        stats={[
          { label: t('stat.usefulness'), value: f.num(token.usefulness_score), hint: t('stat.usefulness.hint') },
          { label: t('stat.rawConfirmed'), value: f.num(c.raw_confirmed_opportunities_count), hint: t('stat.rawConfirmed.hint') },
          {
            label: t('stat.protocols'),
            value: f.num(c.eligible_protocols_count),
            hint: tokenProtocols(index, token).join(', ') || undefined
          },
          { label: t('stat.networks'), value: f.num(c.eligible_networks_count) },
          {
            label: t('stat.directIndirect'),
            value: `${c.direct_eligible_count} / ${c.indirect_eligible_count}`,
            hint: t('stat.directIndirect.hint')
          },
          { label: t('stat.maxReportedApy'), value: f.pct(token.yield.max_reported_apy_percent), hint: t('stat.maxReportedApy.hint') },
          { label: t('stat.medianReportedApy'), value: f.pct(token.yield.median_reported_apy_percent) },
          { label: t('stat.marketCap'), value: f.usd(token.market_cap_usd) }
        ]}
      />
      <MethodNote data={index.data} />

      {token.excluded_reason && (
        <Typography variant="body1" color="warning.main" sx={{ marginBottom: 3 }}>
          {t('token.excluded', { reason: excludedReason(t, token.excluded_reason) })}
        </Typography>
      )}

      <Typography variant="h3" component="h2" gutterBottom sx={{ marginBottom: 2 }}>
        {t('token.deployments')}
      </Typography>
      <DataTableContainer sx={[{ marginBottom: 4 }, stickyColumn(1)]}>
        <Table sx={{ minWidth: 650 }}>
          <TableCaption>{t('table.deployments')}</TableCaption>
          <TableHead>
            <TableRow>
              <TableCell>{t('col.networkCmc')}</TableCell>
              <TableCell>{t('col.portalsNetwork')}</TableCell>
              <TableCell>{t('col.address')}</TableCell>
              <TableCell>{t('col.portalsSymbol')}</TableCell>
              <TableCell>{t('col.decimals')}</TableCell>
              <TableCell>{t('col.status')}</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {token.deployments.length === 0 && (
              <TableRow>
                <TableCell colSpan={6} sx={{ color: 'text.secondary' }}>
                  {t('token.noContracts')}
                </TableCell>
              </TableRow>
            )}
            {token.deployments.map((d) => (
              <TableRow key={`${d.cmc_network}:${d.address}`}>
                <TableCell component="th" scope="row" sx={{ fontWeight: 'inherit' }}>
                  {d.cmc_network}
                </TableCell>
                <TableCell>{d.network ? networkName(d.network) : '—'}</TableCell>
                <TableCell>
                  <CopyableAddress address={d.address} />
                </TableCell>
                <TableCell>{d.portals_symbol ?? '—'}</TableCell>
                <TableCell>{f.num(d.decimals)}</TableCell>
                <TableCell>
                  <DeploymentChip status={d.status} />
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </DataTableContainer>

      <Grid container spacing={2} sx={{ marginBottom: 4 }}>
        <Grid size={{ xs: 12, md: 4 }}>
          <BreakdownTable title={t('token.breakdown.network')} data={token.by_network} label={networkName} />
        </Grid>
        <Grid size={{ xs: 12, md: 4 }}>
          <BreakdownTable title={t('token.breakdown.category')} data={token.by_category} label={(k) => categoryLabel(t, k)} />
        </Grid>
        <Grid size={{ xs: 12, md: 4 }}>
          <BreakdownTable
            title={t('token.breakdown.protocol')}
            data={token.by_platform}
            label={(p) => allRows.find((r) => r.opportunity.platform === p)?.opportunity.platform_name ?? p}
          />
        </Grid>
      </Grid>

      <Typography variant="h3" component="h2" gutterBottom sx={{ marginBottom: 2 }}>
        {t('token.positions')}
      </Typography>
      <FilterPanel
        active={
          [networkFilter, categoryFilter].filter((fl) => fl.length > 0).length + (status !== 'all' ? 1 : 0) + (link !== 'all' ? 1 : 0)
        }
      >
        <Grid container spacing={2} sx={{ marginBottom: 2 }}>
          <Grid size={{ xs: 12, md: 3 }}>
            <MultiFilter
              label={t('filter.network')}
              options={networks}
              value={networkFilter}
              onChange={setNetworkFilter}
              getLabel={networkName}
            />
          </Grid>
          <Grid size={{ xs: 12, md: 3 }}>
            <MultiFilter
              label={t('filter.category')}
              options={categories}
              value={categoryFilter}
              onChange={setCategoryFilter}
              getLabel={(k) => categoryLabel(t, k)}
            />
          </Grid>
          <Grid size={{ xs: 6, md: 3 }}>
            <FormControl size="small" fullWidth>
              <InputLabel id={statusId}>{t('filter.status')}</InputLabel>
              <Select
                labelId={statusId}
                value={status}
                label={t('filter.status')}
                onChange={(e) => setStatus(e.target.value as typeof status)}
              >
                <MenuItem value="all">{t('filter.status.allConfirmed')}</MenuItem>
                <MenuItem value="eligible">{t('filter.status.eligibleOnly')}</MenuItem>
              </Select>
            </FormControl>
          </Grid>
          <Grid size={{ xs: 6, md: 3 }}>
            <FormControl size="small" fullWidth>
              <InputLabel id={linkId}>{t('filter.link')}</InputLabel>
              <Select labelId={linkId} value={link} label={t('filter.link')} onChange={(e) => setLink(e.target.value as typeof link)}>
                <MenuItem value="all">{t('filter.link.all')}</MenuItem>
                <MenuItem value="direct">{t('filter.link.direct')}</MenuItem>
                <MenuItem value="indirect">{t('filter.link.indirect')}</MenuItem>
              </Select>
            </FormControl>
          </Grid>
        </Grid>
      </FilterPanel>
      <OpportunityTable index={index} rows={rows} caption={`${t('token.positions')}: ${title}`} />

      {unconfirmed.length > 0 && (
        <Box sx={{ marginTop: 2 }}>
          <Typography variant="h3" component="h2" gutterBottom>
            {t('token.unconfirmed')}
          </Typography>
          <Typography variant="body2" color="text.secondary" sx={{ marginBottom: 2 }}>
            {t('token.unconfirmedText')}
          </Typography>
          <OpportunityTable index={index} rows={unconfirmed} defaultSort="liquidity" caption={`${t('token.unconfirmed')}: ${title}`} />
        </Box>
      )}

      <Legend
        items={[
          { term: t('token.deployments'), description: t('legend.token.deployments') },
          { term: t('legend.token.breakdownTerm'), description: t('legend.token.breakdown') },
          { term: t('col.position'), description: t('legend.position.position') },
          { term: t('col.protocol'), description: t('legend.position.protocol') },
          { term: t('col.network'), description: t('legend.position.network') },
          { term: t('col.category'), description: t('legend.position.category') },
          { term: t('col.link'), description: t('legend.position.link') },
          { term: t('col.liquidity'), description: t('legend.position.liquidity') },
          { term: t('col.reportedApy'), description: t('legend.position.apy', thresholds) },
          { term: t('col.status'), description: t('legend.position.status', thresholds) },
          { term: t('col.updated'), description: t('legend.position.updated') }
        ]}
      />
    </Box>
  );
}

export default function TokenDetailPage() {
  const { tokenId = '' } = useParams<{ tokenId: string }>();
  return (
    <DataGate>
      {(index) => {
        const token = index.tokenById.get(Number(tokenId));
        return token ? (
          <TokenDetailView key={token.id} index={index} token={token} />
        ) : (
          <NotFound id="notFound.token" values={{ id: tokenId }} />
        );
      }}
    </DataGate>
  );
}
