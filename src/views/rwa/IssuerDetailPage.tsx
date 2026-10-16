import { useMemo } from 'react';
import { Link as RouterLink, useNavigate, useParams } from 'react-router-dom';
import Box from '@mui/material/Box';
import Link from '@mui/material/Link';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableHead from '@mui/material/TableHead';
import TableRow from '@mui/material/TableRow';
import Typography from '@mui/material/Typography';

import { isReportedYield, logoUrl, relationRows, RwaIndex } from 'api/rwa';
import { TokenIcon } from 'components/TokenIcon';
import { Issuer, RelationRow, Token } from 'types/rwa';
import { compareNullable, fmtNum, fmtPct, fmtUsd, num } from 'utils/rwa';
import DataTableContainer from './components/DataTableContainer';
import Age from './components/Age';
import DataGate, { NotFound } from './components/DataGate';
import DetailHeader from './components/StatGrid';
import Legend from './components/Legend';
import MethodNote from './components/MethodNote';
import SortHeader, { useSort } from './components/SortHeader';
import { CoverageChip } from './components/StatusChip';
import { compareTokens, NetworkChips, TokenSortField } from './components/VersionsTable';
import { stickyColumn } from './components/tableSx';
import RowLink from './components/RowLink';
import { RELATIVE_HINT } from './IssuersPage';

// Token columns sort by the token itself; "liquidity" and "updated" sort by the token's best reported-APY position.
type Field = TokenSortField | 'liquidity' | 'updated';

// Highest reported APY position of each token (same rule as the pipeline's yield profile).
function bestYieldByToken(rows: RelationRow[]): Map<number, RelationRow> {
  const best = new Map<number, RelationRow>();
  rows.filter(isReportedYield).forEach((r) => {
    const current = best.get(r.token.id);
    if (!current || Number(r.opportunity.apy_percent) > Number(current.opportunity.apy_percent)) best.set(r.token.id, r);
  });
  return best;
}

