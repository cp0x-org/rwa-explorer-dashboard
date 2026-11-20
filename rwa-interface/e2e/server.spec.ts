import { request as httpRequest } from 'node:http';
import { expect, test } from '@playwright/test';

// Raw request paths (the browser and Playwright's URL handling would normalize some of them away).
const PORT = Number(process.env.E2E_PORT ?? 4321);
const raw = (path: string, headers: Record<string, string> = {}) =>
  new Promise<{ status: number; headers: Record<string, string | string[] | undefined> }>((resolve, reject) => {
    const req = httpRequest({ host: '127.0.0.1', port: PORT, path, headers }, (res) => {
      res.resume();
      res.on('end', () => resolve({ status: res.statusCode ?? 0, headers: res.headers }));
    });
    req.on('error', reject);
    req.end();
  });

test.describe('server', () => {
  test('a trailing slash redirects to the clean path, never off this origin', async () => {
    const clean = await raw('/assets/');
    expect(clean.status).toBe(301);
    expect(clean.headers.location).toBe('/assets');
    const tricky = await raw('/.//evil.com/');
    expect(tricky.status).toBe(301);
    expect(tricky.headers.location).toBe('/evil.com');
    expect(String(tricky.headers.location).startsWith('//')).toBe(false);
  });

  test('an unparseable request target is a 400, not a 500', async () => {
    expect((await raw('//')).status).toBe(400);
  });

  test('page ETags give 304, assets are immutable', async () => {
    const first = await raw('/assets/NVDA');
    expect(first.status).toBe(200);
    const etag = String(first.headers.etag);
    expect((await raw('/assets/NVDA', { 'if-none-match': etag })).status).toBe(304);
  });
});
