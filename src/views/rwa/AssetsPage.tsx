import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import Box from '@mui/material/Box';
import Grid from '@mui/material/Grid';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableHead from '@mui/material/TableHead';
import TableRow from '@mui/material/TableRow';
import TextField from '@mui/material/TextField';
import Typography from '@mui/material/Typography';

import { logoUrl, RwaIndex } from 'api/rwa';
import { TokenIcon } from 'components/TokenIcon';
import { Underlying } from 'types/rwa';
import { compareNullable, fmtNum, fmtUsd, num } from 'utils/rwa';
import DataTableContainer from './components/DataTableContainer';
import Age from './components/Age';
import DataGate from './components/DataGate';
import MethodNote from './components/MethodNote';
import SortHeader, { useSort } from './components/SortHeader';
import { StatGrid } from './components/StatGrid';
import Legend from './components/Legend';
import { stickyColumn } from './components/tableSx';
import RowLink from './components/RowLink';

type Field = 'ticker' | 'versions' | 'issuers' | 'eligible' | 'raw' | 'protocols' | 'mcap';

const sortValue: Record<Field, (u: Underlying) => number | string | null> = {
  ticker: (u) => u.ticker,
  versions: (u) => u.versions_count,
  issuers: (u) => u.issuers_count,
  eligible: (u) => u.eligible_opportunities_count,
  raw: (u) => u.raw_confirmed_opportunities_count,
  protocols: (u) => u.eligible_protocols_count,
  mcap: (u) => num(u.tokenized_market_cap_usd)
};

