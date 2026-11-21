import Box from '@mui/material/Box';
import Link from '@mui/material/Link';
import Typography from '@mui/material/Typography';

import { LocaleLink, useT } from 'i18n';
import { useRwaData } from 'api/rwa';
import { useDocumentHead } from 'hooks/useDocumentHead';

// The catch-all route: an address that no page answers. The server renders it with status 404 for any such address;
// the client router shows it in the language of the URL.
export default function NotFoundPage() {
  const t = useT();
  const { index } = useRwaData();
  useDocumentHead(index?.data);
  return (
    <Box sx={{ padding: 2 }}>
      <Typography variant="h3" component="h1" gutterBottom>
        {t('notFound.page')}
      </Typography>
      <Typography variant="body1" color="text.secondary" sx={{ marginBottom: 2 }}>
        {t('notFound.pageText')}
      </Typography>
      <Link component={LocaleLink} to="/assets" underline="always">
        {t('notFound.goHome')}
      </Link>
    </Box>
  );
}