function IssuerDetailView({ index, issuer }: { index: RwaIndex; issuer: Issuer }) {
  const navigate = useNavigate();
  const { sortField, sortOrder, onSort } = useSort<Field>('score');

  const tokens = useMemo(() => issuer.token_ids.map((id) => index.tokenById.get(id)).filter((t): t is Token => !!t), [index, issuer]);
  const best = useMemo(() => bestYieldByToken(relationRows(index, tokens)), [index, tokens]);
  const s = index.data.settings;

  const sorted = useMemo(() => {
    const positionValue = (t: Token) => {
      const o = best.get(t.id)?.opportunity;
      return sortField === 'liquidity' ? num(o?.liquidity_usd) : (o?.updated_at ?? null);
    };
    const compare = (a: Token, b: Token) =>
      sortField === 'liquidity' || sortField === 'updated'
        ? compareNullable(positionValue(a), positionValue(b), sortOrder)
        : compareTokens(a, b, sortField, sortOrder);
    return [...tokens].sort((a, b) => compare(a, b) || a.ticker.localeCompare(b.ticker) || a.id - b.id);
  }, [tokens, best, sortField, sortOrder]);

  const header = (field: Field, label: string, hint?: string) => (
    <SortHeader field={field} label={label} hint={hint} sortField={sortField} sortOrder={sortOrder} onSort={onSort} />
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
          { label: 'Underlying / tokenized assets', value: `${issuer.basket_underlyings_count} / ${issuer.basket_tokens_count}` },
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

      <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'baseline', flexWrap: 'wrap', gap: 1, marginBottom: 2 }}>
        <Typography variant="h3">Tokenized Assets In Basket</Typography>
        <Link component={RouterLink} to={`/opportunities?issuer=${issuer.id}`} underline="hover">
          All positions of {issuer.name} →
        </Link>
      </Box>
      <DataTableContainer sx={[{ marginBottom: 4 }, stickyColumn(2)]}>
        <Table sx={{ minWidth: 650, '& .MuiTableCell-root': { paddingX: 1.5 } }} aria-label="issuer tokens table">
          <TableHead>
            <TableRow>
              <TableCell>{header('ticker', 'Asset')}</TableCell>
              <TableCell>Token</TableCell>
              <TableCell>Rank in asset</TableCell>
              <TableCell>{header('score', 'Score', 'Eligible positions; ties: protocols, networks, direct positions')}</TableCell>
              <TableCell>{header('protocols', 'Protocols')}</TableCell>
              <TableCell>Networks</TableCell>
              <TableCell>{header('liquidity', 'Liquidity', 'Liquidity of the best position')}</TableCell>
              <TableCell>
                {header('apy', 'Max APY', 'Highest reported APY over eligible positions. APY of one position, not an expected return')}
              </TableCell>
              <TableCell>{header('mcap', 'MCap (CMC)')}</TableCell>
              <TableCell>{header('updated', 'Updated', 'Source timestamp of the best position')}</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {sorted.length === 0 && (
              <TableRow>
                <TableCell colSpan={10} sx={{ color: 'text.secondary' }}>
                  No tokenized assets of this issuer in the basket
                </TableCell>
              </TableRow>
            )}
            {sorted.map((t) => {
              const o = best.get(t.id)?.opportunity;
              return (
                <TableRow key={t.id} hover onClick={() => navigate(`/tokens/${t.id}`)} sx={{ cursor: 'pointer' }}>
                  <TableCell>{t.ticker}</TableCell>
                  <TableCell>
                    <RowLink to={`/tokens/${t.id}`}>
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
                    </RowLink>
                  </TableCell>
                  <TableCell>
                    #{t.rank_in_underlying} of {index.underlyingById.get(t.underlying_id)?.versions_count ?? '—'}
                  </TableCell>
                  <TableCell>
                    <Typography variant="h5" component="span">
                      {t.usefulness_score}
                    </Typography>
                  </TableCell>
                  <TableCell>{t.counts.eligible_protocols_count}</TableCell>
                  <TableCell>
                    <NetworkChips token={t} />
                  </TableCell>
                  <TableCell>{fmtUsd(o?.liquidity_usd)}</TableCell>
                  <TableCell>{fmtPct(t.yield.max_reported_apy_percent)}</TableCell>
                  <TableCell>{fmtUsd(t.market_cap_usd)}</TableCell>
                  <TableCell>{o ? <Age iso={o.updated_at} /> : '—'}</TableCell>
                </TableRow>
              );
            })}
          </TableBody>
        </Table>
      </DataTableContainer>
      <Legend
        items={[
          {
            term: 'Asset',
            description: 'Ticker of the underlying stock or ETF. Click a row to open the token details.'
          },
          {
            term: 'Token',
            description: 'Symbol and name of the issuer’s tokenized asset in CoinMarketCap.'
          },
          {
            term: 'Rank in asset',
            description: 'Place of this token among all tokenized versions of the same underlying in the basket, by score.'
          },
          {
            term: 'Score',
            description:
              `Number of eligible DeFi positions with this token: the token address is confirmed in the position composition ` +
              `(directly or via LP → vault), the position is a real use-case (DEX LP, lending supply, vault, staking, yield market), ` +
              `liquidity ≥ ${fmtUsd(s.min_eligible_liquidity_usd)} and source data not older than ${s.max_position_age_hours}h at collection. ` +
              `Ties are broken by protocols, networks and direct positions. This is the main rating.`
          },
          { term: 'Protocols', description: 'Distinct DeFi protocols among the eligible positions.' },
          {
            term: 'Networks',
            description:
              'Networks where CoinMarketCap lists a contract of the token. Highlighted: indexed by Portals and searched for DeFi positions. ' +
              'Greyed: network not covered by Portals or token not indexed — usage there is unknown, not zero.'
          },
          {
            term: 'Liquidity',
            description:
              'Liquidity of the token’s best position: the eligible position with the highest reported APY. ' +
              'Not the total liquidity of the token; "—" means no position with reported APY.'
          },
          {
            term: 'Max APY',
            description:
              `Highest reported APY over eligible positions with positive APY. APY above ${fmtPct(s.yield_apy_review_threshold_percent, 0)} ` +
              `is treated as an anomaly and excluded. A snapshot of one position, not an expected return.`
          },
          {
            term: 'MCap (CMC)',
            description: 'CoinMarketCap market cap of this token. "—" means not reported; $0.00 is a zero reported by CoinMarketCap.'
          },
          { term: 'Updated', description: 'Source timestamp of the best position at collection.' }
        ]}
      />
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
