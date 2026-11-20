// Files of dist/ (the Vite build): hashed bundles under /assets with an immutable cache, everything else (favicon,
// manifest, ...) with a short one. Never answers an unknown path with dist/index.html: unknown paths are the server's
// 404 pages. dist/index.html itself is the template of server/render.mjs, not a page; /data and /logos belong to the API.
import { readFile, stat } from 'node:fs/promises';
import path from 'node:path';

import { etagOf, notModified, sendBuffer, TYPES } from './http.mjs';

const EXCLUDED = /^\/(index\.html|data|logos)(\/|$)/;

export function createStatic({ root }) {
  const dist = path.join(root, 'dist');
  const cache = new Map(); // file -> { mtimeMs, body, etag }

  // true when the request was answered
  return async function serveStatic(req, res, pathname) {
    if (EXCLUDED.test(pathname)) return false;
    const file = path.join(dist, pathname);
    if (!file.startsWith(dist + path.sep)) return false;
    const info = await stat(file).catch(() => null);
    if (!info?.isFile()) return false;

    let entry = cache.get(file);
    if (!entry || entry.mtimeMs !== info.mtimeMs) {
      const body = await readFile(file);
      entry = { mtimeMs: info.mtimeMs, body, etag: etagOf(body) };
      if (body.length < 5_000_000) cache.set(file, entry);
    }
    const immutable = pathname.startsWith('/assets/');
    const headers = { etag: entry.etag, 'cache-control': immutable ? 'public, max-age=31536000, immutable' : 'public, max-age=3600' };
    if (notModified(req, entry.etag)) {
      res.writeHead(304, headers);
      res.end();
    } else {
      await sendBuffer(res, 200, TYPES[path.extname(file)] ?? 'application/octet-stream', entry.body, headers);
    }
    return true;
  };
}
