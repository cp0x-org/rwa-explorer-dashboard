import { useMemo, useState } from 'react';
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
import DataGate from './components/DataGate';
import MethodNote from './components/MethodNote';
import MultiFilter from './components/MultiFilter';
import OpportunityTable from './components/OpportunityTable';

type Status = 'eligible' | 'confirmed' | 'all';
type LinkFilter = 'all' | 'direct' | 'indirect';

const uniqueSorted = (values: string[]) => Array.from(new Set(values)).sort();

function OpportunitiesView({ index }: { index: RwaIndex }) {
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
        index.data.tokens.filter((t) => t.kind !== 'derivative' && t.kind !== 'unclassified'),
        true
      ),
    [index]
  );
  const platformNames = useMemo(() => new Map(allRows.map((r) => [r.opportunity.platform, r.opportunity.platform_name])), [allRows]);
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
      <Typography variant="h3" gutterBottom>
        DeFi Positions
      </Typography>
      <MethodNote data={index.data} />

      <Grid container spacing={2} sx={{ marginBottom: 2 }}>
        <Grid size={{ xs: 12, md: 4 }}>
          <MultiFilter
            label="Filter By Underlying"
            options={index.data.underlyings.map((u) => u.ticker)}
            value={underlyings}
            onChange={setUnderlyings}
          />
        </Grid>
        <Grid size={{ xs: 12, md: 4 }}>
          <MultiFilter
            label="Filter By Issuer"
            options={index.data.issuers.map((i) => i.id)}
            value={issuers}
            onChange={setIssuers}
            getLabel={(id) => issuerNames.get(id) ?? id}
          />
        </Grid>
        <Grid size={{ xs: 12, md: 4 }}>
          <MultiFilter
            label="Filter By Network"
            options={uniqueSorted(allRows.map((r) => r.opportunity.network))}
            value={networks}
            onChange={setNetworks}
            getLabel={networkName}
          />
        </Grid>
        <Grid size={{ xs: 12, md: 4 }}>
          <MultiFilter
            label="Filter By Category"
            options={uniqueSorted(allRows.map((r) => r.opportunity.category))}
            value={categories}
            onChange={setCategories}
            getLabel={categoryLabel}
          />
        </Grid>
        <Grid size={{ xs: 12, md: 4 }}>
          <MultiFilter
            label="Filter By Protocol"
            options={uniqueSorted(allRows.map((r) => r.opportunity.platform))}
            value={platforms}
            onChange={setPlatforms}
            getLabel={(p) => platformNames.get(p) ?? p}
          />
        </Grid>
        <Grid size={{ xs: 12, md: 4 }}>
          <TextField label="Filter By Name" value={search} onChange={(e) => setSearch(e.target.value)} size="small" fullWidth />
        </Grid>
        <Grid size={{ xs: 6, md: 4 }}>
          <FormControl size="small" fullWidth>
            <InputLabel>Status</InputLabel>
            <Select value={status} label="Status" onChange={(e) => setStatus(e.target.value as Status)}>
              <MenuItem value="eligible">Eligible</MenuItem>
              <MenuItem value="confirmed">All confirmed (raw)</MenuItem>
              <MenuItem value="all">Incl. unconfirmed symbol matches</MenuItem>
            </Select>
          </FormControl>
        </Grid>
        <Grid size={{ xs: 6, md: 4 }}>
          <FormControl size="small" fullWidth>
            <InputLabel>Link</InputLabel>
            <Select value={link} label="Link" onChange={(e) => setLink(e.target.value as LinkFilter)}>
              <MenuItem value="all">Direct and indirect</MenuItem>
              <MenuItem value="direct">Direct</MenuItem>
              <MenuItem value="indirect">Indirect</MenuItem>
            </Select>
          </FormControl>
        </Grid>
      </Grid>

      <OpportunityTable index={index} rows={rows} showToken defaultRows={25} />
      <Typography variant="body2" color="text.secondary">
        Showing {rows.length} token–position links ({distinctPositions} distinct positions)
      </Typography>
    </Box>
  );
}

export default function OpportunitiesPage() {
  return <DataGate>{(index) => <OpportunitiesView index={index} />}</DataGate>;
}
