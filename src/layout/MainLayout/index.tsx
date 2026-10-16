import { useEffect, useRef } from 'react';
import { Link as RouterLink, Outlet, useLocation } from 'react-router-dom';

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
import Breadcrumbs from 'ui-component/extended/Breadcrumbs';

import { MenuOrientation, ThemeMode } from 'config';
import useConfig from 'hooks/useConfig';
import { handlerDrawerOpen, useGetMenuMaster } from 'api/menu';
import Tabs from '@mui/material/Tabs';
import Tab, { TabProps } from '@mui/material/Tab';
import MainCard from '../../ui-component/cards/MainCard';

// ==============================|| MAIN LAYOUT ||============================== //

// icon tab style
const AntTabs = styled(Tabs)(({ theme }) => ({
  background: theme.palette.mode === ThemeMode.DARK ? theme.palette.dark[800] : theme.palette.primary.light,
  width: 'fit-content',
  borderBottom: 'none', // убираем бордер
  '& .MuiTabs-flexContainer': {
    border: '1px solid',
    borderRadius: '12px',
    borderColor: '#3F3F3F',
    height: '61px'
  },
  '& .MuiTabs-scroller': {
    borderBottom: 'none'
  },
  borderRadius: '12px',
  boxShadow: 'none',
  '& .MuiTabs-indicator': {
    backgroundColor: theme.palette.secondary.main,
    height: 0 // скрываем индикатор активного таба
  }
}));

// style constant
// tabs are links: sections open without JS and in a new tab
const AntTab = styled((props: TabProps<typeof RouterLink>) => <Tab disableRipple component={RouterLink} {...props} />)(({ theme }) => ({
  textTransform: 'none',
  minWidth: 0,
  fontWeight: theme.typography.fontWeightRegular,
  fontFamily: 'Roboto, -apple-system, BlinkMacSystemFont, "Segoe UI", "Helvetica Neue", Arial, sans-serif',
  // fontWeight: 500, // Medium
  fontSize: '16px',
  color: theme.palette.grey[100],
  '&.MuiTab-root': {
    borderRight: '1px solid #3F3F3F',
    borderColor: '#3F3F3F',
    minWidth: '125px'
  },
  '&:hover': {
    color: theme.palette.grey[500],
    opacity: 1
  },
  '&.Mui-selected': {
    color: theme.palette.background.default,
    backgroundColor: theme.palette.secondary.main,
    fontWeight: theme.typography.fontWeightMedium
  },
  '&.Mui-focusVisible': {
    backgroundColor: theme.palette.secondary.main
  }
}));
// below md every section stays visible instead of a sideways-scrolling tab strip: phones 3 + 2 grid, tablets one row
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
    fontFamily: 'Roboto, -apple-system, BlinkMacSystemFont, "Segoe UI", "Helvetica Neue", Arial, sans-serif',
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

export default function MainLayout() {
  const theme = useTheme();
  const downMD = useMediaQuery(theme.breakpoints.down('md'));

  const { borderRadius, container, miniDrawer, menuOrientation } = useConfig();
  const { menuMaster, menuMasterLoading } = useGetMenuMaster();
  const drawerOpen = menuMaster?.isDashboardDrawerOpened;

  const tabs = [
    { label: 'Underlying assets', short: 'Underlying', path: 'assets', match: ['/assets'], iconPosition: 'top' },
    { label: 'Tokenized assets', short: 'Tokenized', path: 'tokenized', match: ['/tokenized', '/tokens'], iconPosition: 'top' },
    { label: 'Issuers', short: 'Issuers', path: 'issuers', match: ['/issuers'], iconPosition: 'top' },
    { label: 'Positions', short: 'Positions', path: 'opportunities', match: ['/opportunities'], iconPosition: 'top' },
    { label: 'Summary', short: 'Summary', path: 'summary', match: ['/summary'], iconPosition: 'top' }
  ];

  const location = useLocation();
  const currentTabIndex = tabs.findIndex((tab) => tab.match.some((prefix) => location.pathname.startsWith(prefix)));

  useEffect(() => {
    window.history.scrollRestoration = 'manual';
  }, []);

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
          maxWidth={'lg'}
          sx={{
            ...(!container && { px: { xs: 0 } }),
            minHeight: 'calc(100vh - 228px)',
            display: 'flex',
            flexDirection: 'column'
          }}
        >
          {/* breadcrumb */}
          <Breadcrumbs />
          {/*<Outlet />*/}

          {/*<MainCard>*/}
          <MainCard sx={{ borderRadius: { xs: 0, sm: `${borderRadius}px` } }} contentSX={{ p: { xs: 2, sm: 3 } }}>
            {/* both navs are in the markup and CSS picks one, so the prerendered page is right at any width without JS */}
            <SectionGrid
              aria-label="Sections"
              sx={{
                display: { xs: 'grid', md: 'none' },
                gridTemplateColumns: { xs: 'repeat(6, minmax(0, 1fr))', sm: `repeat(${tabs.length}, minmax(0, 1fr))` }
              }}
            >
              {tabs.map((tab, index) => {
                const lastRow = tabs.length % 3 || 3; // items in the last row share it evenly
                return (
                  <ButtonBase
                    key={tab.path}
                    component={RouterLink}
                    to={`/${tab.path}`}
                    replace={currentTabIndex === index}
                    aria-current={currentTabIndex === index ? 'page' : undefined}
                    className="section-link"
                    sx={{ gridColumn: { xs: `span ${index < tabs.length - lastRow ? 2 : 6 / lastRow}`, sm: 'span 1' } }}
                  >
                    {tab.short}
                  </ButtonBase>
                );
              })}
            </SectionGrid>
            <Box sx={{ display: { xs: 'none', md: 'block' } }}>
              <AntTabs value={currentTabIndex === -1 ? false : currentTabIndex} centered>
                {tabs.map((tab, index) => (
                  // клик на активный таб: тот же адрес без query, без новой записи в истории
                  <AntTab wrapped={true} key={tab.path} label={tab.label} to={`/${tab.path}`} replace={currentTabIndex === index} />
                ))}
              </AntTabs>
            </Box>
            <Box sx={{ pt: 3 }}>
              <Outlet />
            </Box>
          </MainCard>
          {/*</MainCard>*/}
        </Container>
        {/* footer */}
        <Footer />
      </MainContentStyled>
    </Box>
  );
}
