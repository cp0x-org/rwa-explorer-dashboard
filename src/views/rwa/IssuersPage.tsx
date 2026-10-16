import { useNavigate } from 'react-router-dom';
import Box from '@mui/material/Box';
import Paper from '@mui/material/Paper';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableContainer from '@mui/material/TableContainer';
import TableHead from '@mui/material/TableHead';
import TableRow from '@mui/material/TableRow';
import Tooltip from '@mui/material/Tooltip';
import Typography from '@mui/material/Typography';

import { logoUrl, RwaIndex } from 'api/rwa';
import { TokenIcon } from 'components/TokenIcon';
import { Issuer } from 'types/rwa';
import { compareNullable, fmtPct, fmtUsd, num } from 'utils/rwa';
import DataGate from './components/DataGate';
import Legend from './components/Legend';
import MethodNote from './components/MethodNote';
import SortHeader, { useSort } from './components/SortHeader';
import { stickyColumn } from './components/tableSx';
import RowLink from './components/RowLink';

type Field = 'score' | 'relative' | 'raw' | 'protocols' | 'assets' | 'yield' | 'pools';

const sortValue: Record<Field, (i: Issuer) => number | null> = {
  score: (i) => i.issuer_usefulness_score,
  relative: (i) => num(i.basket_relative_score),
  raw: (i) => i.raw_confirmed_opportunities_count,
  protocols: (i) => i.eligible_protocols_count,
  assets: (i) => i.basket_underlyings_count,
  yield: (i) => i.yield_opportunities_count,
  pools: (i) => num(i.direct_pools_liquidity_usd)
};

export const RELATIVE_HINT =
  'For each asset with at least two issuers: issuer positions / best issuer positions × 100, averaged with equal weight per asset. ' +
  'Provisional: based on search coverage.';

