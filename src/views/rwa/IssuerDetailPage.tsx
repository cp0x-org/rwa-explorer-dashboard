import { useMemo } from 'react';
import { useParams } from 'react-router-dom';
import Box from '@mui/material/Box';
import Link from '@mui/material/Link';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableHead from '@mui/material/TableHead';
import TableRow from '@mui/material/TableRow';
import Typography from '@mui/material/Typography';

import { isReportedYield, logoUrl, relationRows, RwaIndex, tokenProtocols } from 'api/rwa';
import { TokenIcon } from 'components/TokenIcon';
import { Issuer, RelationRow, Token } from 'types/rwa';
import { compareNullable, num } from 'utils/rwa';
import { LocaleLink, useFormat, useLocaleNavigate, useT } from 'i18n';
import DataTableContainer from './components/DataTableContainer';
import Age from './components/Age';
import DataGate, { NotFound } from './components/DataGate';
import DetailHeader from './components/StatGrid';
import Legend from './components/Legend';
import MethodNote from './components/MethodNote';
import SortHeader, { useSort } from './components/SortHeader';
import TableCaption from './components/TableCaption';
import { compareTokens, NetworkChips, TokenSortField } from './components/VersionsTable';
import { stickyColumn } from './components/tableSx';
import RowLink from './components/RowLink';
import ProtocolsCell from './components/ProtocolsCell';

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
  const t = useT();
  const f = useFormat();
  const navigate = useLocaleNavigate();
  const { sortField, sortOrder, onSort } = useSort<Field>('score');

  const tokens = useMemo(() => issuer.token_ids.map((id) => index.tokenById.get(id)).filter((tk): tk is Token => !!tk), [index, issuer]);
  const best = useMemo(() => bestYieldByToken(relationRows(index, tokens)), [index, tokens]);
  const s = index.data.settings;
  const thresholds = {
    liquidity: f.usd(s.min_eligible_liquidity_usd),
    hours: s.max_position_age_hours,
    threshold: f.pct(s.yield_apy_review_threshold_percent, 0)
  };
  const name = issuer.name ?? t('common.unknownIssuer');

  const sorted = useMemo(() => {
    const positionValue = (tk: Token) => {
      const o = best.get(tk.id)?.opportunity;
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
        crumbs={[{ label: t('nav.issuers'), to: '/issuers' }]}
        title={name}
        logo={issuer.logo}
        subtitle={
          <>
            {issuer.website && (
              <Link href={issuer.website} target="_blank" rel="noopener noreferrer" underline="always">
                {issuer.website.replace(/^https?:\/\//, '')}
              </Link>
            )}
            {issuer.roster_size !== null && ` · ${t('issuer.roster', { n: issuer.roster_size })}`}
          </>
        }
        stats={[
          { label: t('stat.score'), value: f.num(issuer.issuer_usefulness_score), hint: t('stat.score.hint') },
          { label: t('stat.rawConfirmed'), value: f.num(issuer.raw_confirmed_opportunities_count) },
          {
            label: t('stat.protocols'),
            value: f.num(issuer.eligible_protocols_count),
            hint: issuer.eligible_platforms.join(', ') || undefined
          },
          { label: t('stat.networks'), value: f.num(issuer.eligible_networks_count) },
          {
            label: t('stat.relative', { n: issuer.compared_underlyings_count }),
            value: f.pct(issuer.basket_relative_score, 1).replace('%', ''),
            hint: t('issuers.relativeHint')
          },
          { label: t('stat.underlyingTokenized'), value: `${issuer.basket_underlyings_count} / ${issuer.basket_tokens_count}` },
          { label: t('stat.deploymentsPortals'), value: `${issuer.supported_deployments_count} / ${issuer.deployments_count}` },
          { label: t('stat.directPools'), value: f.usd(issuer.direct_pools_liquidity_usd), hint: t('stat.directPools.hint') }
        ]}
      />
      <MethodNote data={index.data} />

      <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'baseline', flexWrap: 'wrap', gap: 1, marginBottom: 2 }}>
        <Typography variant="h3" component="h2">
          {t('issuer.tokensInBasket')}
        </Typography>
        <Link component={LocaleLink} to={`/opportunities?issuer=${issuer.id}`} underline="always">
          {t('issuer.allPositions', { name })}
        </Link>
      </Box>
      <DataTableContainer sx={[{ marginBottom: 4 }, stickyColumn(2)]}>
        <Table sx={{ minWidth: 650, '& .MuiTableCell-root': { paddingX: 1.5 } }}>
          <TableCaption>{t('table.issuerTokens')}</TableCaption>
          <TableHead>
            <TableRow>
              {header('ticker', t('col.asset'))}
              <TableCell>{t('col.token')}</TableCell>
              <TableCell>{t('col.rankInAsset')}</TableCell>
              {header('score', t('col.score'), t('col.score.hint'))}
              {header('protocols', t('col.protocols'))}
              <TableCell>{t('col.networks')}</TableCell>
              {header('liquidity', t('col.liquidity'), t('col.liquidity.hintBest'))}
              {header('apy', t('col.maxApy'), t('col.maxApy.hint'))}
              {header('mcap', t('col.mcapCmc'))}
              {header('updated', t('col.updated'), t('col.updated.hintBest'))}
            </TableRow>
          </TableHead>
          <TableBody>
            {sorted.length === 0 && (
              <TableRow>
                <TableCell colSpan={10} sx={{ color: 'text.secondary' }}>
                  {t('issuer.none')}
                </TableCell>
              </TableRow>
            )}
            {sorted.map((tk) => {
              const o = best.get(tk.id)?.opportunity;
              return (
                <TableRow key={tk.id} hover onClick={() => navigate(`/tokens/${tk.id}`)} sx={{ cursor: 'pointer' }}>
                  <TableCell>{tk.ticker}</TableCell>
                  <TableCell component="th" scope="row" sx={{ fontWeight: 'inherit' }}>
                    <RowLink to={`/tokens/${tk.id}`}>
                      <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                        <TokenIcon
                          symbol={tk.symbol ?? '?'}
                          src={logoUrl(tk.logo)}
                          avatarProps={{ sx: { width: 24, height: 24, fontSize: 10 } }}
                        />
                        <Box>
                          <Typography variant="body2" sx={{ fontWeight: 500 }}>
                            {tk.symbol}
                          </Typography>
                          <Typography variant="body2" color="text.secondary">
                            {tk.name}
                          </Typography>
                        </Box>
                      </Box>
                    </RowLink>
                  </TableCell>
                  <TableCell>
                    {t('common.rankOf', {
                      rank: tk.rank_in_underlying ?? '—',
                      count: index.underlyingById.get(tk.underlying_id)?.versions_count ?? '—'
                    })}
                  </TableCell>
                  <TableCell>
                    <Typography variant="h5" component="span">
                      {tk.usefulness_score}
                    </Typography>
                  </TableCell>
                  <TableCell>
                    <ProtocolsCell count={tk.counts.eligible_protocols_count} names={tokenProtocols(index, tk)} />
                  </TableCell>
                  <TableCell>
                    <NetworkChips token={tk} />
                  </TableCell>
                  <TableCell>{f.usd(o?.liquidity_usd)}</TableCell>
                  <TableCell>{f.pct(tk.yield.max_reported_apy_percent)}</TableCell>
                  <TableCell>{f.usd(tk.market_cap_usd)}</TableCell>
                  <TableCell>{o ? <Age iso={o.updated_at} /> : '—'}</TableCell>
                </TableRow>
              );
            })}
          </TableBody>
        </Table>
      </DataTableContainer>
      <Legend
        items={[
          { term: t('col.asset'), description: t('legend.issuer.asset') },
          { term: t('col.token'), description: t('legend.issuer.token') },
          { term: t('col.rankInAsset'), description: t('legend.issuer.rankInAsset') },
          { term: t('col.score'), description: t('legend.issuer.score', thresholds) },
          { term: t('col.protocols'), description: t('legend.tokenized.protocols') },
          { term: t('col.networks'), description: t('legend.tokenized.networks') },
          { term: t('col.liquidity'), description: t('legend.issuer.liquidity') },
          { term: t('col.maxApy'), description: t('legend.issuer.maxApy', thresholds) },
          { term: t('col.mcapCmc'), description: t('legend.tokenized.mcap') },
          { term: t('col.updated'), description: t('legend.issuer.updated') }
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
        return issuer ? <IssuerDetailView index={index} issuer={issuer} /> : <NotFound id="notFound.issuer" />;
      }}
    </DataGate>
  );
}
