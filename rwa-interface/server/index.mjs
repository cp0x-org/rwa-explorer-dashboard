// Server for RWA Explorer 2.0: renders every page on request from the export of the API (docs/v2/04-frontend.md §2).
//   PORT      listen port, default 4173
//   API_URL   base URL of the API the export is read from, default http://127.0.0.1:8080
//   SITE_URL  public address of the site (canonical links, sitemap.xml, robots.txt); default: VITE_SITE_URL of the build
//   API_PUBLIC_URL  public address of the API for programs and agents (llms.txt, JSON-LD, API links in <head>);
//             default: VITE_API_PUBLIC_URL of the build, else SITE_URL (the browser always calls this server's /api/)
// Needs `pnpm build` first: dist/ (client bundle and HTML template) and dist-ssr/ (server bundle).
// In production a reverse proxy sends /api/ and /logos/ to the API; this server forwards them itself when asked
// (e2e tests, `pnpm serve`), so the browser always talks to one origin.
import { createServer, request as httpRequest } from 'node:http';
import { request as httpsRequest } from 'node:https';
import { pipeline } from 'node:stream';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

import { printBanner } from './banner.mjs';
import { createDataStore, REFRESH_MS, RETRY_MS } from './data.mjs';
import { etagOf, notModified, sendBuffer, TYPES } from './http.mjs';
import { createRenderer } from './render.mjs';
import { createSitemap } from './sitemap.mjs';
import { createStatic } from './static.mjs';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const port = Number(process.env.PORT ?? 4173);
const apiUrl = (process.env.API_URL ?? 'http://127.0.0.1:8080').replace(/\/+$/, '');
const siteUrl = process.env.SITE_URL?.replace(/\/+$/, '') || undefined;
const apiPublicUrl = process.env.API_PUBLIC_URL?.replace(/\/+$/, '') || undefined;

const store = createDataStore({ apiUrl });
const renderer = await createRenderer({ root, siteUrl, apiPublicUrl });
const sitemap = createSitemap({ renderer });
const serveStatic = createStatic({ root });

// /api/* and /logos/* are the API's: streamed through as they are (conditional requests and encodings included), not cached here
const FORWARD_REQUEST = ['accept', 'accept-encoding', 'if-none-match', 'if-modified-since'];
const FORWARD_RESPONSE = ['content-type', 'content-encoding', 'content-length', 'etag', 'last-modified', 'cache-control', 'vary', 'retry-after'];

function proxy(req, res, url) {
  const target = new URL(url.pathname + url.search, apiUrl);
  const headers = {};
  for (const name of FORWARD_REQUEST) if (req.headers[name]) headers[name] = req.headers[name];
  const upstream = (target.protocol === 'https:' ? httpsRequest : httpRequest)(target, { method: req.method, headers, timeout: 30_000 });
  // the browser may go away before the API answers (a closed tab, a cancelled fetch): drop the upstream request and
  // never pipe into the closed response, which would throw outside any handler and stop the whole server
  res.on('close', () => {
    if (!res.writableFinished) upstream.destroy();
  });
  const fail = (err) => {
    if (res.destroyed) return;
    if (res.headersSent) return res.destroy();
    res.writeHead(502, { 'content-type': TYPES['.json'] });
    res.end(JSON.stringify({ error: { code: 'unavailable', message: `API unreachable: ${err.message}` } }));
  };
  upstream.on('timeout', () => upstream.destroy(new Error('timeout')));
  upstream.on('error', fail);
  upstream.on('response', (up) => {
    if (res.destroyed) return up.destroy();
    const out = {};
    for (const name of FORWARD_RESPONSE) if (up.headers[name]) out[name] = up.headers[name];
    res.writeHead(up.statusCode ?? 502, out);
    pipeline(up, res, () => {});
  });
  upstream.end();
}

const text = (res, status, type, body, headers = {}) => sendBuffer(res, status, type, Buffer.from(body), headers);

