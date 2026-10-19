import Box from '@mui/material/Box';
import Grid from '@mui/material/Grid';
import Typography from '@mui/material/Typography';

import { useT } from 'i18n';

export interface LegendItem {
  term: string;
  description: string;
}

// Column descriptions under a table: a definition list with its own heading.
export default function Legend({ items }: { items: LegendItem[] }) {
  const t = useT();
  return (
    <Box
      component="section"
      aria-labelledby="legend-title"
      sx={{ marginTop: 4, padding: '20px', backgroundColor: 'background.default', borderRadius: '12px' }}
    >
      <Typography variant="h4" component="h2" id="legend-title" gutterBottom sx={{ marginBottom: 2 }}>
        {t('legend.title')}
      </Typography>
      <Grid container component="dl" rowSpacing={1.5} columnSpacing={2} sx={{ marginTop: 0, marginBottom: 0 }}>
        {items.map((item) => (
          <Grid key={item.term} container size={12} columnSpacing={2}>
            <Grid component="dt" size={{ xs: 12, sm: 3, md: 2 }}>
              <Typography variant="body2" component="span" sx={{ fontWeight: 500 }}>
                {item.term}
              </Typography>
            </Grid>
            <Grid component="dd" size={{ xs: 12, sm: 9, md: 10 }} sx={{ marginLeft: 0 }}>
              <Typography variant="body2" component="span" color="text.secondary">
                {item.description}
              </Typography>
            </Grid>
          </Grid>
        ))}
      </Grid>
    </Box>
  );
}
