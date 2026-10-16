import { useState } from 'react';
import { Link as RouterLink, useParams } from 'react-router-dom';
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

import { relationRows, RwaIndex } from 'api/rwa';
import { CopyableAddress } from 'components/CopyableAddress';
import { Token } from 'types/rwa';
import { categoryLabel, EXCLUDED_REASONS, fmtNum, fmtPct, fmtUsd, networkName } from 'utils/rwa';
import DataTableContainer from './components/DataTableContainer';
import BreakdownTable from './components/BreakdownTable';
import DataGate, { NotFound } from './components/DataGate';
import DetailHeader from './components/StatGrid';
import Legend from './components/Legend';
import MethodNote from './components/MethodNote';
import MultiFilter from './components/MultiFilter';
import OpportunityTable from './components/OpportunityTable';
import { CoverageChip, DeploymentChip } from './components/StatusChip';
import { stickyColumn } from './components/tableSx';
import FilterPanel from './components/FilterPanel';

function TokenDetailView({ index, token }: { index: RwaIndex; token: Token }) {
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

  return (
    <Box sx={{ width: '100%' }}>
      <DetailHeader
        caption={
          <>
            Tokenized{' '}
            <Link component={RouterLink} to={`/assets/${token.ticker}`} color="inherit" underline="hover">
              {token.ticker}
            </Link>
          </>
        }
        title={token.symbol ?? `CMC #${token.id}`}
        logo={token.logo}
        subtitle={
          <>
            {token.name} · Issuer:{' '}
            {issuer ? (
              <Link component={RouterLink} to={`/issuers/${issuer.id}`} underline="hover">
                {issuer.name}
              </Link>
            ) : (
              (token.issuer_name ?? 'unknown')
            )}
            {token.cmc_slug && (
              <>
                {' · '}
                <Link
                  href={`https://coinmarketcap.com/currencies/${token.cmc_slug}/`}
                  target="_blank"
                  rel="noopener noreferrer"
                  underline="hover"
                >
                  CoinMarketCap
                </Link>
              </>
            )}
          </>
        }
        chips={
          <>
            {token.rank_in_underlying && u && (
              <Chip size="small" label={`#${token.rank_in_underlying} of ${u.versions_count} in ${u.ticker}`} />
            )}
            {token.kind === 'wrapper' && <Chip size="small" label="Wrapper" />}
            <CoverageChip coverage={token.coverage} rawCount={c.raw_confirmed_opportunities_count} />
          </>
        }
        stats={[
          { label: 'Usefulness score', value: fmtNum(token.usefulness_score), hint: 'Number of eligible DeFi positions' },
          { label: 'Raw confirmed', value: fmtNum(c.raw_confirmed_opportunities_count), hint: 'Confirmed by address, before filters' },
          { label: 'Protocols', value: fmtNum(c.eligible_protocols_count) },
          { label: 'Networks', value: fmtNum(c.eligible_networks_count) },
          {
            label: 'Direct / Indirect',
            value: `${c.direct_eligible_count} / ${c.indirect_eligible_count}`,
            hint: 'Indirect: e.g. vault holding an LP with the token'
          },
          {
            label: 'Max reported APY',
            value: fmtPct(token.yield.max_reported_apy_percent),
            hint: 'Highest reported APY of one eligible position'
          },
          { label: 'Median reported APY', value: fmtPct(token.yield.median_reported_apy_percent) },
          { label: 'Market cap (CMC)', value: fmtUsd(token.market_cap_usd) }
        ]}
      />
      <MethodNote data={index.data} />

      {token.excluded_reason && (
        <Typography variant="body1" color="warning.main" sx={{ marginBottom: 3 }}>
          Excluded from rankings: {EXCLUDED_REASONS[token.excluded_reason] ?? token.excluded_reason}
        </Typography>
      )}

      <Typography variant="h3" gutterBottom sx={{ marginBottom: 2 }}>
        Deployments
      </Typography>
      <DataTableContainer sx={[{ marginBottom: 4 }, stickyColumn(1)]}>
        <Table sx={{ minWidth: 650 }} aria-label="deployments table">
          <TableHead>
            <TableRow>
              <TableCell>Network (CMC)</TableCell>
              <TableCell>Portals network</TableCell>
              <TableCell>Address</TableCell>
              <TableCell>Portals symbol</TableCell>
              <TableCell>Decimals</TableCell>
              <TableCell>Status</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {token.deployments.length === 0 && (
              <TableRow>
                <TableCell colSpan={6} sx={{ color: 'text.secondary' }}>
                  CMC lists no contract addresses for this token
                </TableCell>
              </TableRow>
            )}
            {token.deployments.map((d) => (
              <TableRow key={`${d.cmc_network}:${d.address}`}>
                <TableCell>{d.cmc_network}</TableCell>
                <TableCell>{d.network ? networkName(d.network) : '—'}</TableCell>
                <TableCell>
                  <CopyableAddress address={d.address} />
                </TableCell>
                <TableCell>{d.portals_symbol ?? '—'}</TableCell>
                <TableCell>{fmtNum(d.decimals)}</TableCell>
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
          <BreakdownTable title="Network" data={token.by_network} label={networkName} />
        </Grid>
        <Grid size={{ xs: 12, md: 4 }}>
          <BreakdownTable title="Category" data={token.by_category} label={categoryLabel} />
        </Grid>
        <Grid size={{ xs: 12, md: 4 }}>
          <BreakdownTable
            title="Protocol"
            data={token.by_platform}
            label={(p) => allRows.find((r) => r.opportunity.platform === p)?.opportunity.platform_name ?? p}
          />
        </Grid>
      </Grid>

      <Typography variant="h3" gutterBottom sx={{ marginBottom: 2 }}>
        Positions
      </Typography>
      <FilterPanel
        active={[networkFilter, categoryFilter].filter((f) => f.length > 0).length + (status !== 'all' ? 1 : 0) + (link !== 'all' ? 1 : 0)}
      >
        <Grid container spacing={2} sx={{ marginBottom: 2 }}>
          <Grid size={{ xs: 12, md: 3 }}>
            <MultiFilter
              label="Filter By Network"
              options={networks}
              value={networkFilter}
              onChange={setNetworkFilter}
              getLabel={networkName}
            />
          </Grid>
          <Grid size={{ xs: 12, md: 3 }}>
            <MultiFilter
              label="Filter By Category"
              options={categories}
              value={categoryFilter}
              onChange={setCategoryFilter}
              getLabel={categoryLabel}
            />
          </Grid>
          <Grid size={{ xs: 6, md: 3 }}>
            <FormControl size="small" fullWidth>
              <InputLabel>Status</InputLabel>
              <Select value={status} label="Status" onChange={(e) => setStatus(e.target.value as typeof status)}>
                <MenuItem value="all">All confirmed</MenuItem>
                <MenuItem value="eligible">Eligible only</MenuItem>
              </Select>
            </FormControl>
          </Grid>
          <Grid size={{ xs: 6, md: 3 }}>
            <FormControl size="small" fullWidth>
              <InputLabel>Link</InputLabel>
              <Select value={link} label="Link" onChange={(e) => setLink(e.target.value as typeof link)}>
                <MenuItem value="all">Direct and indirect</MenuItem>
                <MenuItem value="direct">Direct</MenuItem>
                <MenuItem value="indirect">Indirect</MenuItem>
              </Select>
            </FormControl>
          </Grid>
        </Grid>
      </FilterPanel>
      <OpportunityTable index={index} rows={rows} />

      {unconfirmed.length > 0 && (
        <Box sx={{ marginTop: 2 }}>
          <Typography variant="h3" gutterBottom>
            Unconfirmed Candidates
          </Typography>
          <Typography variant="body2" color="text.secondary" sx={{ marginBottom: 2 }}>
            Symbol matches on the same network, but the position composition in Portals does not contain this token address (e.g. lending
            markets that expose only the loan asset). Not counted anywhere.
          </Typography>
          <OpportunityTable index={index} rows={unconfirmed} defaultSort="liquidity" />
        </Box>
      )}

      <Legend
        items={[
          {
            term: 'Deployments',
            description:
              'Contracts of the token listed by CoinMarketCap, with the matching Portals network, symbol and decimals. Indexed: Portals knows ' +
              'the contract and its positions were searched. Not indexed / Unsupported by Portals: DeFi usage on that network is unknown, not zero.'
          },
          {
            term: 'Network / Category / Protocol',
            description: 'Eligible positions of the token grouped by network, use-case category and DeFi protocol. Raw: before the filters.'
          },
          {
            term: 'Position',
            description:
              'A DeFi position indexed by Portals: pool, vault, lending market or yield market. Click a row for details: composition, ' +
              'link path, APY breakdown, volume and flags.'
          },
          { term: 'Protocol', description: 'DeFi protocol of the position (Portals platform).' },
          { term: 'Network', description: 'Blockchain where the position lives.' },
          {
            term: 'Category',
            description:
              'DEX LP: liquidity in a pool with the token. Lending supply: the token supplied to a lending market. Vault: a vault or ' +
              'strategy holding the token or its LP. Yield market: PT/YT and similar markets. Wrapper: a wrapped token, not a use-case.'
          },
          {
            term: 'Link',
            description:
              'Direct: the position holds the token itself. Indirect (n): it holds the token through other positions, n levels deep ' +
              '(e.g. vault → LP → token); hover for the path. Symbol match: the name matches, but the token address is not in the ' +
              'composition — unconfirmed and never counted.'
          },
          {
            term: 'Liquidity',
            description: 'Pool TVL or vault deposits reported by Portals, in USD. Values of nested positions are never summed.'
          },
          {
            term: 'Reported APY',
            description:
              `Total APY reported by Portals for this position (base + rewards): a snapshot, not an expected return. ⚠ marks APY above ` +
              `${fmtPct(s.yield_apy_review_threshold_percent, 0)}, excluded from yield statistics. Borrow APY is a cost and is shown only in the details.`
          },
          {
            term: 'Status',
            description:
              `Eligible: counted in the ratings. Low liquidity: below ${fmtUsd(s.min_eligible_liquidity_usd)}. Stale: source data older ` +
              `than ${s.max_position_age_hours}h at collection. Not a use-case: wrapper or unknown category. Unconfirmed: symbol match only.`
          },
          { term: 'Updated', description: 'How long ago Portals last updated the data of this position.' }
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
        return token ? <TokenDetailView key={token.id} index={index} token={token} /> : <NotFound what={`Token ${tokenId}`} />;
      }}
    </DataGate>
  );
}
