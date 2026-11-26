import { createRoot, hydrateRoot } from 'react-dom/client';

// third party
import { Provider } from 'react-redux';
import { SWRConfig } from 'swr';
import { createBrowserRouter, RouterProvider } from 'react-router-dom';

// project imports
import App from 'App';
import routes from 'routes';
import { loadRwaBuild, loadRwaData, RwaDataContext } from 'api/rwa';
import { RwaExport } from 'types/rwa';
import { store } from 'store';
import * as serviceWorker from 'serviceWorker';
import reportWebVitals from 'reportWebVitals';
import { ConfigProvider } from 'contexts/ConfigContext';
import { loadMessages, localeFromPath } from 'i18n';

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

// API answers the server rendered the page with (server/render.mjs embeds them): the first client render uses the same ones
const readFallback = (): Record<string, unknown> => {
  try {
    return JSON.parse(document.getElementById('swr-fallback')?.textContent ?? '{}');
  } catch {
    return {};
  }
};
const fallback = readFallback();

// same provider tree as the server render in entry-server.tsx, except for the router
const app = (data?: RwaExport) => (
  <Provider store={store}>
    <ConfigProvider>
      <RwaDataContext value={data}>
        <SWRConfig value={{ fallback }}>
          <App router={<RouterProvider router={router} />} />
        </SWRConfig>
      </RwaDataContext>
    </ConfigProvider>
  </Provider>
);

// The language is in the URL (/zh/...): its dictionary must be in place before the first render, so the hydrated
// markup matches the server-rendered page and nothing flickers.
const { pathname, search } = router.state.location;
const messages = loadMessages(localeFromPath(pathname));

declare global {
  interface Window {
    __RWA_BUILD__?: number; // build id of the export the page was rendered from (read by the e2e tests)
  }
}

const RETRY_MS = [5_000, 10_000, 20_000, 40_000, 60_000];

if (container.hasChildNodes()) {
  // A page rendered by the server (server/render.mjs) stays on screen until the data is here. The server names the build
  // it rendered from in <meta name="rwa-build">: the client loads exactly that build (immutable, and preloaded by the
  // page), so the data is the one behind the markup. React then hydrates the page if it was rendered for this exact
  // location. If that build is gone, the latest export is used and the app renders anew in place of the server markup
  // (the root path and a query string do the same). If no export can be loaded at all, the server markup stays untouched
  // and the load is retried with a growing delay: a failing API must not replace a readable page with an error.
  const pinned = document.querySelector('meta[name="rwa-build"]')?.getAttribute('content');
  const load = () => (pinned ? loadRwaBuild(pinned).catch(() => loadRwaData()) : loadRwaData());
  const start = async () => {
    for (let attempt = 0; ; attempt++) {
      try {
        const [data] = await Promise.all([load(), messages]);
        window.__RWA_BUILD__ = data.build_id;
        const samePage = (!pinned || String(data.build_id) === pinned) && !search && container.dataset.path === pathname;
        if (samePage) hydrateRoot(container, app(data));
        else createRoot(container).render(app(data));
        return;
      } catch {
        await new Promise((resolve) => setTimeout(resolve, RETRY_MS[Math.min(attempt, RETRY_MS.length - 1)]));
      }
    }
  };
  start();
} else {
  messages.then(() => createRoot(container).render(app()));
}

// If you want your app to work offline and load faster, you can change
// unregister() to register() below. Note this comes with some pitfalls.
// Learn more about service workers: https://bit.ly/CRA-PWA
serviceWorker.unregister();

// If you want to start measuring performance in your app, pass a function
// to log results (for example: reportWebVitals(console.log))
// or send to an analytics endpoint. Learn more: https://bit.ly/CRA-vitals
reportWebVitals();
