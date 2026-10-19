import { useId, useMemo, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import Box from '@mui/material/Box';
import FormControl from '@mui/material/FormControl';
import Grid from '@mui/material/Grid';
import InputLabel from '@mui/material/InputLabel';
import MenuItem from '@mui/material/MenuItem';
import Select from '@mui/material/Select';
import TextField from '@mui/material/TextField';
import Typography from '@mui/material/Typography';

import { relationRows, RwaIndex } from 'api/rwa';
import { categoryLabel, networkName } from 'utils/rwa';
import { useFormat, useT } from 'i18n';
import DataGate from './components/DataGate';
import Legend from './components/Legend';
import MethodNote from './components/MethodNote';
import MultiFilter from './components/MultiFilter';
import OpportunityTable from './components/OpportunityTable';
import FilterPanel from './components/FilterPanel';

type Status = 'eligible' | 'confirmed' | 'all';
type LinkFilter = 'all' | 'direct' | 'indirect';

const uniqueSorted = (values: string[]) => Array.from(new Set(values)).sort();

function OpportunitiesView({ index }: { index: RwaIndex }) {
  const t = useT();
  const f = useFormat();
  const statusId = useId();
  const linkId = useId();
  const [params] = useSearchParams();
  const [underlyings, setUnderlyings] = useState<string[]>(params.get('underlying') ? [params.get('underlying')!] : []);
  const [issuers, setIssuers] = useState<string[]>(params.get('issuer') ? [params.get('issuer')!] : []);
  const [networks, setNetworks] = useState<string[]>([]);
  const [categories, setCategories] = useState<string[]>([]);
  const [platforms, setPlatforms] = useState<string[]>([]);
  const [status, setStatus] = useState<Status>('eligible');
  const [link, setLink] = useState<LinkFilter>('all');
  const [search, setSearch] = useState('');

  // rows = (token, position) pairs; a pool with two tracked tokens appears once per token
  const allRows = useMemo(
    () =>
      relationRows(
        index,
        index.data.tokens.filter((tk) => tk.kind !== 'derivative' && tk.kind !== 'unclassified'),
        true
      ),
    [index]
  );
  const platformNames = useMemo(() => new Map(allRows.map((r) => [r.opportunity.platform, r.opportunity.platform_name])), [allRows]);
  const { settings: s } = index.data;
  const thresholds = {
    liquidity: f.usd(s.min_eligible_liquidity_usd),
    hours: s.max_position_age_hours,
    threshold: f.pct(s.yield_apy_review_threshold_percent, 0)
  };
  const issuerNames = new Map(index.data.issuers.map((i) => [i.id, i.name ?? i.id]));

  const query = search.trim().toLowerCase();
  const rows = allRows.filter((r) => {
    const o = r.opportunity;
    if (status === 'eligible' && (!r.relation || o.eligibility !== 'eligible')) return false;
    if (status === 'confirmed' && !r.relation) return false;
    if (link === 'direct' && r.relation?.depth !== 1) return false;
    if (link === 'indirect' && !(r.relation && r.relation.depth > 1)) return false;
    return (
      (underlyings.length === 0 || underlyings.includes(r.token.ticker)) &&
      (issuers.length === 0 || (r.token.issuer_id !== null && issuers.includes(r.token.issuer_id))) &&
      (networks.length === 0 || networks.includes(o.network)) &&
      (categories.length === 0 || categories.includes(o.category)) &&
      (platforms.length === 0 || platforms.includes(o.platform)) &&
      (!query || `${o.symbol ?? ''} ${o.name ?? ''} ${r.token.symbol ?? ''}`.toLowerCase().includes(query))
    );
  });
  const distinctPositions = new Set(rows.map((r) => r.opportunity.key)).size;

  return (
    <Box sx={{ width: '100%' }}>
      <Typography variant="h3" component="h1" gutterBottom>
        {t('positions.title')}
      </Typography>
      <Typography variant="body2" color="text.secondary" sx={{ marginBottom: 2, maxWidth: 900 }}>
        {t('positions.intro')}
      </Typography>
      <MethodNote data={index.data} />

      <FilterPanel
        active={
          [underlyings, issuers, networks, categories, platforms].filter((fl) => fl.length > 0).length +
          (search.trim() ? 1 : 0) +
          (status !== 'eligible' ? 1 : 0) +
          (link !== 'all' ? 1 : 0)
        }
      >
        <Grid container spacing={2} sx={{ marginBottom: 2 }}>
          <Grid size={{ xs: 12, md: 4 }}>
            <MultiFilter
              label={t('filter.underlying')}
              options={index.data.underlyings.map((u) => u.ticker)}
              value={underlyings}
              onChange={setUnderlyings}
            />
          </Grid>
          <Grid size={{ xs: 12, md: 4 }}>
            <MultiFilter
              label={t('filter.issuer')}
              options={index.data.issuers.map((i) => i.id)}
              value={issuers}
              onChange={setIssuers}
              getLabel={(id) => issuerNames.get(id) ?? id}
            />
          </Grid>
          <Grid size={{ xs: 12, md: 4 }}>
            <MultiFilter
              label={t('filter.network')}
              options={uniqueSorted(allRows.map((r) => r.opportunity.network))}
              value={networks}
              onChange={setNetworks}
              getLabel={networkName}
            />
          </Grid>
          <Grid size={{ xs: 12, md: 4 }}>
            <MultiFilter
              label={t('filter.category')}
              options={uniqueSorted(allRows.map((r) => r.opportunity.category))}
              value={categories}
              onChange={setCategories}
              getLabel={(k) => categoryLabel(t, k)}
            />
          </Grid>
          <Grid size={{ xs: 12, md: 4 }}>
            <MultiFilter
              label={t('filter.protocol')}
              options={uniqueSorted(allRows.map((r) => r.opportunity.platform))}
              value={platforms}
              onChange={setPlatforms}
              getLabel={(p) => platformNames.get(p) ?? p}
            />
          </Grid>
          <Grid size={{ xs: 12, md: 4 }}>
            <TextField label={t('filter.name')} value={search} onChange={(e) => setSearch(e.target.value)} size="small" fullWidth />
          </Grid>
          <Grid size={{ xs: 6, md: 4 }}>
            <FormControl size="small" fullWidth>
              <InputLabel id={statusId}>{t('filter.status')}</InputLabel>
              <Select labelId={statusId} value={status} label={t('filter.status')} onChange={(e) => setStatus(e.target.value as Status)}>
                <MenuItem value="eligible">{t('filter.status.eligible')}</MenuItem>
                <MenuItem value="confirmed">{t('filter.status.confirmedRaw')}</MenuItem>
                <MenuItem value="all">{t('filter.status.inclUnconfirmed')}</MenuItem>
              </Select>
            </FormControl>
          </Grid>
          <Grid size={{ xs: 6, md: 4 }}>
            <FormControl size="small" fullWidth>
              <InputLabel id={linkId}>{t('filter.link')}</InputLabel>
              <Select labelId={linkId} value={link} label={t('filter.link')} onChange={(e) => setLink(e.target.value as LinkFilter)}>
                <MenuItem value="all">{t('filter.link.all')}</MenuItem>
                <MenuItem value="direct">{t('filter.link.direct')}</MenuItem>
                <MenuItem value="indirect">{t('filter.link.indirect')}</MenuItem>
              </Select>
            </FormControl>
          </Grid>
        </Grid>
      </FilterPanel>

      <OpportunityTable index={index} rows={rows} showToken defaultRows={25} />
      <Typography variant="body2" color="text.secondary" role="status">
        {t('positions.showing', { n: rows.length, d: distinctPositions })}
      </Typography>

      <Legend
        items={[
          { term: t('col.position'), description: t('legend.position.position') },
          { term: t('col.token'), description: t('legend.position.token') },
          { term: t('col.protocol'), description: t('legend.position.protocol') },
          { term: t('col.network'), description: t('legend.position.network') },
          { term: t('col.category'), description: t('legend.position.category') },
          { term: t('col.link'), description: t('legend.position.link') },
          { term: t('col.liquidity'), description: t('legend.position.liquidity') },
          { term: t('col.reportedApy'), description: t('legend.position.apy', thresholds) },
          { term: t('col.status'), description: t('legend.position.status', thresholds) },
          { term: t('col.updated'), description: t('legend.position.updated') }
        ]}
      />
    </Box>
  );
}

export default function OpportunitiesPage() {
  return <DataGate>{(index) => <OpportunitiesView index={index} />}</DataGate>;
}
