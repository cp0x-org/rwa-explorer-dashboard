// Writes a static HTML page for every route into dist/, so pages are readable without JavaScript (crawlers, AI agents,
// link previews) and show the data at once, plus sitemap.xml and robots.txt. Runs after `vite build` and
// `vite build --ssr src/entry-server.tsx`; the browser then hydrates these pages (src/index.tsx).
// Data comes from dist/data/rwa.json, the same file the site serves.
import { mkdir, readFile, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const dist = path.join(root, 'dist');
const { render, prerenderPaths, siteUrl } = await import(pathToFileURL(path.join(root, 'dist-ssr', 'entry-server.mjs')).href);

const template = await readFile(path.join(dist, 'index.html'), 'utf8');
const data = JSON.parse(await readFile(path.join(dist, 'data', 'rwa.json'), 'utf8'));

const escapeHtml = (s) => s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
const canonical = (route) => (siteUrl ? `<link rel="canonical" href="${escapeHtml(siteUrl + route)}" />` : '');

// replacements use functions: rendered HTML contains "$" (prices), which a replacement string would treat as a pattern
const slots = [
  [/<title>[^<]*<\/title>/, ({ meta }) => `<title>${escapeHtml(meta.title)}</title>`],
  [/(<meta (?:name|property)="(?:title|og:title)" content=")[^"]*"/g, ({ meta }, open) => `${open}${escapeHtml(meta.title)}"`],
  [
    /(<meta (?:name|property)="(?:description|og:description)" content=")[^"]*"/g,
    ({ meta }, open) => `${open}${escapeHtml(meta.description)}"`
  ],
  ['<!--prerender-head-->', ({ styles }, _open, route) => canonical(route) + styles],
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

// unknown paths (served with status 404): the app shell, the client router shows the right page or "not found"
await writeFile(path.join(dist, '404.html'), template);

const started = Date.now();
let bytes = 0;
const routes = prerenderPaths(data);
for (const route of routes) {
  const html = fill(await render(route, data), route);
  const file = path.join(dist, ...route.split('/').filter(Boolean).map(decodeURIComponent), 'index.html');
  await mkdir(path.dirname(file), { recursive: true });
  await writeFile(file, html);
  bytes += html.length;
  // "/" shows the first section (the app then redirects to it); its canonical link points to /assets
  if (route === '/assets') await writeFile(path.join(dist, 'index.html'), html);
}
console.log(
  `prerendered ${routes.length} pages in ${((Date.now() - started) / 1000).toFixed(1)}s, ` +
    `${(bytes / routes.length / 1024).toFixed(0)} KB per page on average`
);

// sitemap: every prerendered page, last modified = newest data collection
if (siteUrl) {
  const lastmod = data.summary.newest_fetched_at ? new Date(data.summary.newest_fetched_at).toISOString().slice(0, 10) : null;
  const urls = routes.map(
    (route) => `  <url><loc>${escapeHtml(siteUrl + route)}</loc>${lastmod ? `<lastmod>${lastmod}</lastmod>` : ''}</url>`
  );
  await writeFile(
    path.join(dist, 'sitemap.xml'),
    `<?xml version="1.0" encoding="UTF-8"?>\n<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">\n${urls.join('\n')}\n</urlset>\n`
  );
  await writeFile(path.join(dist, 'robots.txt'), `User-agent: *\nAllow: /\n\nSitemap: ${siteUrl}/sitemap.xml\n`);
  console.log(`sitemap.xml: ${routes.length} URLs on ${siteUrl}`);
} else {
  console.warn('VITE_SITE_URL is not set: no canonical links, sitemap.xml or robots.txt');
}
