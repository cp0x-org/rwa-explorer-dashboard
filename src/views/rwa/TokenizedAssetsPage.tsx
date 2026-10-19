import { useMemo, useState } from 'react';
import Box from '@mui/material/Box';
import Grid from '@mui/material/Grid';
import TextField from '@mui/material/TextField';
import Typography from '@mui/material/Typography';

import { RwaIndex } from 'api/rwa';
import { Token } from 'types/rwa';
import { useFormat, useT } from 'i18n';
import DataGate from './components/DataGate';
import Legend from './components/Legend';
import MethodNote from './components/MethodNote';
import MultiFilter from './components/MultiFilter';
import VersionsTable, { deploymentLabel } from './components/VersionsTable';
import FilterPanel from './components/FilterPanel';

const uniqueSorted = (values: string[]) => Array.from(new Set(values)).sort();

// All tokenized assets of the basket in one sortable, filterable table.
function TokenizedAssetsView({ index }: { index: RwaIndex }) {
  const t = useT();
  const f = useFormat();
  const [underlyings, setUnderlyings] = useState<string[]>([]);
  const [issuers, setIssuers] = useState<string[]>([]);
  const [networks, setNetworks] = useState<string[]>([]);
  const [search, setSearch] = useState('');
  const { settings: s } = index.data;
  const thresholds = {
    liquidity: f.usd(s.min_eligible_liquidity_usd),
    hours: s.max_position_age_hours,
    threshold: f.pct(s.yield_apy_review_threshold_percent, 0)
  };

  const allTokens = useMemo(
    () => index.data.underlyings.flatMap((u) => u.token_ids.map((id) => index.tokenById.get(id)).filter((tk): tk is Token => !!tk)),
    [index]
  );
  const underlyingLogos = useMemo(() => new Map(index.data.underlyings.map((u) => [u.ticker, u.logo])), [index]);
  const issuerNames = new Map(index.data.issuers.map((i) => [i.id, i.name ?? i.id]));

  const query = search.trim().toLowerCase();
  const tokens = allTokens.filter(
    (tk) =>
      (underlyings.length === 0 || underlyings.includes(tk.ticker)) &&
      (issuers.length === 0 || (tk.issuer_id !== null && issuers.includes(tk.issuer_id))) &&
      (networks.length === 0 || tk.deployments.some((d) => networks.includes(deploymentLabel(d)))) &&
      (!query || `${tk.symbol ?? ''} ${tk.name ?? ''}`.toLowerCase().includes(query))
  );
  const filtered = underlyings.length + issuers.length + networks.length > 0 || !!query;

  return (
    <Box sx={{ width: '100%' }}>
      <Typography variant="h3" component="h1" gutterBottom>
        {t('tokenized.title')}
      </Typography>
      <Typography variant="body2" color="text.secondary" sx={{ marginBottom: 2, maxWidth: 900 }}>
        {t('tokenized.intro')}
      </Typography>
      <MethodNote data={index.data} />

      <FilterPanel active={[underlyings, issuers, networks].filter((fl) => fl.length > 0).length + (search.trim() ? 1 : 0)}>
        <Grid container spacing={2} sx={{ marginBottom: 2 }}>
          <Grid size={{ xs: 12, md: 3 }}>
            <MultiFilter
              label={t('filter.underlying')}
              options={index.data.underlyings.map((u) => u.ticker)}
              value={underlyings}
              onChange={setUnderlyings}
            />
          </Grid>
          <Grid size={{ xs: 12, md: 3 }}>
            <MultiFilter
              label={t('filter.issuer')}
              options={uniqueSorted(allTokens.map((tk) => tk.issuer_id).filter((id): id is string => !!id))}
              value={issuers}
              onChange={setIssuers}
              getLabel={(id) => issuerNames.get(id) ?? id}
            />
          </Grid>
          <Grid size={{ xs: 12, md: 3 }}>
            <MultiFilter
              label={t('filter.network')}
              options={uniqueSorted(allTokens.flatMap((tk) => tk.deployments.map(deploymentLabel)))}
              value={networks}
              onChange={setNetworks}
            />
          </Grid>
          <Grid size={{ xs: 12, md: 3 }}>
            <TextField label={t('filter.symbolOrName')} value={search} onChange={(e) => setSearch(e.target.value)} size="small" fullWidth />
          </Grid>
        </Grid>
      </FilterPanel>

      <VersionsTable index={index} tokens={tokens} showUnderlying underlyingLogos={underlyingLogos} paginate />
      <Typography variant="body2" color="text.secondary" role="status">
        {t('tokenized.showing', { n: tokens.length, total: allTokens.length })}
        {filtered ? t('tokenized.filtered') : ''}
      </Typography>

      <Legend
        items={[
          { term: t('col.underlying'), description: t('legend.tokenized.underlying') },
          { term: t('col.token'), description: t('legend.tokenized.token') },
          { term: t('col.issuer'), description: t('legend.tokenized.issuer') },
          { term: t('col.networks'), description: t('legend.tokenized.networks') },
          { term: t('col.score'), description: t('legend.tokenized.score', thresholds) },
          { term: t('col.raw'), description: t('legend.tokenized.raw') },
          { term: t('col.protocols'), description: t('legend.tokenized.protocols') },
          { term: t('col.directIndirect'), description: t('legend.tokenized.directIndirect') },
          { term: t('col.apyMaxMedian'), description: t('legend.tokenized.apy', thresholds) },
          { term: t('col.mcapCmc'), description: t('legend.tokenized.mcap') }
        ]}
      />
    </Box>
  );
}

export default function TokenizedAssetsPage() {
  return <DataGate>{(index) => <TokenizedAssetsView index={index} />}</DataGate>;
}
