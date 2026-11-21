import { ReactNode } from 'react';
import Box from '@mui/material/Box';
import Typography from '@mui/material/Typography';

import Loader from 'ui-component/Loader';
import { ApiError, RwaIndex, useRwaData } from 'api/rwa';
import { useDocumentHead } from 'hooks/useDocumentHead';
import { MessageId, useT, Values } from 'i18n';

// Renders children only with a fully loaded export; an HTTP failure is shown as such, never as empty data.
export default function DataGate({ children }: { children: (index: RwaIndex) => ReactNode }) {
  const { index, error, isLoading } = useRwaData();
  const t = useT();
  useDocumentHead(index?.data);

  if (isLoading) {
    return (
      <>
        <Loader />
        <Typography variant="body2" color="text.secondary" role="status" aria-live="polite" sx={{ padding: 2 }}>
          {t('data.loading')}
        </Typography>
      </>
    );
  }

  // data already on screen stays when a later revalidation fails: only a page without data shows the error
  if (!index) {
    return (
      <Box sx={{ padding: 2 }} role="alert">
        <Typography variant="h4" component="h1" color="error" gutterBottom>
          {t('data.unavailable')}
        </Typography>
        <Typography variant="body2" color="text.secondary">
          {error instanceof ApiError ? `${t('data.apiError', { status: error.status })} ` : ''}
          {t('data.retryHint')}
        </Typography>
      </Box>
    );
  }

  return <>{children(index)}</>;
}

// An asset, token or issuer that is not in the current data: a real page with a heading, served with HTTP 404 by the
// server (server/render.mjs, `meta.found` is false).
export function NotFound({ id, values }: { id: MessageId; values?: Values }) {
  const t = useT();
  return (
    <Box sx={{ padding: 2 }}>
      <Typography variant="h5" component="h1" color="error">
        {t(id, values)}
      </Typography>
    </Box>
  );
}
