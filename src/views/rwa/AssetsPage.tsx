import { useState } from 'react';
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
import { compareNullable, num } from 'utils/rwa';
import { useFormat, useLocaleNavigate, useT } from 'i18n';
import DataTableContainer from './components/DataTableContainer';
import Age from './components/Age';
import DataGate from './components/DataGate';
import MethodNote from './components/MethodNote';
import SortHeader, { HeaderCell, useSort } from './components/SortHeader';
import { StatGrid } from './components/StatGrid';
import Legend from './components/Legend';
import TableCaption from './components/TableCaption';
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
  const t = useT();
  const f = useFormat();
  const navigate = useLocaleNavigate();
  const { sortField, sortOrder, onSort } = useSort<Field>('eligible');
  const [nameFilter, setNameFilter] = useState('');
  const { summary, settings: s } = index.data;
  const thresholds = { liquidity: f.usd(s.min_eligible_liquidity_usd), hours: s.max_position_age_hours };

  const query = nameFilter.trim().toLowerCase();
  const rows = index.data.underlyings
    .filter((u) => !query || u.ticker.toLowerCase().includes(query) || u.name.toLowerCase().includes(query))
    .sort((a, b) => compareNullable(sortValue[sortField](a), sortValue[sortField](b), sortOrder) || a.ticker.localeCompare(b.ticker));
  const header = (field: Field, label: string, hint?: string) => (
    <SortHeader field={field} label={label} hint={hint} sortField={sortField} sortOrder={sortOrder} onSort={onSort} />
  );

  return (
    <Box sx={{ width: '100%' }}>
      <Typography variant="h3" component="h1" gutterBottom>
        {t('assets.title')}
      </Typography>
      <Typography variant="body2" color="text.secondary" sx={{ marginBottom: 2, maxWidth: 900 }}>
        {t('assets.intro')}
      </Typography>
      <MethodNote data={index.data} />

      <Box sx={{ marginBottom: 3 }}>
        <StatGrid
          stats={[
            { label: t('stat.underlyings'), value: f.num(summary.underlyings_count) },
            { label: t('stat.tokens'), value: f.num(summary.tokens_count), hint: t('stat.tokens.hint') },
            { label: t('stat.issuers'), value: f.num(summary.issuers_count) },
            {
              label: t('stat.eligible'),
              value: f.num(summary.eligible_opportunities_count),
              hint: t('stat.eligible.hintRaw', { n: f.num(summary.raw_confirmed_opportunities_count) })
            },
            { label: t('stat.protocols'), value: f.num(summary.eligible_protocols_count) },
            { label: t('stat.networks'), value: f.num(summary.eligible_networks_count) },
            {
              label: t('stat.deployments'),
              value: `${f.num(summary.indexed_deployments_count)} / ${f.num(summary.deployments_count)}`,
              hint: t('stat.deployments.hint')
            }
          ]}
        />
      </Box>

      <Grid container spacing={2} sx={{ marginBottom: 2 }}>
        <Grid size={{ xs: 12, md: 4 }}>
          <TextField
            label={t('filter.tickerOrName')}
            value={nameFilter}
            onChange={(e) => setNameFilter(e.target.value)}
            size="small"
            fullWidth
          />
        </Grid>
      </Grid>

      <DataTableContainer sx={[{ marginBottom: 2 }, stickyColumn(1)]}>
        <Table sx={{ minWidth: 650 }}>
          <TableCaption>{t('table.assets')}</TableCaption>
          <TableHead>
            <TableRow>
              {header('ticker', t('col.asset'))}
              {header('versions', t('col.tokenized'), t('col.tokenized.hint'))}
              {header('issuers', t('col.issuers'))}
              {header('eligible', t('col.eligible'), t('col.eligible.hint'))}
              {header('raw', t('col.raw'), t('col.raw.hint'))}
              {header('protocols', t('col.protocols'))}
              {header('mcap', t('col.tokenizedMcap'), t('col.tokenizedMcap.hint'))}
              <HeaderCell label={t('col.updated')} />
            </TableRow>
          </TableHead>
          <TableBody>
            {rows.map((u) => (
              <TableRow key={u.id} hover onClick={() => navigate(`/assets/${u.ticker}`)} sx={{ cursor: 'pointer' }}>
                <TableCell component="th" scope="row" sx={{ fontWeight: 'inherit' }}>
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
                <TableCell>{f.usd(u.tokenized_market_cap_usd)}</TableCell>
                <TableCell>
                  <Age iso={u.fetched_at} />
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </DataTableContainer>
      <Typography variant="body2" color="text.secondary" role="status">
        {t('assets.showing', { n: rows.length })}
        {query ? t('assets.filteredBy', { query: nameFilter }) : ''}
      </Typography>

      <Legend
        items={[
          { term: t('col.asset'), description: t('legend.assets.asset') },
          { term: t('col.tokenized'), description: t('legend.assets.tokenized') },
          { term: t('col.issuers'), description: t('legend.assets.issuers') },
          { term: t('col.eligible'), description: t('legend.assets.eligible', thresholds) },
          { term: t('col.raw'), description: t('legend.assets.raw') },
          { term: t('col.protocols'), description: t('legend.assets.protocols') },
          { term: t('col.tokenizedMcap'), description: t('legend.assets.tokenizedMcap') },
          { term: t('col.updated'), description: t('legend.assets.updated') }
        ]}
      />
    </Box>
  );
}

export default function AssetsPage() {
  return <DataGate>{(index) => <AssetsView index={index} />}</DataGate>;
}
