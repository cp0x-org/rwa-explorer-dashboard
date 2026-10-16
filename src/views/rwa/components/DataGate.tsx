import { ReactNode, useEffect } from 'react';
import { useLocation } from 'react-router-dom';
import Box from '@mui/material/Box';
import Typography from '@mui/material/Typography';

import Loader from 'ui-component/Loader';
import { RwaIndex, useRwaData } from 'api/rwa';
import { pageMeta } from 'utils/pageMeta';

// Renders children only with a fully loaded export; an HTTP failure is shown as such, never as empty data.
export default function DataGate({ children }: { children: (index: RwaIndex) => ReactNode }) {
  const { index, error, isLoading } = useRwaData();
  const { pathname } = useLocation();

  // same title as the prerendered <head> of this path, kept current on client-side navigation
  useEffect(() => {
    if (index) document.title = pageMeta(pathname, index.data).title;
  }, [pathname, index]);

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
