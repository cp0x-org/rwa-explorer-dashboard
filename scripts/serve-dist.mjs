// Static server for dist/ with the same behaviour as `serve dist -c ../serve.json` in the Dockerfile: clean URLs
// (`/assets/NVDA` -> dist/assets/NVDA/index.html), unknown paths -> dist/404.html with status 404, no SPA fallback.
// Used by the e2e tests (playwright.config.ts); `node scripts/serve-dist.mjs [port]` also serves a build by hand.
import { createReadStream, existsSync, statSync } from 'node:fs';
import { createServer } from 'node:http';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const dist = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const port = Number(process.argv[2] ?? process.env.PORT ?? 4173);

const types = {
  '.html': 'text/html; charset=utf-8',
  '.js': 'text/javascript; charset=utf-8',
  '.mjs': 'text/javascript; charset=utf-8',
  '.css': 'text/css; charset=utf-8',
  '.json': 'application/json; charset=utf-8',
  '.xml': 'application/xml; charset=utf-8',
  '.txt': 'text/plain; charset=utf-8',
  '.png': 'image/png',
  '.svg': 'image/svg+xml',
  '.ico': 'image/x-icon',
  '.woff': 'font/woff',
  '.woff2': 'font/woff2'
};

const send = (res, file, status = 200) => {
  res.writeHead(status, { 'content-type': types[path.extname(file)] ?? 'application/octet-stream' });
  createReadStream(file).pipe(res);
};

createServer((req, res) => {
  const pathname = decodeURIComponent(new URL(req.url, 'http://localhost').pathname).replace(/\/+$/, '') || '/';
  const target = path.join(dist, pathname);
  if (!target.startsWith(dist)) return send(res, path.join(dist, '404.html'), 404);
  if (existsSync(target) && statSync(target).isFile()) return send(res, target);
  const index = path.join(target, 'index.html');
  if (existsSync(index)) return send(res, index);
  return send(res, path.join(dist, '404.html'), 404);
}).listen(port, () => console.log(`serving ${dist} on http://localhost:${port}`));
