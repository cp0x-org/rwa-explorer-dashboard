import { useEffect, useRef } from 'react';
import { Outlet, useLocation } from 'react-router-dom';

// material-ui
import { styled, useTheme } from '@mui/material/styles';
import useMediaQuery from '@mui/material/useMediaQuery';
import AppBar from '@mui/material/AppBar';
import Container from '@mui/material/Container';
import Toolbar from '@mui/material/Toolbar';
import Box from '@mui/material/Box';
import ButtonBase from '@mui/material/ButtonBase';

// project imports
import Footer from './Footer';
import Header from './Header';
import MainContentStyled from './MainContentStyled';
import Loader from 'ui-component/Loader';
import MainCard from 'ui-component/cards/MainCard';

import { MenuOrientation, ThemeMode } from 'config';
import useConfig from 'hooks/useConfig';
import { handlerDrawerOpen, useGetMenuMaster } from 'api/menu';
import { LocaleLink, MessageId, stripLocale, useLocale, useT } from 'i18n';
import { withCjkFallback } from 'themes/typography';

// ==============================|| MAIN LAYOUT ||============================== //

export const MAIN_ID = 'main';

const NAV_FONT = withCjkFallback('Roboto, -apple-system, BlinkMacSystemFont, "Segoe UI", "Helvetica Neue", Arial, sans-serif');

// Desktop section navigation: a row of link "tabs" (same look as the template's tab strip, but a real <nav> of links:
// each section is a page, not a tab panel).
const SectionTabs = styled('nav')(({ theme }) => ({
  display: 'flex',
  justifyContent: 'flex-start',
  '& ul': {
    display: 'flex',
    listStyle: 'none',
    margin: 0,
    padding: 0,
    height: 61,
    border: '1px solid #3F3F3F',
    borderRadius: '12px',
    overflow: 'hidden',
    background: theme.palette.mode === ThemeMode.DARK ? theme.palette.dark[800] : theme.palette.primary.light
  },
  '& li': { display: 'flex' },
  '& li + li .section-tab': { borderLeft: '1px solid #3F3F3F' },
  '& .section-tab': {
    minWidth: 125,
    padding: theme.spacing(0, 2),
    fontFamily: NAV_FONT,
    fontSize: '16px',
    fontWeight: theme.typography.fontWeightRegular,
    color: theme.palette.grey[100],
    textDecoration: 'none',
    '&:hover': { color: theme.palette.grey[500] },
    '&[aria-current="page"]': {
      color: theme.palette.background.default,
      backgroundColor: theme.palette.secondary.main,
      fontWeight: theme.typography.fontWeightMedium
    },
    '&.Mui-focusVisible': { outline: `2px solid ${theme.palette.secondary.main}`, outlineOffset: -2 }
  }
}));

// Below md every section stays visible instead of a sideways-scrolling strip: phones 3 + 2 grid, tablets one row.
const SectionGrid = styled('nav')(({ theme }) => ({
  display: 'grid',
  gap: '1px', // hairline dividers: the grid background shows between the cells
  backgroundColor: '#3F3F3F',
  border: '1px solid #3F3F3F',
  borderRadius: '12px',
  overflow: 'hidden',
  '& .section-link': {
    minHeight: 46,
    padding: theme.spacing(0.5, 1),
    fontFamily: NAV_FONT,
    fontSize: '15px',
    lineHeight: 1.2,
    textAlign: 'center',
    color: theme.palette.grey[100],
    backgroundColor: theme.palette.mode === ThemeMode.DARK ? theme.palette.dark[800] : theme.palette.primary.light,
    '&[aria-current="page"]': {
      color: theme.palette.background.default,
      backgroundColor: theme.palette.secondary.main,
      fontWeight: theme.typography.fontWeightMedium
    },
    '&.Mui-focusVisible': { outline: `2px solid ${theme.palette.secondary.main}`, outlineOffset: -2 }
  }
}));

interface Section {
  label: MessageId;
  short: MessageId;
  path: string;
  match: string[];
}

export const SECTIONS: Section[] = [
  { label: 'nav.assets', short: 'nav.assets.short', path: '/assets', match: ['/assets'] },
  { label: 'nav.tokenized', short: 'nav.tokenized.short', path: '/tokenized', match: ['/tokenized', '/tokens'] },
  { label: 'nav.issuers', short: 'nav.issuers.short', path: '/issuers', match: ['/issuers'] },
  { label: 'nav.positions', short: 'nav.positions.short', path: '/opportunities', match: ['/opportunities'] },
  { label: 'nav.summary', short: 'nav.summary.short', path: '/summary', match: ['/summary'] }
];

