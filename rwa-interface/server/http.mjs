// Small helpers shared by the server modules.
import { createHash } from 'node:crypto';
import { gzip } from 'node:zlib';
import { promisify } from 'node:util';

const gzipAsync = promisify(gzip);

export const TYPES = {
  '.html': 'text/html; charset=utf-8',
  '.js': 'text/javascript; charset=utf-8',
  '.mjs': 'text/javascript; charset=utf-8',
  '.css': 'text/css; charset=utf-8',
  '.json': 'application/json; charset=utf-8',
  '.xml': 'application/xml; charset=utf-8',
  '.txt': 'text/plain; charset=utf-8',
  '.webmanifest': 'application/manifest+json',
  '.png': 'image/png',
  '.jpg': 'image/jpeg',
  '.svg': 'image/svg+xml',
  '.ico': 'image/x-icon',
  '.woff': 'font/woff',
  '.woff2': 'font/woff2'
};

const COMPRESSIBLE = /^(text\/|application\/(json|xml|manifest)|image\/svg)/;

export const etagOf = (body) => `"${createHash('sha1').update(body).digest('base64url')}"`;

// Sends a buffer, gzipped when the client accepts it and the type is text. `res.req` is the request.
export async function sendBuffer(res, status, type, body, headers = {}) {
  const req = res.req;
  const out = { 'content-type': type, vary: 'accept-encoding', ...headers };
  let payload = body;
  if (body.length > 1024 && COMPRESSIBLE.test(type) && /\bgzip\b/.test(req.headers['accept-encoding'] ?? '')) {
    payload = await gzipAsync(body);
    out['content-encoding'] = 'gzip';
    // the ETag names the content, not these bytes: weak, so it never claims byte equality with the identity body
    if (out.etag && !out.etag.startsWith('W/')) out.etag = `W/${out.etag}`;
  }
  out['content-length'] = payload.length;
  res.writeHead(status, out);
  res.end(req.method === 'HEAD' ? undefined : payload);
}

export const notModified = (req, etag) => {
  const given = req.headers['if-none-match'];
  return !!given && given.split(',').some((tag) => tag.trim().replace(/^W\//, '') === etag.replace(/^W\//, ''));
};
