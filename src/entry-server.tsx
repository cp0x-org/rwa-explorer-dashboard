import { renderToString } from 'react-dom/server';

// third party
import createCache from '@emotion/cache';
import { CacheProvider } from '@emotion/react';
import createEmotionServer from '@emotion/server/create-instance';
import { Provider } from 'react-redux';
import { createStaticHandler, createStaticRouter, StaticRouterProvider } from 'react-router-dom';

// project imports
import App from 'App';
import routes from 'routes';
import { RwaDataContext } from 'api/rwa';
import { ConfigProvider } from 'contexts/ConfigContext';
import { store } from 'store';
import { RwaExport } from 'types/rwa';
import { intlFor, pageMeta, PageMeta } from 'utils/pageMeta';
import { headLinks, jsonLd } from 'utils/seo';
import { ENGLISH, Locale, LOCALES, localizePath, registerMessages } from 'i18n';
import zh from 'i18n/messages/zh';

// Both dictionaries are in the Node bundle: every page is rendered in every language.
registerMessages('zh-CN', zh);

// ==============================|| BUILD-TIME PRERENDER ||============================== //

// Public address of the site (VITE_SITE_URL in .env.production): canonical links, sitemap.xml, robots.txt.
export const siteUrl: string | undefined = import.meta.env.VITE_SITE_URL?.replace(/\/+$/, '');

export { LOCALES };

// A path that no page answers: rendered once as dist/404.html, the page `serve` returns for unknown paths.
export const NOT_FOUND_PATH = '/__not_found__';

// Every page worth opening directly, locale-free: the sections and one page per asset, tokenized asset and issuer.
export function pagePaths(data: RwaExport): string[] {
  return [
    '/assets',
    '/tokenized',
    '/issuers',
    '/opportunities',
    '/summary',
    ...data.underlyings.map((u) => `/assets/${encodeURIComponent(u.ticker)}`),
    ...data.tokens.map((t) => `/tokens/${t.id}`),
    ...data.issuers.map((i) => `/issuers/${encodeURIComponent(i.id)}`)
  ];
}

// The same pages in every locale: `/assets`, `/zh/assets`, ...
export function prerenderPaths(data: RwaExport): string[] {
  const paths = pagePaths(data);
  return LOCALES.flatMap((locale) => paths.map((p) => localizePath(p, locale)));
}

export interface RenderedPage {
  html: string;
  styles: string;
  meta: PageMeta;
  lang: Locale;
  noscript: string;
  head: ReturnType<typeof headLinks> | null;
  jsonLd: object | null;
}

// HTML of one page with the data already in place, its critical CSS and <head> metadata. Used by scripts/prerender.mjs.
export async function render(path: string, data: RwaExport): Promise<RenderedPage> {
  const handler = createStaticHandler(routes, { basename: import.meta.env.VITE_APP_BASE_NAME });
  const context = await handler.query(new Request(new URL(path, 'http://prerender.local')));
  if (context instanceof Response) throw new Error(`${path}: redirects, nothing to prerender`);
  const router = createStaticRouter(handler.dataRoutes, context);

  // a fresh style cache per page, so each page gets all the styles it uses
  const cache = createCache({ key: 'css' });
  const { extractCriticalToChunks, constructStyleTagsFromChunks } = createEmotionServer(cache);

  const html = renderToString(
    <CacheProvider value={cache}>
      <Provider store={store}>
        <ConfigProvider>
          <RwaDataContext value={data}>
            <App router={<StaticRouterProvider router={router} context={context} hydrate={false} />} />
          </RwaDataContext>
        </ConfigProvider>
      </Provider>
    </CacheProvider>
  );

  const meta = pageMeta(path, data);
  const intl = intlFor(meta.locale);
  return {
    html,
    styles: constructStyleTagsFromChunks(extractCriticalToChunks(html)),
    meta,
    lang: meta.locale,
    noscript: intl.formatMessage({ id: 'site.noscript', defaultMessage: ENGLISH['site.noscript'] }),
    head: siteUrl && meta.found ? headLinks(meta, siteUrl) : null,
    jsonLd: siteUrl ? jsonLd(meta, data, siteUrl) : null
  };
}
