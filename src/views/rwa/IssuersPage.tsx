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

import { RwaIndex } from 'api/rwa';
import { TokenIcon } from 'components/TokenIcon';
import { Issuer } from 'types/rwa';
import { compareNullable, fmtPct, fmtUsd, num } from 'utils/rwa';
import DataGate from './components/DataGate';
import MethodNote from './components/MethodNote';
import SortHeader, { useSort } from './components/SortHeader';

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
      <Typography variant="body2" color="text.secondary" sx={{ marginBottom: 2 }}>
        Score = unique eligible positions across the issuer&apos;s tokens in this basket (a pool shared by two of its tokens counts once).
        Tokens outside the basket do not affect the score.
      </Typography>

      <TableContainer component={Paper} sx={{ marginBottom: 2 }}>
        <Table sx={{ minWidth: 650 }} aria-label="issuers table">
          <TableHead>
            <TableRow>
              <TableCell>#</TableCell>
              <TableCell>Issuer</TableCell>
              <TableCell>{header('score', 'Score', 'Unique eligible positions')}</TableCell>
              <TableCell>{header('relative', 'Relative', RELATIVE_HINT)}</TableCell>
              <TableCell>{header('raw', 'Raw')}</TableCell>
              <TableCell>{header('protocols', 'Protocols')}</TableCell>
              <TableCell>{header('assets', 'Assets', 'Basket assets / tokenized versions')}</TableCell>
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
                  <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                    <TokenIcon symbol={i.name ?? '?'} />
                    <Typography variant="body2" sx={{ fontWeight: 500 }}>
                      {i.name ?? 'Unknown issuer'}
                    </Typography>
                  </Box>
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
      <Typography variant="body2" color="text.secondary">
        Relative score in brackets: number of compared assets. Issuer names are CoinMarketCap labels, not verified legal entities.
      </Typography>
    </Box>
  );
}

export default function IssuersPage() {
  return <DataGate>{(index) => <IssuersView index={index} />}</DataGate>;
}
