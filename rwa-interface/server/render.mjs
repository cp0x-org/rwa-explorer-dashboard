// HTML pages: dist/index.html (the Vite build) as a template, filled per request with the markup, <head> metadata and
// critical CSS from dist-ssr/entry-server.mjs, for the export in the data cache.
import { createHash } from 'node:crypto';
import { readFile } from 'node:fs/promises';
import path from 'node:path';
import { pathToFileURL } from 'node:url';

const escapeHtml = (s) => s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
// "</script>" inside JSON-LD would end the script element
const escapeJson = (obj) => JSON.stringify(obj).replace(/</g, '\\u003c');

const CACHE_PAGES = 100; // rendered pages kept for the current build (a page is about 300 KB)

export async function createRenderer({ root, siteUrl }) {
  const entry = await import(pathToFileURL(path.join(root, 'dist-ssr', 'entry-server.mjs')).href);
  const { render, prerenderPaths, sitemapHeads, NOT_FOUND_PATH, LOCALES, UNAVAILABLE } = entry;
  const site = siteUrl ?? entry.siteUrl;
  const template = await readFile(path.join(root, 'dist', 'index.html'), 'utf8');
  // identifies the frontend build (the template names the hashed bundles) and the site address: part of every page's ETag,
  // so a new image never answers 304 to a browser that holds HTML pointing at deleted bundles
  const version = createHash('sha1').update(template).update(site ?? '').digest('base64url');

  // The preload is same-origin like the client's pinned fetch (VITE_API_URL empty). With VITE_API_URL set the client calls
  // the absolute URL, so the preload would go unused: that setup is for the dev server, which renders no server markup.
  // <head> additions of one page: build pin, canonical, hreflang pairs, Open Graph URL and locale, robots, JSON-LD.
  const headTags = (page, buildId) => {
    const tags = [
      `<meta name="rwa-build" content="${buildId}" />`,
      `<link rel="preload" as="fetch" href="/api/v1/export?build=${buildId}" crossorigin />`
    ];
    if (page.head) {
      const { canonical, hreflang, ogLocale, ogLocaleAlternates } = page.head;
      tags.push(`<link rel="canonical" href="${escapeHtml(canonical)}" />`);
      hreflang.forEach(({ lang, href }) => tags.push(`<link rel="alternate" hreflang="${lang}" href="${escapeHtml(href)}" />`));
      tags.push(`<meta property="og:url" content="${escapeHtml(canonical)}" />`);
      tags.push(`<meta property="og:locale" content="${ogLocale}" />`);
      ogLocaleAlternates.forEach((l) => tags.push(`<meta property="og:locale:alternate" content="${l}" />`));
    }
    if (!page.meta.found) tags.push('<meta name="robots" content="noindex" />');
    if (page.jsonLd) tags.push(`<script type="application/ld+json">${escapeJson(page.jsonLd)}</script>`);
    return tags.join('\n        ');
  };

  // replacements use functions: rendered HTML contains "$" (prices), which a replacement string would treat as a pattern
  const slots = [
    [/<html lang="[^"]*">/, ({ lang }) => `<html lang="${lang}">`],
    [/<title>[^<]*<\/title>/, ({ meta }) => `<title>${escapeHtml(meta.title)}</title>`],
    [
      /(<meta (?:name|property)="(?:title|og:title|twitter:title)" content=")[^"]*"/g,
      ({ meta }, open) => `${open}${escapeHtml(meta.title)}"`
    ],
    [
      /(<meta (?:name|property)="(?:description|og:description|twitter:description)" content=")[^"]*"/g,
      ({ meta }, open) => `${open}${escapeHtml(meta.description)}"`
    ],
    [/(<meta name="keywords" content=")[^"]*"/, ({ meta }, open) => `${open}${escapeHtml(meta.keywords)}"`],
    ['<!--ssr-head-->', (page, _open, _route, buildId) => headTags(page, buildId) + page.styles],
    [/<noscript>[^<]*<\/noscript>/, ({ noscript }) => `<noscript>${escapeHtml(noscript)}</noscript>`],
    ['<div id="root"></div>', ({ html }, _open, route) => `<div id="root" data-path="${escapeHtml(route)}">${html}</div>`]
  ];
  for (const [pattern] of slots) {
    if (typeof pattern === 'string' ? !template.includes(pattern) : template.search(pattern) === -1) {
      throw new Error(`dist/index.html has no ${pattern}: run \`pnpm build\` (the template was not built from index.html)`);
    }
  }
  const fill = (page, route, buildId) =>
    slots.reduce((html, [pattern, value]) => html.replace(pattern, (_match, open) => value(page, open, route, buildId)), template);

  // rendered pages of the current build only
  let cacheBuild = null;
  const cache = new Map();

  // "/" and "/zh" show the first section: they are rendered as /assets (the client then redirects to it)
  const routeOf = (pathname) => (pathname === '/' ? '/assets' : pathname === '/zh' ? '/zh/assets' : pathname);
  const prefix = (pathname) => (/^\/zh(\/|$)/.test(pathname) ? '/zh' : '');

  return {
    // { status, html } of a page; 404 when it is not a page of the current data
    async page(pathname, data) {
      if (cacheBuild !== data.build_id) {
        cache.clear();
        cacheBuild = data.build_id;
      }
      const hit = cache.get(pathname);
      if (hit) return hit;
      let result;
      try {
        const route = routeOf(pathname);
        const page = await render(route, data, site);
        result = { status: page.meta.found ? 200 : 404, html: fill(page, route, data.build_id) };
      } catch (err) {
        // only a malformed percent escape is the caller's mistake (404); any other error is ours and becomes a 500
        if (!(err instanceof URIError)) throw err;
        const route = prefix(pathname) + NOT_FOUND_PATH;
        result = { status: 404, html: fill(await render(route, data, site), route, data.build_id) };
      }
      if (cache.size >= CACHE_PAGES) cache.delete(cache.keys().next().value);
      cache.set(pathname, result);
      return result;
    },

    // pages of both languages with their hreflang alternates, for sitemap.xml
    sitemapHeads: (data) => sitemapHeads(data, site),
    pageCount: (data) => prerenderPaths(data).length,
    site,
    version,
    unavailable: (pathname) => unavailablePage(pathname, UNAVAILABLE),
    locales: LOCALES
  };
}

// Static page for the time before the first export is loaded: no data, no app, but a correct status and language.
// The strings are the ones of the app's data gate (entry-server.tsx exports them from the dictionaries).
function unavailablePage(pathname, strings) {
  const t = /^\/zh(\/|$)/.test(pathname) ? strings.zh : strings.en;
  return (
    `<!doctype html>\n<html lang="${t.lang}">\n<head>\n<meta charset="utf-8" />\n` +
    `<meta name="viewport" content="width=device-width, initial-scale=1" />\n<meta name="robots" content="noindex" />\n` +
    `<title>${escapeHtml(t.heading)} | RWA Explorer</title>\n</head>\n` +
    `<body style="font-family: system-ui, sans-serif; margin: 2rem">\n<main>\n<h1>${escapeHtml(t.heading)}</h1>\n<p>${escapeHtml(t.text)}</p>\n` +
    `<p><a href="${escapeHtml(pathname)}">${escapeHtml(t.reload)}</a></p>\n</main>\n</body>\n</html>\n`
  );
}
