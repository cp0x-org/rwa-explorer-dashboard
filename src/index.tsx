import { createRoot, hydrateRoot } from 'react-dom/client';

// third party
import { Provider } from 'react-redux';
import { createBrowserRouter, RouterProvider } from 'react-router-dom';

// project imports
import App from 'App';
import routes from 'routes';
import { loadRwaData, RwaDataContext } from 'api/rwa';
import { RwaExport } from 'types/rwa';
import { store } from 'store';
import * as serviceWorker from 'serviceWorker';
import reportWebVitals from 'reportWebVitals';
import { ConfigProvider } from 'contexts/ConfigContext';

// style + assets
import 'assets/scss/style.scss';

// google-fonts
import '@fontsource/roboto/400.css';
import '@fontsource/roboto/500.css';
import '@fontsource/roboto/300.css';
import '@fontsource/roboto/700.css';

import '@fontsource/inter/400.css';
import '@fontsource/inter/500.css';
import '@fontsource/inter/600.css';
import '@fontsource/inter/700.css';

import '@fontsource/poppins/400.css';
import '@fontsource/poppins/500.css';
import '@fontsource/poppins/600.css';
import '@fontsource/poppins/700.css';

// ==============================|| REACT DOM RENDER ||============================== //

const container = document.getElementById('root')!;
const router = createBrowserRouter(routes, {
  basename: import.meta.env.VITE_APP_BASE_NAME
});

// same provider tree as the prerender in entry-server.tsx, except for the router
const app = (data?: RwaExport) => (
  <Provider store={store}>
    <ConfigProvider>
      <RwaDataContext value={data}>
        <App router={<RouterProvider router={router} />} />
      </RwaDataContext>
    </ConfigProvider>
  </Provider>
);

if (container.hasChildNodes()) {
  // A page prerendered at build time (scripts/prerender.mjs) stays on screen until the data is here. Then React hydrates
  // it if it was rendered for this exact location; otherwise (the root path, a query string, an unknown path served
  // with the fallback page) it renders the app anew in place of the static markup.
  loadRwaData()
    .catch(() => undefined) // the data gate shows the error after its own retry
    .then((data) => {
      const { pathname, search } = router.state.location;
      if (data && !search && container.dataset.path === pathname) hydrateRoot(container, app(data));
      else createRoot(container).render(app(data));
    });
} else {
  createRoot(container).render(app());
}

// If you want your app to work offline and load faster, you can change
// unregister() to register() below. Note this comes with some pitfalls.
// Learn more about service workers: https://bit.ly/CRA-PWA
serviceWorker.unregister();

// If you want to start measuring performance in your app, pass a function
// to log results (for example: reportWebVitals(console.log))
// or send to an analytics endpoint. Learn more: https://bit.ly/CRA-vitals
reportWebVitals();
