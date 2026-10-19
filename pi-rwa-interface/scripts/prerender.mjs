// Writes a static HTML page for every route and language into dist/, so pages are readable without JavaScript
// (crawlers, AI agents, link previews) and show the data at once, plus sitemap.xml and robots.txt. Runs after
// `vite build` and `vite build --ssr src/entry-server.tsx`; the browser then hydrates these pages (src/index.tsx).
// Data comes from dist/data/rwa.json, the same file the site serves.
import { mkdir, readFile, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const dist = path.join(root, 'dist');
const { render, prerenderPaths, siteUrl, NOT_FOUND_PATH, LOCALES } = await import(
  pathToFileURL(path.join(root, 'dist-ssr', 'entry-server.mjs')).href
);

const template = await readFile(path.join(dist, 'index.html'), 'utf8');
const data = JSON.parse(await readFile(path.join(dist, 'data', 'rwa.json'), 'utf8'));

const escapeHtml = (s) => s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
// "</script>" inside JSON-LD would end the script element
const escapeJson = (obj) => JSON.stringify(obj).replace(/</g, '\\u003c');

// <head> additions of one page: canonical, hreflang pairs, Open Graph URL and locale, robots, JSON-LD.
function headTags(page) {
  const tags = [];
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
}

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
  ['<!--prerender-head-->', (page) => headTags(page) + page.styles],
  [/<noscript>[^<]*<\/noscript>/, ({ noscript }) => `<noscript>${escapeHtml(noscript)}</noscript>`],
  ['<div id="root"></div>', ({ html }, _open, route) => `<div id="root" data-path="${escapeHtml(route)}">${html}</div>`]
];
for (const [pattern] of slots) {
  if (typeof pattern === 'string' ? !template.includes(pattern) : template.search(pattern) === -1) {
    // also after a second run: dist/index.html is then a prerendered page, run the whole `pnpm build`
    throw new Error(`dist/index.html has no ${pattern}: the prerender cannot fill it`);
  }
}

const fill = (page, route) =>
  slots.reduce((html, [pattern, value]) => html.replace(pattern, (_match, open) => value(page, open, route)), template);

const fileOf = (route) => path.join(dist, ...route.split('/').filter(Boolean).map(decodeURIComponent), 'index.html');

// unknown paths (served with status 404): a real "not found" page with the header, both languages' links and noindex;
// the client router then shows the right page or "not found" in the language of the URL
const notFound = await render(NOT_FOUND_PATH, data);
await writeFile(path.join(dist, '404.html'), fill(notFound, NOT_FOUND_PATH));

const started = Date.now();
let bytes = 0;
const routes = prerenderPaths(data);
const heads = []; // canonical and alternates of every page, for the sitemap
for (const route of routes) {
  const page = await render(route, data);
  if (!page.meta.found) throw new Error(`${route}: rendered as "not found"`);
  if (page.head) heads.push(page.head);
  const html = fill(page, route);
  const file = fileOf(route);
  await mkdir(path.dirname(file), { recursive: true });
  await writeFile(file, html);
  bytes += html.length;
  // "/" and "/zh" show the first section (the app then redirects to it); their canonical link points to the section
  if (route === '/assets') await writeFile(path.join(dist, 'index.html'), html);
  if (route === '/zh/assets') await writeFile(path.join(dist, 'zh', 'index.html'), html);
}
console.log(
  `prerendered ${routes.length} pages in ${LOCALES.length} languages in ${((Date.now() - started) / 1000).toFixed(1)}s, ` +
    `${(bytes / routes.length / 1024).toFixed(0)} KB per page on average`
);

// sitemap: every prerendered page in every language with its alternates; last modified = newest data collection
if (siteUrl) {
  const lastmod = data.summary.newest_fetched_at ? new Date(data.summary.newest_fetched_at).toISOString().slice(0, 10) : null;
  const urls = heads.map(({ canonical, hreflang }) => {
    const links = hreflang.map(({ lang, href }) => `<xhtml:link rel="alternate" hreflang="${lang}" href="${escapeHtml(href)}"/>`);
    return `  <url><loc>${escapeHtml(canonical)}</loc>${lastmod ? `<lastmod>${lastmod}</lastmod>` : ''}${links.join('')}</url>`;
  });
  await writeFile(
    path.join(dist, 'sitemap.xml'),
    `<?xml version="1.0" encoding="UTF-8"?>\n` +
      `<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9" xmlns:xhtml="http://www.w3.org/1999/xhtml">\n` +
      `${urls.join('\n')}\n</urlset>\n`
  );
  await writeFile(path.join(dist, 'robots.txt'), `User-agent: *\nAllow: /\n\nSitemap: ${siteUrl}/sitemap.xml\n`);
  console.log(`sitemap.xml: ${urls.length} URLs on ${siteUrl}`);
} else {
  console.warn('VITE_SITE_URL is not set: no canonical links, hreflang, sitemap.xml or robots.txt');
}
