import { ReactNode, useEffect } from 'react';
import { useLocation } from 'react-router-dom';
import Box from '@mui/material/Box';
import Typography from '@mui/material/Typography';

import Loader from 'ui-component/Loader';
import { RwaIndex, useRwaData } from 'api/rwa';
import { RwaExport } from 'types/rwa';
import { pageMeta } from 'utils/pageMeta';
import { MessageId, useT, Values } from 'i18n';

// Same title as the prerendered <head> of this path, kept current on client-side navigation.
export function useDocumentTitle(data?: RwaExport) {
  const { pathname } = useLocation();
  const t = useT();
  useEffect(() => {
    document.title = data ? pageMeta(pathname, data).title : `${t('notFound.page')} | cp0x`;
  }, [pathname, data, t]);
}

// Renders children only with a fully loaded export; an HTTP failure is shown as such, never as empty data.
export default function DataGate({ children }: { children: (index: RwaIndex) => ReactNode }) {
  const { index, error, isLoading } = useRwaData();
  const t = useT();
  useDocumentTitle(index?.data);

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

  if (error || !index) {
    return (
      <Box sx={{ padding: 2 }} role="alert">
        <Typography variant="h4" component="h1" color="error" gutterBottom>
          {t('data.unavailable')}
        </Typography>
        <Typography variant="body2" color="text.secondary">
          {error?.message ?? t('data.exportMissing')} {t('data.publishHint')}
        </Typography>
      </Box>
    );
  }

  return <>{children(index)}</>;
}

// An asset, token or issuer that is not in the current data: a real page with a heading, served with HTTP 404 by the
// static server (the prerender writes no file for it).
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
