import MainLayout from 'layout/MainLayout';
import { Navigate } from 'react-router';
import AssetsPage from 'views/rwa/AssetsPage';
import AssetDetailPage from 'views/rwa/AssetDetailPage';
import TokenDetailPage from 'views/rwa/TokenDetailPage';
import TokenizedAssetsPage from 'views/rwa/TokenizedAssetsPage';
import IssuersPage from 'views/rwa/IssuersPage';
import IssuerDetailPage from 'views/rwa/IssuerDetailPage';
import OpportunitiesPage from 'views/rwa/OpportunitiesPage';
import SummaryPage from 'views/rwa/SummaryPage';

// ==============================|| MAIN ROUTING ||============================== //

const MainRoutes = {
  path: '/',
  element: <MainLayout />,
  children: [
    {
      index: true,
      element: <Navigate to="assets" replace />
    },
    {
      path: '/assets',
      element: <AssetsPage />
    },
    {
      path: '/assets/:ticker',
      element: <AssetDetailPage />
    },
    {
      path: '/tokenized',
      element: <TokenizedAssetsPage />
    },
    {
      path: '/tokens/:tokenId',
      element: <TokenDetailPage />
    },
    {
      path: '/issuers',
      element: <IssuersPage />
    },
    {
      path: '/issuers/:issuerId',
      element: <IssuerDetailPage />
    },
    {
      path: '/opportunities',
      element: <OpportunitiesPage />
    },
    {
      path: '/summary',
      element: <SummaryPage />
    }
  ]
};

export default MainRoutes;
