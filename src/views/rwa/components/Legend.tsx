import Box from '@mui/material/Box';
import Grid from '@mui/material/Grid';
import Typography from '@mui/material/Typography';

export interface LegendItem {
  term: string;
  description: string;
}

// Column descriptions under a table.
export default function Legend({ items }: { items: LegendItem[] }) {
  return (
    <Box sx={{ marginTop: 4, padding: '20px', backgroundColor: 'background.default', borderRadius: '12px' }}>
      <Typography variant="h4" gutterBottom sx={{ marginBottom: 2 }}>
        Legend
      </Typography>
      <Grid container rowSpacing={1.5} columnSpacing={2}>
        {items.map((item) => (
          <Grid key={item.term} container size={12} columnSpacing={2}>
            <Grid size={{ xs: 12, sm: 3, md: 2 }}>
              <Typography variant="body2" sx={{ fontWeight: 500 }}>
                {item.term}
              </Typography>
            </Grid>
            <Grid size={{ xs: 12, sm: 9, md: 10 }}>
              <Typography variant="body2" color="text.secondary">
                {item.description}
              </Typography>
            </Grid>
          </Grid>
        ))}
      </Grid>
    </Box>
  );
}
