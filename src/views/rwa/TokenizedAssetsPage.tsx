import { useMemo, useState } from 'react';
import Box from '@mui/material/Box';
import Grid from '@mui/material/Grid';
import TextField from '@mui/material/TextField';
import Typography from '@mui/material/Typography';

import { RwaIndex } from 'api/rwa';
import { Token } from 'types/rwa';
import { fmtPct, fmtUsd } from 'utils/rwa';
import DataGate from './components/DataGate';
import Legend from './components/Legend';
import MethodNote from './components/MethodNote';
import MultiFilter from './components/MultiFilter';
import VersionsTable, { deploymentLabel } from './components/VersionsTable';
import FilterPanel from './components/FilterPanel';

const uniqueSorted = (values: string[]) => Array.from(new Set(values)).sort();

// All tokenized assets of the basket in one sortable, filterable table.
function TokenizedAssetsView({ index }: { index: RwaIndex }) {
  const [underlyings, setUnderlyings] = useState<string[]>([]);
  const [issuers, setIssuers] = useState<string[]>([]);
  const [networks, setNetworks] = useState<string[]>([]);
  const [search, setSearch] = useState('');
  const { settings: s } = index.data;

  const allTokens = useMemo(
    () => index.data.underlyings.flatMap((u) => u.token_ids.map((id) => index.tokenById.get(id)).filter((t): t is Token => !!t)),
    [index]
  );
  const underlyingLogos = useMemo(() => new Map(index.data.underlyings.map((u) => [u.ticker, u.logo])), [index]);
  const issuerNames = new Map(index.data.issuers.map((i) => [i.id, i.name ?? i.id]));

  const query = search.trim().toLowerCase();
  const tokens = allTokens.filter(
    (t) =>
      (underlyings.length === 0 || underlyings.includes(t.ticker)) &&
      (issuers.length === 0 || (t.issuer_id !== null && issuers.includes(t.issuer_id))) &&
      (networks.length === 0 || t.deployments.some((d) => networks.includes(deploymentLabel(d)))) &&
      (!query || `${t.symbol ?? ''} ${t.name ?? ''}`.toLowerCase().includes(query))
  );
  const filtered = underlyings.length + issuers.length + networks.length > 0 || !!query;

  return (
    <Box sx={{ width: '100%' }}>
      <Typography variant="h3" gutterBottom>
        Tokenized Assets
      </Typography>
      <MethodNote data={index.data} />

      <FilterPanel active={[underlyings, issuers, networks].filter((f) => f.length > 0).length + (search.trim() ? 1 : 0)}>
        <Grid container spacing={2} sx={{ marginBottom: 2 }}>
          <Grid size={{ xs: 12, md: 3 }}>
            <MultiFilter
              label="Filter By Underlying"
              options={index.data.underlyings.map((u) => u.ticker)}
              value={underlyings}
              onChange={setUnderlyings}
            />
          </Grid>
          <Grid size={{ xs: 12, md: 3 }}>
            <MultiFilter
              label="Filter By Issuer"
              options={uniqueSorted(allTokens.map((t) => t.issuer_id).filter((id): id is string => !!id))}
              value={issuers}
              onChange={setIssuers}
              getLabel={(id) => issuerNames.get(id) ?? id}
            />
          </Grid>
          <Grid size={{ xs: 12, md: 3 }}>
            <MultiFilter
              label="Filter By Network"
              options={uniqueSorted(allTokens.flatMap((t) => t.deployments.map(deploymentLabel)))}
              value={networks}
              onChange={setNetworks}
            />
          </Grid>
          <Grid size={{ xs: 12, md: 3 }}>
            <TextField label="Filter By Symbol or Name" value={search} onChange={(e) => setSearch(e.target.value)} size="small" fullWidth />
          </Grid>
        </Grid>
      </FilterPanel>

      <VersionsTable tokens={tokens} showUnderlying underlyingLogos={underlyingLogos} paginate />
      <Typography variant="body2" color="text.secondary">
        Showing {tokens.length} of {allTokens.length} tokenized assets{filtered ? ' (filtered)' : ''}
      </Typography>

      <Legend
        items={[
          {
            term: 'Underlying',
            description:
              'Ticker of the underlying stock or ETF. Click a row to open the token details; its rank among tokenized assets of ' +
              'the same underlying is shown on the underlying and issuer pages.'
          },
          {
            term: 'Token',
            description:
              'Symbol of the tokenized asset in CoinMarketCap; the full name is in the tooltip. "Wrapper" marks a wrapper around another token.'
          },
          { term: 'Issuer', description: 'Issuer of the token, as labeled by CoinMarketCap (not a verified legal entity).' },
          {
            term: 'Networks',
            description:
              'Networks where CoinMarketCap lists a contract of the token. Highlighted: indexed by Portals and searched for DeFi positions. ' +
              'Greyed: network not covered by Portals or token not indexed — usage there is unknown, not zero.'
          },
          {
            term: 'Score',
            description:
              `Number of eligible DeFi positions with this token: the token address is confirmed in the position composition ` +
              `(directly or via LP → vault), the position is a real use-case (DEX LP, lending supply, vault, staking, yield market), ` +
              `liquidity ≥ ${fmtUsd(s.min_eligible_liquidity_usd)} and source data not older than ${s.max_position_age_hours}h at collection. ` +
              `This is the main rating.`
          },
          {
            term: 'Raw',
            description: 'Positions confirmed by contract address before the use-case, liquidity and freshness filters.'
          },
          { term: 'Protocols', description: 'Distinct DeFi protocols among the eligible positions.' },
          {
            term: 'Direct / Indirect',
            description:
              'Eligible positions holding the token itself (e.g. a DEX pool) / holding it through another position (e.g. a vault that holds an LP with the token).'
          },
          {
            term: 'APY max / median',
            description:
              `Highest and median reported APY over eligible positions with positive APY. APY above ` +
              `${fmtPct(s.yield_apy_review_threshold_percent, 0)} is treated as an anomaly and excluded. A snapshot of one position, ` +
              `not an expected return; "—" means no reported yield in the current coverage.`
          },
          {
            term: 'MCap (CMC)',
            description: 'CoinMarketCap market cap of this token. "—" means not reported; $0.00 is a zero reported by CoinMarketCap.'
          },
          {
            term: 'Coverage',
            description:
              'Search coverage: positions found via Portals text search and confirmed by address. No positions: searched, nothing contains ' +
              'the token address. Not indexed / Unsupported by Portals: DeFi usage is unknown.'
          }
        ]}
      />
    </Box>
  );
}

export default function TokenizedAssetsPage() {
  return <DataGate>{(index) => <TokenizedAssetsView index={index} />}</DataGate>;
}