function IssuersView({ index }: { index: RwaIndex }) {
  const navigate = useNavigate();
  const { sortField, sortOrder, onSort } = useSort<Field>('score');
  const { settings: s } = index.data;
  const rows = [...index.data.issuers].sort(
    (a, b) => compareNullable(sortValue[sortField](a), sortValue[sortField](b), sortOrder) || (a.name ?? '').localeCompare(b.name ?? '')
  );
  const header = (field: Field, label: string, hint?: string) => (
    <SortHeader field={field} label={label} hint={hint} sortField={sortField} sortOrder={sortOrder} onSort={onSort} />
  );

  return (
    <Box sx={{ width: '100%' }}>
      <Typography variant="h3" gutterBottom>
        Issuers
      </Typography>
      <MethodNote data={index.data} />
      <TableContainer component={Paper} sx={[{ marginBottom: 2 }, stickyColumn(2)]}>
        <Table sx={{ minWidth: 650 }} aria-label="issuers table">
          <TableHead>
            <TableRow>
              <TableCell>#</TableCell>
              <TableCell>Issuer</TableCell>
              <TableCell>{header('score', 'Score', 'Unique eligible positions')}</TableCell>
              <TableCell>{header('relative', 'Relative', RELATIVE_HINT)}</TableCell>
              <TableCell>{header('raw', 'Raw')}</TableCell>
              <TableCell>{header('protocols', 'Protocols')}</TableCell>
              <TableCell>{header('assets', 'Assets', 'Underlying assets / the issuer’s tokenized assets of them')}</TableCell>
              <TableCell>
                <Tooltip title="Deployments on networks covered by Portals / all CMC deployments" arrow>
                  <span>Deployments</span>
                </Tooltip>
              </TableCell>
              <TableCell>Direct / Indirect</TableCell>
              <TableCell>{header('yield', 'Yield', 'Eligible positions with positive reported APY')}</TableCell>
              <TableCell>{header('pools', 'Pools liquidity', 'Liquidity of tracked distinct direct pools. Not issuer TVL')}</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {rows.map((i, n) => (
              <TableRow key={i.id} hover onClick={() => navigate(`/issuers/${i.id}`)} sx={{ cursor: 'pointer' }}>
                <TableCell>{n + 1}</TableCell>
                <TableCell>
                  <RowLink to={`/issuers/${i.id}`}>
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                      <TokenIcon symbol={i.name ?? '?'} src={logoUrl(i.logo)} />
                      <Typography variant="body2" sx={{ fontWeight: 500 }}>
                        {i.name ?? 'Unknown issuer'}
                      </Typography>
                    </Box>
                  </RowLink>
                </TableCell>
                <TableCell>
                  <Typography variant="h5" component="span">
                    {i.issuer_usefulness_score}
                  </Typography>
                </TableCell>
                <TableCell>
                  {fmtPct(i.basket_relative_score, 1).replace('%', '')}
                  {i.compared_underlyings_count > 0 && (
                    <Typography component="span" variant="body2" color="text.secondary">
                      {' '}
                      ({i.compared_underlyings_count})
                    </Typography>
                  )}
                </TableCell>
                <TableCell>{i.raw_confirmed_opportunities_count}</TableCell>
                <TableCell>{i.eligible_protocols_count}</TableCell>
                <TableCell>
                  {i.basket_underlyings_count} / {i.basket_tokens_count}
                </TableCell>
                <TableCell>
                  {i.supported_deployments_count} / {i.deployments_count}
                </TableCell>
                <TableCell>
                  {i.direct_eligible_count} / {i.indirect_eligible_count}
                </TableCell>
                <TableCell>{i.yield_opportunities_count}</TableCell>
                <TableCell>{fmtUsd(i.direct_pools_liquidity_usd)}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </TableContainer>

      <Legend
        items={[
          { term: '#', description: 'Position of the issuer in the current sort.' },
          {
            term: 'Issuer',
            description:
              'Issuer of tokenized assets, as labeled by CoinMarketCap (not a verified legal entity). Click a row for its tokenized assets and positions.'
          },
          {
            term: 'Score',
            description:
              `Unique eligible DeFi positions across all tokenized assets of the issuer in this basket. A position counts as eligible when the token ` +
              `address is confirmed in its composition (directly or via LP → vault), it is a real use-case (DEX LP, lending supply, vault, ` +
              `staking, yield market), liquidity ≥ ${fmtUsd(s.min_eligible_liquidity_usd)} and source data is not older than ` +
              `${s.max_position_age_hours}h at collection. A pool shared by two tokenized assets of the same issuer counts once; tokens outside the ` +
              `basket do not count. This is the main rating.`
          },
          {
            term: 'Relative',
            description:
              'Basket relative score, 0–100: for every asset with at least two issuers, the issuer’s eligible positions divided by the best ' +
              'issuer’s on that asset × 100, averaged with equal weight per asset. In brackets: number of compared assets. Shows quality ' +
              'per asset regardless of how many assets the issuer covers. Provisional: based on search coverage.'
          },
          {
            term: 'Raw',
            description: 'Unique positions confirmed by contract address before the use-case, liquidity and freshness filters.'
          },
          { term: 'Protocols', description: 'Distinct DeFi protocols among the eligible positions.' },
          { term: 'Assets', description: 'Underlying assets of the basket covered by the issuer / the issuer’s tokenized assets of them.' },
          {
            term: 'Deployments',
            description:
              'Token contracts on networks covered by Portals / all contracts listed by CoinMarketCap. Contracts on other networks ' +
              '(Solana, TON, X Layer, …) are not searched: usage there is unknown, not zero.'
          },
          {
            term: 'Direct / Indirect',
            description:
              'Eligible positions holding the tokenized asset itself (e.g. a DEX pool) / holding it through another position (e.g. a vault that holds an LP with the token).'
          },
          {
            term: 'Yield',
            description: `Eligible positions with a positive reported APY (APY above ${fmtPct(s.yield_apy_review_threshold_percent, 0)} excluded as anomaly).`
          },
          {
            term: 'Pools liquidity',
            description:
              'Sum of liquidity of distinct eligible DEX pools that directly hold the issuer’s tokenized assets. Not the issuer’s TVL or capital: vaults ' +
              'on top of these pools are not added.'
          }
        ]}
      />
    </Box>
  );
}

export default function IssuersPage() {
  return <DataGate>{(index) => <IssuersView index={index} />}</DataGate>;
}