async function handle(req, res) {
  if (req.method !== 'GET' && req.method !== 'HEAD') {
    res.writeHead(405, { allow: 'GET, HEAD' });
    return res.end();
  }
  let url;
  try {
    url = new URL(req.url, 'http://localhost');
  } catch {
    return text(res, 400, TYPES['.txt'], 'Bad request');
  }
  const raw = url.pathname;

  if (raw === '/healthz') {
    const ok = !!store.export;
    return text(res, ok ? 200 : 503, TYPES['.json'], JSON.stringify({ ok, build_id: store.buildId }), { 'cache-control': 'no-store' });
  }
  if (raw.startsWith('/api/') || raw.startsWith('/logos/')) return proxy(req, res, url);

  let pathname;
  try {
    pathname = decodeURIComponent(raw);
  } catch {
    pathname = raw;
  }
  if (await serveStatic(req, res, pathname)) return;

  // browsers and crawlers ask for /favicon.ico whatever the page links: the same icon the pages name
  if (raw === '/favicon.ico' && renderer.icon && (await serveStatic(req, res, renderer.icon))) return;

  if (raw === '/robots.txt') {
    const robots = sitemap.robots();
    if (robots) return text(res, 200, TYPES['.txt'], robots, { 'cache-control': 'public, max-age=3600' });
  }

  const data = store.export;
  if (!data) {
    // no export yet: a plain page with a 503, the client (and crawlers) retry later
    return text(res, 503, TYPES['.html'], renderer.unavailable(raw), { 'cache-control': 'no-store', 'retry-after': '10' });
  }

  if (raw === '/llms.txt') {
    const llms = renderer.llms(data);
    if (llms) return text(res, 200, TYPES['.txt'], llms, { 'cache-control': 'public, max-age=300' });
  }

  if (raw === '/sitemap.xml') {
    const xml = sitemap.xml(data);
    if (xml) return text(res, 200, TYPES['.xml'], xml, { 'cache-control': 'public, max-age=300' });
  }

  // a trailing slash is the same page: send the client to the clean address
  if (raw.length > 1 && raw.endsWith('/')) {
    // never a "//host" location: the path is rebuilt from its segments, so it stays on this origin
    res.writeHead(301, { location: '/' + raw.replace(/^\/+|\/+$/g, '') + url.search });
    return res.end();
  }

  // API answers the page needs besides the export (the runs on /updates); a page renders without them when the API fails
  const fallback = {};
  for (const key of renderer.apiFor(raw)) {
    const body = await store.getApi(key);
    if (body !== undefined) fallback[key] = body;
  }
  const { status, html } = await renderer.page(raw, data, fallback);
  const headers = { 'cache-control': 'no-cache' };
  if (status === 200) {
    const api = Object.keys(fallback).length ? etagOf(JSON.stringify(fallback)) : '';
    headers.etag = etagOf(`${renderer.version}:${data.build_id}:${raw}:${api}`);
    if (notModified(req, headers.etag)) {
      res.writeHead(304, headers);
      return res.end();
    }
  }
  return text(res, status, TYPES['.html'], html, headers);
}

const server = createServer((req, res) => {
  handle(req, res).catch((err) => {
    console.error(`${req.method} ${req.url}: ${err.stack ?? err}`);
    if (!res.headersSent) res.writeHead(500, { 'content-type': 'text/plain; charset=utf-8' });
    res.end('Internal server error');
  });
});

server.listen(port, () => {
  printBanner({ port, apiUrl, siteUrl: renderer.site, apiPublicUrl: renderer.api, refreshMs: REFRESH_MS, retryMs: RETRY_MS, templateVersion: renderer.version.slice(0, 8) });
  console.log(`rwa-interface on http://localhost:${port}, API ${apiUrl}${renderer.site ? `, site ${renderer.site}` : ''}`);
});
store.start();


for (const signal of ['SIGINT', 'SIGTERM']) {
  process.on(signal, () => {
    store.stop();
    server.close(() => process.exit(0));
    setTimeout(() => process.exit(0), 2000).unref();
  });
}
