import { Link as RouterLink, useNavigate, useParams } from 'react-router-dom';
import Box from '@mui/material/Box';
import Grid from '@mui/material/Grid';
import Link from '@mui/material/Link';
import Paper from '@mui/material/Paper';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableContainer from '@mui/material/TableContainer';
import TableHead from '@mui/material/TableHead';
import TableRow from '@mui/material/TableRow';
import Typography from '@mui/material/Typography';

import { isReportedYield, logoUrl, relationRows, RwaIndex } from 'api/rwa';
import { TokenIcon } from 'components/TokenIcon';
import { Breakdown, Issuer, Token } from 'types/rwa';
import { categoryLabel, fmtNum, fmtPct, fmtUsd, networkName } from 'utils/rwa';
import BreakdownTable from './components/BreakdownTable';
import DataGate, { NotFound } from './components/DataGate';
import DetailHeader from './components/StatGrid';
import MethodNote from './components/MethodNote';
import OpportunityTable from './components/OpportunityTable';
import { CoverageChip } from './components/StatusChip';
import { NetworkChips } from './components/VersionsTable';
import { RELATIVE_HINT } from './IssuersPage';

function IssuerDetailView({ index, issuer }: { index: RwaIndex; issuer: Issuer }) {
  const navigate = useNavigate();
  const tokens = issuer.token_ids
    .map((id) => index.tokenById.get(id))
    .filter((t): t is Token => !!t)
    .sort((a, b) => a.ticker.localeCompare(b.ticker) || b.usefulness_score - a.usefulness_score);
  const rows = relationRows(index, tokens);
  const yieldRows = rows.filter(isReportedYield);

  // distinct positions per asset: a pool shared by two versions of one asset counts once
  const perAsset = new Map<string, { raw: Set<string>; eligible: Set<string> }>();
  tokens.forEach((t) => perAsset.set(t.ticker, perAsset.get(t.ticker) ?? { raw: new Set(), eligible: new Set() }));
  rows.forEach((r) => {
    const sets = perAsset.get(r.token.ticker)!;
    sets.raw.add(r.opportunity.key);
    if (r.opportunity.eligibility === 'eligible') sets.eligible.add(r.opportunity.key);
  });
  const byUnderlying: Breakdown = Object.fromEntries(
    [...perAsset.entries()]
      .map(([ticker, s]) => [ticker, { raw: s.raw.size, eligible: s.eligible.size }] as const)
      .sort((a, b) => b[1].eligible - a[1].eligible || b[1].raw - a[1].raw)
  );

  return (
    <Box sx={{ width: '100%' }}>
      <DetailHeader
        caption={
          <Link component={RouterLink} to="/issuers" color="inherit" underline="hover">
            Issuers
          </Link>
        }
        title={issuer.name ?? 'Unknown issuer'}
        logo={issuer.logo}
        subtitle={
          <>
            {issuer.website && (
              <Link href={issuer.website} target="_blank" rel="noopener noreferrer" underline="hover">
                {issuer.website.replace(/^https?:\/\//, '')}
              </Link>
            )}
            {issuer.roster_size !== null && ` · ${issuer.roster_size} tokens tracked by CMC in total (not all in this basket)`}
          </>
        }
        chips={<CoverageChip coverage={issuer.coverage} />}
        stats={[
          { label: 'Score', value: fmtNum(issuer.issuer_usefulness_score), hint: 'Unique eligible positions' },
          { label: 'Raw confirmed', value: fmtNum(issuer.raw_confirmed_opportunities_count) },
          { label: 'Protocols', value: fmtNum(issuer.eligible_protocols_count) },
          { label: 'Networks', value: fmtNum(issuer.eligible_networks_count) },
          {
            label: `Relative basket score (${issuer.compared_underlyings_count})`,
            value: fmtPct(issuer.basket_relative_score, 1).replace('%', ''),
            hint: RELATIVE_HINT
          },
          { label: 'Assets / versions', value: `${issuer.basket_underlyings_count} / ${issuer.basket_tokens_count}` },
          {
            label: 'Deployments on Portals networks',
            value: `${issuer.supported_deployments_count} / ${issuer.deployments_count}`
          },
          {
            label: 'Direct pools liquidity',
            value: fmtUsd(issuer.direct_pools_liquidity_usd),
            hint: 'Liquidity of tracked distinct direct pools. Not issuer TVL or capital'
          }
        ]}
      />
      <MethodNote data={index.data} />

      <Typography variant="h3" gutterBottom sx={{ marginBottom: 2 }}>
        Versions In Basket
      </Typography>
      <TableContainer component={Paper} sx={{ marginBottom: 4 }}>
        <Table sx={{ minWidth: 650 }} aria-label="issuer tokens table">
          <TableHead>
            <TableRow>
              <TableCell>Asset</TableCell>
              <TableCell>Version</TableCell>
              <TableCell>Rank in asset</TableCell>
              <TableCell>Score</TableCell>
              <TableCell>Raw</TableCell>
              <TableCell>Protocols</TableCell>
              <TableCell>Networks</TableCell>
              <TableCell>Max APY</TableCell>
              <TableCell>MCap (CMC)</TableCell>
              <TableCell>Coverage</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {tokens.map((t) => (
              <TableRow key={t.id} hover onClick={() => navigate(`/tokens/${t.id}`)} sx={{ cursor: 'pointer' }}>
                <TableCell>{t.ticker}</TableCell>
                <TableCell>
                  <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                    <TokenIcon
                      symbol={t.symbol ?? '?'}
                      src={logoUrl(t.logo)}
                      avatarProps={{ sx: { width: 24, height: 24, fontSize: 10 } }}
                    />
                    <Box>
                      <Typography variant="body2" sx={{ fontWeight: 500 }}>
                        {t.symbol}
                      </Typography>
                      <Typography variant="body2" color="text.secondary">
                        {t.name}
                      </Typography>
                    </Box>
                  </Box>
                </TableCell>
                <TableCell>
                  #{t.rank_in_underlying} of {index.underlyingById.get(t.underlying_id)?.versions_count ?? '—'}
                </TableCell>
                <TableCell>
                  <Typography variant="h5" component="span">
                    {t.usefulness_score}
                  </Typography>
                </TableCell>
                <TableCell>{t.counts.raw_confirmed_opportunities_count}</TableCell>
                <TableCell>{t.counts.eligible_protocols_count}</TableCell>
                <TableCell>
                  <NetworkChips token={t} />
                </TableCell>
                <TableCell>{fmtPct(t.yield.max_reported_apy_percent)}</TableCell>
                <TableCell>{fmtUsd(t.market_cap_usd)}</TableCell>
                <TableCell>
                  <CoverageChip coverage={t.coverage} rawCount={t.counts.raw_confirmed_opportunities_count} />
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </TableContainer>

      <Grid container spacing={2} sx={{ marginBottom: 4 }}>
        <Grid size={{ xs: 12, md: 4 }}>
          <BreakdownTable title="Asset" data={byUnderlying} />
        </Grid>
        <Grid size={{ xs: 12, md: 4 }}>
          <BreakdownTable title="Network" data={issuer.by_network} label={networkName} />
        </Grid>
        <Grid size={{ xs: 12, md: 4 }}>
          <BreakdownTable title="Category" data={issuer.by_category} label={categoryLabel} />
        </Grid>
      </Grid>

      <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'baseline', flexWrap: 'wrap', gap: 1, marginBottom: 2 }}>
        <Typography variant="h3">Highest Reported APY</Typography>
        <Link component={RouterLink} to={`/opportunities?issuer=${issuer.id}`} underline="hover">
          All positions of {issuer.name} →
        </Link>
      </Box>
      <OpportunityTable index={index} rows={yieldRows} showToken defaultRows={10} emptyText="No eligible positions with reported APY" />
    </Box>
  );
}

export default function IssuerDetailPage() {
  const { issuerId = '' } = useParams<{ issuerId: string }>();
  return (
    <DataGate>
      {(index) => {
        const issuer = index.issuerById.get(issuerId);
        return issuer ? <IssuerDetailView index={index} issuer={issuer} /> : <NotFound what="Issuer" />;
      }}
    </DataGate>
  );
}
