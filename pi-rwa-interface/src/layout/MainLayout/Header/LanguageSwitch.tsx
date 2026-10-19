import { Fragment } from 'react';
import { useLocation } from 'react-router-dom';

// material-ui
import Box from '@mui/material/Box';
import Link from '@mui/material/Link';

// project imports
import VisuallyHidden from 'ui-component/VisuallyHidden';
import { LOCALE_LABEL, LOCALE_NAME, LOCALES, switchLocale, useLocale, useT } from 'i18n';

// ==============================|| LANGUAGE SWITCH: EN | 中文 ||============================== //

// Plain links to the same page in the other language (query string kept): a full navigation to the prerendered page
// of that language, so <html lang>, dictionary and metadata always agree. Works without JavaScript.
export default function LanguageSwitch() {
  const { locale } = useLocale();
  const { pathname, search, hash } = useLocation();
  const t = useT();

  return (
    <Box
      component="nav"
      aria-label={t('lang.label')}
      sx={{ display: 'flex', alignItems: 'center', gap: 0.75, fontSize: '14px', fontWeight: 500, whiteSpace: 'nowrap' }}
    >
      {LOCALES.map((l, i) => {
        const active = l === locale;
        return (
          <Fragment key={l}>
            {i > 0 && (
              <Box component="span" aria-hidden="true" sx={{ color: 'text.disabled' }}>
                |
              </Box>
            )}
            <Link
              href={switchLocale(pathname, l, search, hash)}
              lang={l}
              hrefLang={l}
              aria-current={active ? 'page' : undefined}
              title={active ? undefined : t('lang.switchTo', { language: LOCALE_NAME[l] })}
              underline="none"
              sx={{
                px: 0.75,
                py: 0.5,
                borderRadius: '6px',
                color: active ? 'secondary.main' : 'text.secondary',
                fontWeight: active ? 700 : 500,
                '&:hover': { color: active ? 'secondary.main' : 'text.primary', bgcolor: 'action.hover' },
                '&:focus-visible': { outline: '2px solid', outlineColor: 'secondary.main', outlineOffset: 2 }
              }}
            >
              {/* the visible short label stays the start of the accessible name; the full name follows for screen readers */}
              {LOCALE_LABEL[l]}
              <VisuallyHidden> — {LOCALE_NAME[l]}</VisuallyHidden>
            </Link>
          </Fragment>
        );
      })}
    </Box>
  );
}
