import { Navigate, RouteObject } from 'react-router-dom';

import MainLayout from 'layout/MainLayout';
import { Locale, LOCALE_PREFIX, LocaleProvider } from 'i18n';
import AssetsPage from 'views/rwa/AssetsPage';
import AssetDetailPage from 'views/rwa/AssetDetailPage';
import TokenDetailPage from 'views/rwa/TokenDetailPage';
import TokenizedAssetsPage from 'views/rwa/TokenizedAssetsPage';
import IssuersPage from 'views/rwa/IssuersPage';
import IssuerDetailPage from 'views/rwa/IssuerDetailPage';
import OpportunitiesPage from 'views/rwa/OpportunitiesPage';
import SummaryPage from 'views/rwa/SummaryPage';
import NotFoundPage from 'views/rwa/NotFoundPage';

// ==============================|| MAIN ROUTING ||============================== //

// One page tree, mounted once per locale: `/assets` and `/zh/assets` render the same components; the LocaleProvider
// around the layout tells them which language the URL is in.
export const pageRoutes: RouteObject[] = [
  { index: true, element: <Navigate to="assets" replace /> },
  { path: 'assets', element: <AssetsPage /> },
  { path: 'assets/:ticker', element: <AssetDetailPage /> },
  { path: 'tokenized', element: <TokenizedAssetsPage /> },
  { path: 'tokens/:tokenId', element: <TokenDetailPage /> },
  { path: 'issuers', element: <IssuersPage /> },
  { path: 'issuers/:issuerId', element: <IssuerDetailPage /> },
  { path: 'opportunities', element: <OpportunitiesPage /> },
  { path: 'summary', element: <SummaryPage /> },
  { path: '*', element: <NotFoundPage /> }
];

export const localeRoute = (locale: Locale): RouteObject => ({
  path: LOCALE_PREFIX[locale] || '/',
  element: (
    <LocaleProvider locale={locale}>
      <MainLayout />
    </LocaleProvider>
  ),
  children: pageRoutes
});

const MainRoutes = localeRoute('en');

export default MainRoutes;