function AssetsView({ index }: { index: RwaIndex }) {
  const navigate = useNavigate();
  const { sortField, sortOrder, onSort } = useSort<Field>('eligible');
  const [nameFilter, setNameFilter] = useState('');
  const { summary, settings: s } = index.data;

  const query = nameFilter.trim().toLowerCase();
  const rows = index.data.underlyings
    .filter((u) => !query || u.ticker.toLowerCase().includes(query) || u.name.toLowerCase().includes(query))
    .sort((a, b) => compareNullable(sortValue[sortField](a), sortValue[sortField](b), sortOrder) || a.ticker.localeCompare(b.ticker));
  const header = (field: Field, label: string, hint?: string) => (
    <SortHeader field={field} label={label} hint={hint} sortField={sortField} sortOrder={sortOrder} onSort={onSort} />
  );

  return (
    <Box sx={{ width: '100%' }}>
      <Typography variant="h3" gutterBottom>
        Underlying Assets
      </Typography>
      <MethodNote data={index.data} />

      <Box sx={{ marginBottom: 3 }}>
        <StatGrid
          stats={[
            { label: 'Underlying assets', value: fmtNum(summary.underlyings_count) },
            { label: 'Tokenized assets', value: fmtNum(summary.tokens_count), hint: 'Derivatives and other share classes excluded' },
            { label: 'Issuers', value: fmtNum(summary.issuers_count) },
            {
              label: 'Eligible positions',
              value: fmtNum(summary.eligible_opportunities_count),
              hint: `${fmtNum(summary.raw_confirmed_opportunities_count)} confirmed by address before liquidity/freshness filters`
            },
            { label: 'Protocols', value: fmtNum(summary.eligible_protocols_count) },
            { label: 'Networks', value: fmtNum(summary.eligible_networks_count) },
            {
              label: 'Deployments in Portals',
              value: `${fmtNum(summary.indexed_deployments_count)} / ${fmtNum(summary.deployments_count)}`,
              hint: 'Contract deployments indexed by Portals out of all CMC deployments'
            }
          ]}
        />
      </Box>

      <Grid container spacing={2} sx={{ marginBottom: 2 }}>
        <Grid size={{ xs: 12, md: 4 }}>
          <TextField
            label="Filter By Ticker or Name"
            value={nameFilter}
            onChange={(e) => setNameFilter(e.target.value)}
            size="small"
            fullWidth
          />
        </Grid>
      </Grid>

      <DataTableContainer sx={[{ marginBottom: 2 }, stickyColumn(1)]}>
        <Table sx={{ minWidth: 650 }} aria-label="underlying assets table">
          <TableHead>
            <TableRow>
              <TableCell>{header('ticker', 'Asset')}</TableCell>
              <TableCell>
                {header('versions', 'Tokenized assets', 'Tokenized assets of the stock or ETF tracked by CoinMarketCap')}
              </TableCell>
              <TableCell>{header('issuers', 'Issuers')}</TableCell>
              <TableCell>{header('eligible', 'Eligible', 'Distinct eligible DeFi positions across all tokenized assets')}</TableCell>
              <TableCell>{header('raw', 'Raw', 'Distinct positions confirmed by address, before filters')}</TableCell>
              <TableCell>{header('protocols', 'Protocols')}</TableCell>
              <TableCell>{header('mcap', 'Tokenized MCap', 'CoinMarketCap tokenized market cap')}</TableCell>
              <TableCell>Updated</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {rows.map((u) => (
              <TableRow key={u.id} hover onClick={() => navigate(`/assets/${u.ticker}`)} sx={{ cursor: 'pointer' }}>
                <TableCell>
                  <RowLink to={`/assets/${u.ticker}`}>
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                      <TokenIcon symbol={u.ticker} src={logoUrl(u.logo)} />
                      <Box>
                        <Typography variant="body2" sx={{ fontWeight: 500 }}>
                          {u.ticker}
                        </Typography>
                        <Typography variant="body2" color="text.secondary">
                          {u.name}
                        </Typography>
                      </Box>
                    </Box>
                  </RowLink>
                </TableCell>
                <TableCell>{u.versions_count}</TableCell>
                <TableCell>{u.issuers_count}</TableCell>
                <TableCell>{u.eligible_opportunities_count}</TableCell>
                <TableCell>{u.raw_confirmed_opportunities_count}</TableCell>
                <TableCell>{u.eligible_protocols_count}</TableCell>
                <TableCell>{fmtUsd(u.tokenized_market_cap_usd)}</TableCell>
                <TableCell>
                  <Age iso={u.fetched_at} />
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </DataTableContainer>
      <Typography variant="body2" color="text.secondary">
        Showing {rows.length} {rows.length === 1 ? 'asset' : 'assets'}
        {query ? ` (filtered by "${nameFilter}")` : ''}
      </Typography>

      <Legend
        items={[
          { term: 'Asset', description: 'Ticker and name of the underlying stock or ETF. Click a row to compare its tokenized assets.' },
          {
            term: 'Tokenized assets',
            description:
              'On-chain tokens of the stock or ETF tracked by CoinMarketCap (one per issuer product). Derivatives and tokens of another share class are excluded.'
          },
          { term: 'Issuers', description: 'Distinct issuers of these tokenized assets, as labeled by CoinMarketCap.' },
          {
            term: 'Eligible',
            description:
              `Distinct DeFi positions that contain any of its tokenized assets and pass all filters: the token address is confirmed ` +
              `in the position composition (directly or via LP → vault), the position is a real use-case (DEX LP, lending supply, ` +
              `vault, staking, yield market), liquidity ≥ ${fmtUsd(s.min_eligible_liquidity_usd)} and source data not older than ` +
              `${s.max_position_age_hours}h at collection. A position shared by two tokenized assets counts once. This is the main rating.`
          },
          {
            term: 'Raw',
            description: 'Distinct positions confirmed by contract address before the use-case, liquidity and freshness filters.'
          },
          { term: 'Protocols', description: 'Distinct DeFi protocols among the eligible positions.' },
          {
            term: 'Tokenized MCap',
            description: 'CoinMarketCap market cap of all its tokenized assets. Not DeFi liquidity.'
          },
          { term: 'Updated', description: 'When CoinMarketCap and Portals data for this asset was collected.' }
        ]}
      />
    </Box>
  );
}

export default function AssetsPage() {
  return <DataGate>{(index) => <AssetsView index={index} />}</DataGate>;
}