export default function MainLayout() {
  const theme = useTheme();
  const downMD = useMediaQuery(theme.breakpoints.down('md'));
  const t = useT();
  const { locale } = useLocale();

  const { borderRadius, container, miniDrawer, menuOrientation } = useConfig();
  const { menuMaster, menuMasterLoading } = useGetMenuMaster();
  const drawerOpen = menuMaster?.isDashboardDrawerOpened;

  const location = useLocation();
  const pathname = stripLocale(location.pathname);
  const current = SECTIONS.findIndex((s) => s.match.some((prefix) => pathname === prefix || pathname.startsWith(`${prefix}/`)));

  useEffect(() => {
    window.history.scrollRestoration = 'manual';
  }, []);

  // the language of the document follows the URL (matters when the 404 shell is rendered anew in another language)
  useEffect(() => {
    document.documentElement.lang = locale;
    document.documentElement.dataset.hydrated = 'true'; // after the first commit: tests wait for this before interacting
  }, [locale]);

  // top of the page on navigation, but not on the first render: a prerendered page may already be scrolled when React takes over
  const firstPath = useRef(true);
  useEffect(() => {
    if (firstPath.current) {
      firstPath.current = false;
      return;
    }
    window.scrollTo(0, 0);
  }, [location.pathname]);

  useEffect(() => {
    handlerDrawerOpen(!miniDrawer);
  }, [miniDrawer]);

  useEffect(() => {
    downMD && handlerDrawerOpen(false);
  }, [downMD]);

  if (menuMasterLoading) return <Loader />;

  const sectionLink = (section: Section, index: number) => ({
    to: section.path,
    replace: current === index, // click on the active section: same address without query, no new history entry
    'aria-current': current === index ? ('page' as const) : undefined
  });

  return (
    <Box sx={{ display: 'flex' }}>
      {/* header */}
      <AppBar enableColorOnDark position="fixed" color="inherit" elevation={0} sx={{ bgcolor: 'background.default' }}>
        <Toolbar sx={{ p: menuOrientation === MenuOrientation.HORIZONTAL ? { xs: 2, md: 1.25 } : 2 }}>
          <Header />
        </Toolbar>
      </AppBar>

      {/* main content */}
      <MainContentStyled {...{ borderRadius, menuOrientation, open: drawerOpen, marginTop: 80 }}>
        <Container
          component="main"
          id={MAIN_ID}
          maxWidth="lg"
          sx={{
            ...(!container && { px: { xs: 0 } }),
            minHeight: 'calc(100vh - 228px)',
            display: 'flex',
            flexDirection: 'column',
            '&:focus': { outline: 'none' }
          }}
        >
          {/* overflow: visible — Card clips by default, which would make it the sticky container and keep table headers from sticking under the app bar */}
          <MainCard sx={{ borderRadius: { xs: 0, sm: `${borderRadius}px` }, overflow: 'visible' }} contentSX={{ p: { xs: 2, sm: 3 } }}>
            {/* both navs are in the markup and CSS picks one (the hidden one is display:none, so it is not in the
                accessibility tree); the prerendered page is right at any width without JS */}
            <SectionGrid
              aria-label={t('nav.sections')}
              sx={{
                display: { xs: 'grid', md: 'none' },
                gridTemplateColumns: { xs: 'repeat(6, minmax(0, 1fr))', sm: `repeat(${SECTIONS.length}, minmax(0, 1fr))` }
              }}
            >
              {SECTIONS.map((section, index) => {
                const lastRow = SECTIONS.length % 3 || 3; // items in the last row share it evenly
                return (
                  <ButtonBase
                    key={section.path}
                    component={LocaleLink}
                    {...sectionLink(section, index)}
                    className="section-link"
                    sx={{ gridColumn: { xs: `span ${index < SECTIONS.length - lastRow ? 2 : 6 / lastRow}`, sm: 'span 1' } }}
                  >
                    {t(section.short)}
                  </ButtonBase>
                );
              })}
            </SectionGrid>
            <SectionTabs aria-label={t('nav.sections')} sx={{ display: { xs: 'none', md: 'flex' } }}>
              <ul>
                {SECTIONS.map((section, index) => (
                  <li key={section.path}>
                    <ButtonBase component={LocaleLink} {...sectionLink(section, index)} className="section-tab">
                      {t(section.label)}
                    </ButtonBase>
                  </li>
                ))}
              </ul>
            </SectionTabs>
            <Box sx={{ pt: 3 }}>
              <Outlet />
            </Box>
          </MainCard>
        </Container>
        {/* footer */}
        <Footer />
      </MainContentStyled>
    </Box>
  );
}
