import { Link as RouterLink, useParams } from 'react-router-dom';
import Box from '@mui/material/Box';
import Chip from '@mui/material/Chip';
import Link from '@mui/material/Link';
import Paper from '@mui/material/Paper';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableContainer from '@mui/material/TableContainer';
import TableHead from '@mui/material/TableHead';
import TableRow from '@mui/material/TableRow';
import Typography from '@mui/material/Typography';

import { isReportedYield, relationRows, RwaIndex } from 'api/rwa';
import { Token } from 'types/rwa';
import { EXCLUDED_REASONS, fmtNum, fmtUsd, UNDERLYING_STATUS } from 'utils/rwa';
import DataGate, { NotFound } from './components/DataGate';
import DetailHeader from './components/StatGrid';
import MethodNote from './components/MethodNote';
import OpportunityTable from './components/OpportunityTable';
import { CoverageChip } from './components/StatusChip';
import VersionsTable from './components/VersionsTable';

function AssetDetailView({ index, ticker }: { index: RwaIndex; ticker: string }) {
  const u = index.underlyingByTicker.get(ticker.toUpperCase());
  if (!u) return <NotFound what={`Asset ${ticker}`} />;

  const tokens = u.token_ids.map((id) => index.tokenById.get(id)).filter((t): t is Token => !!t);
  const excluded = u.excluded_token_ids.map((id) => index.tokenById.get(id)).filter((t): t is Token => !!t);
  const yieldRows = relationRows(index, tokens).filter(isReportedYield);

  return (
    <Box sx={{ width: '100%' }}>
      <DetailHeader
        caption={
          <Link component={RouterLink} to="/assets" color="inherit" underline="hover">
            Underlying assets
          </Link>
        }
        title={u.ticker}
        logo={u.logo}
        subtitle={
          <>
            {`${u.cmc_name ?? u.name} · ISIN ${u.isin ?? '—'} · ${u.asset_type.toUpperCase()}`}
            {u.website && (
              <>
                {' · '}
                <Link href={u.website} target="_blank" rel="noopener noreferrer" underline="hover">
                  {u.website.replace(/^https?:\/\/(www\.)?/, '')}
                </Link>
              </>
            )}
          </>
        }
        chips={
          u.status === 'ok' ? <CoverageChip coverage={u.coverage} /> : <Chip size="small" label={UNDERLYING_STATUS[u.status] ?? u.status} />
        }
        stats={[
          { label: 'Tokenized versions', value: fmtNum(u.versions_count) },
          { label: 'Issuers', value: fmtNum(u.issuers_count) },
          { label: 'Eligible positions', value: fmtNum(u.eligible_opportunities_count), hint: 'Distinct positions across versions' },
          { label: 'Raw confirmed', value: fmtNum(u.raw_confirmed_opportunities_count) },
          { label: 'Protocols', value: fmtNum(u.eligible_protocols_count) },
          { label: 'Networks', value: fmtNum(u.eligible_networks_count) },
          { label: 'Tokenized MCap (CMC)', value: fmtUsd(u.tokenized_market_cap_usd) },
          { label: 'Avg tokenized price (CMC)', value: fmtUsd(u.average_tokenized_price_usd) }
        ]}
      />
      <MethodNote data={index.data} />

      <Typography variant="h3" gutterBottom sx={{ marginBottom: 2 }}>
        Tokenized Versions
      </Typography>
      <VersionsTable tokens={tokens} />

      <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'baseline', flexWrap: 'wrap', gap: 1, marginBottom: 2 }}>
        <Typography variant="h3">Highest Reported APY</Typography>
        <Link component={RouterLink} to={`/opportunities?underlying=${u.ticker}`} underline="hover">
          All {u.ticker} positions →
        </Link>
      </Box>
      <OpportunityTable index={index} rows={yieldRows} showToken emptyText="No eligible positions with reported APY" />

      {excluded.length > 0 && (
        <Box sx={{ marginTop: 4 }}>
          <Typography variant="h3" gutterBottom sx={{ marginBottom: 2 }}>
            Excluded From Ranking
          </Typography>
          <TableContainer component={Paper}>
            <Table sx={{ minWidth: 650 }} size="small" aria-label="excluded tokens table">
              <TableHead>
                <TableRow>
                  <TableCell>Symbol</TableCell>
                  <TableCell>Name</TableCell>
                  <TableCell>CMC issuer label</TableCell>
                  <TableCell>Reason</TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {excluded.map((t) => (
                  <TableRow key={t.id}>
                    <TableCell>{t.symbol ?? '—'}</TableCell>
                    <TableCell>{t.name ?? '—'}</TableCell>
                    <TableCell>{t.issuer_name ?? '—'}</TableCell>
                    <TableCell>{EXCLUDED_REASONS[t.excluded_reason ?? ''] ?? t.excluded_reason}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </TableContainer>
        </Box>
      )}
    </Box>
  );
}

export default function AssetDetailPage() {
  const { ticker = '' } = useParams<{ ticker: string }>();
  return <DataGate>{(index) => <AssetDetailView index={index} ticker={ticker} />}</DataGate>;
}
