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
import { HeadLinks, headLinks, jsonLd } from 'utils/seo';
import { ENGLISH, Locale, LOCALES, localizePath, registerMessages } from 'i18n';
import zh from 'i18n/messages/zh';

// Both dictionaries are in the Node bundle: every page is rendered in every language.
registerMessages('zh-CN', zh);

// ==============================|| SERVER RENDER ||============================== //

// Public address of the site at build time (VITE_SITE_URL in .env.production): canonical links, sitemap.xml, robots.txt.
// server/ overrides it with its SITE_URL variable (the `site` argument of render() and sitemapHeads()).
export const siteUrl: string | undefined = import.meta.env.VITE_SITE_URL?.replace(/\/+$/, '');

export { LOCALES };

// Strings of the static "Data unavailable" page (server/render.mjs), the same as the data gate shows in the app.
const unavailable = (lang: string, m: Record<string, string>) => ({
  lang,
  heading: m['data.unavailable'],
  text: m['data.retryHint'],
  reload: m['data.reload']
});
export const UNAVAILABLE = { en: unavailable('en', ENGLISH), zh: unavailable('zh-CN', zh) };

// A path that no page answers: the server renders it for an address it cannot render as such (malformed escapes).
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

// canonical and hreflang links of every page in every language (what sitemap.xml lists), without rendering the pages.
export function sitemapHeads(data: RwaExport, site: string | undefined = siteUrl): HeadLinks[] {
  if (!site) return [];
  return prerenderPaths(data).map((path) => headLinks(pageMeta(path, data), site));
}

// HTML of one page with the data already in place, its critical CSS and <head> metadata. Used by server/render.mjs.
// `meta.found` is false for an address that is not a page (unknown ticker, token or issuer, unknown path): the server
// answers it with HTTP 404.
export async function render(path: string, data: RwaExport, site: string | undefined = siteUrl): Promise<RenderedPage> {
  const handler = createStaticHandler(routes, { basename: import.meta.env.VITE_APP_BASE_NAME });
  const context = await handler.query(new Request(new URL(path, 'http://ssr.local')));
  if (context instanceof Response) throw new Error(`${path}: redirects, nothing to render`);
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
    head: site && meta.found ? headLinks(meta, site) : null,
    jsonLd: site ? jsonLd(meta, data, site) : null
  };
}
