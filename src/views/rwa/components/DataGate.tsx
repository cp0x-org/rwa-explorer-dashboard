import { ReactNode } from 'react';
import Box from '@mui/material/Box';
import Typography from '@mui/material/Typography';

import Loader from 'ui-component/Loader';
import { RwaIndex, useRwaData } from 'api/rwa';

// Renders children only with a fully loaded export; an HTTP failure is shown as such, never as empty data.
export default function DataGate({ children }: { children: (index: RwaIndex) => ReactNode }) {
  const { index, error, isLoading } = useRwaData();

  if (isLoading) {
    return (
      <>
        <Loader />
        <Typography variant="body2" color="text.secondary" sx={{ padding: 2 }}>
          Loading data…
        </Typography>
      </>
    );
  }

  if (error || !index) {
    return (
      <Box sx={{ padding: 2 }}>
        <Typography variant="h4" color="error" gutterBottom>
          Data unavailable
        </Typography>
        <Typography variant="body2" color="text.secondary">
          {error?.message ?? 'Export not found.'} Publish it with `uv run rwa-pipeline build` in rwa-data-pipeline.
        </Typography>
      </Box>
    );
  }

  return <>{children(index)}</>;
}

export function NotFound({ what }: { what: string }) {
  return (
    <Box sx={{ padding: 2 }}>
      <Typography variant="h5" color="error">
        {what} not found in the current data
      </Typography>
    </Box>
  );
}
