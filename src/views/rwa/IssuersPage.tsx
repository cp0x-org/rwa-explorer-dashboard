import Box from '@mui/material/Box';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableHead from '@mui/material/TableHead';
import TableRow from '@mui/material/TableRow';
import Typography from '@mui/material/Typography';

import { logoUrl, RwaIndex } from 'api/rwa';
import { TokenIcon } from 'components/TokenIcon';
import { Issuer } from 'types/rwa';
import { compareNullable, num } from 'utils/rwa';
import { useFormat, useLocaleNavigate, useT } from 'i18n';
import DataTableContainer from './components/DataTableContainer';
import DataGate from './components/DataGate';
import Legend from './components/Legend';
import MethodNote from './components/MethodNote';
import SortHeader, { HeaderCell, useSort } from './components/SortHeader';
import TableCaption from './components/TableCaption';
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

function IssuersView({ index }: { index: RwaIndex }) {
  const t = useT();
  const f = useFormat();
  const navigate = useLocaleNavigate();
  const { sortField, sortOrder, onSort } = useSort<Field>('score');
  const { settings: s } = index.data;
  const thresholds = {
    liquidity: f.usd(s.min_eligible_liquidity_usd),
    hours: s.max_position_age_hours,
    threshold: f.pct(s.yield_apy_review_threshold_percent, 0)
  };
  const rows = [...index.data.issuers].sort(
    (a, b) => compareNullable(sortValue[sortField](a), sortValue[sortField](b), sortOrder) || (a.name ?? '').localeCompare(b.name ?? '')
  );
  const header = (field: Field, label: string, hint?: string) => (
    <SortHeader field={field} label={label} hint={hint} sortField={sortField} sortOrder={sortOrder} onSort={onSort} />
  );

  return (
    <Box sx={{ width: '100%' }}>
      <Typography variant="h3" component="h1" gutterBottom>
        {t('issuers.title')}
      </Typography>
      <Typography variant="body2" color="text.secondary" sx={{ marginBottom: 2, maxWidth: 900 }}>
        {t('issuers.intro')}
      </Typography>
      <MethodNote data={index.data} />
      <DataTableContainer sx={[{ marginBottom: 2 }, stickyColumn(2)]}>
        <Table sx={{ minWidth: 650 }}>
          <TableCaption>{t('table.issuers')}</TableCaption>
          <TableHead>
            <TableRow>
              <TableCell>{t('col.rank')}</TableCell>
              <TableCell>{t('col.issuer')}</TableCell>
              {header('score', t('col.score'), t('col.score.hintUnique'))}
              {header('relative', t('col.relative'), t('issuers.relativeHint'))}
              {header('raw', t('col.raw'))}
              {header('protocols', t('col.protocols'))}
              {header('assets', t('col.assets'), t('col.assets.hint'))}
              <HeaderCell label={t('col.deployments')} hint={t('col.deployments.hint')} />
              <HeaderCell label={t('col.directIndirect')} />
              {header('yield', t('col.yield'), t('col.yield.hint'))}
              {header('pools', t('col.poolsLiquidity'), t('col.poolsLiquidity.hint'))}
            </TableRow>
          </TableHead>
          <TableBody>
            {rows.map((i, n) => (
              <TableRow key={i.id} hover onClick={() => navigate(`/issuers/${i.id}`)} sx={{ cursor: 'pointer' }}>
                <TableCell>{n + 1}</TableCell>
                <TableCell component="th" scope="row" sx={{ fontWeight: 'inherit' }}>
                  <RowLink to={`/issuers/${i.id}`}>
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                      <TokenIcon symbol={i.name ?? '?'} src={logoUrl(i.logo)} />
                      <Typography variant="body2" sx={{ fontWeight: 500 }}>
                        {i.name ?? t('common.unknownIssuer')}
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
                  {f.pct(i.basket_relative_score, 1).replace('%', '')}
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
                <TableCell>{f.usd(i.direct_pools_liquidity_usd)}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </DataTableContainer>

      <Legend
        items={[
          { term: t('col.rank'), description: t('legend.issuers.rank') },
          { term: t('col.issuer'), description: t('legend.issuers.issuer') },
          { term: t('col.score'), description: t('legend.issuers.score', thresholds) },
          { term: t('col.relative'), description: t('legend.issuers.relative') },
          { term: t('col.raw'), description: t('legend.issuers.raw') },
          { term: t('col.protocols'), description: t('legend.issuers.protocols') },
          { term: t('col.assets'), description: t('legend.issuers.assets') },
          { term: t('col.deployments'), description: t('legend.issuers.deployments') },
          { term: t('col.directIndirect'), description: t('legend.issuers.directIndirect') },
          { term: t('col.yield'), description: t('legend.issuers.yield', thresholds) },
          { term: t('col.poolsLiquidity'), description: t('legend.issuers.poolsLiquidity') }
        ]}
      />
    </Box>
  );
}

export default function IssuersPage() {
  return <DataGate>{(index) => <IssuersView index={index} />}</DataGate>;
}
