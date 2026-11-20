import { expect, test } from '@playwright/test';

import { hydrated } from './helpers';

// The server renders from build N and says so in <meta name="rwa-build">; the client loads exactly that build before it
// hydrates, so the data behind the markup and behind the first client render are the same: no hydration mismatch.
for (const route of ['/assets/NVDA', '/zh/tokens/36992']) {
  test(`${route} hydrates the server markup without console errors or warnings`, async ({ page }) => {
    const problems: string[] = [];
    page.on('console', (msg) => {
      if (msg.type() === 'error' || msg.type() === 'warning') problems.push(`${msg.type()}: ${msg.text()}`);
    });
    page.on('pageerror', (err) => problems.push(err.message));
    const exportRequests: string[] = [];
    page.on('request', (req) => {
      if (/\/api\/v1\/export/.test(req.url())) exportRequests.push(req.url());
    });
    // remember the server-rendered <h1> before any script runs: hydration must keep that very node (a client render would replace it)
    await page.addInitScript(() => {
      document.addEventListener('DOMContentLoaded', () => {
        (window as unknown as { __serverH1?: Element | null }).__serverH1 = document.querySelector('#root h1');
      });
    });

    await page.goto(route);
    await hydrated(page);
    await page.waitForLoadState('networkidle');

    const build = await page.locator('meta[name="rwa-build"]').getAttribute('content');
    expect(build).toMatch(/^\d+$/);
    expect(await page.evaluate(() => window.__RWA_BUILD__)).toBe(Number(build));
    expect(
      await page.evaluate(() => {
        const first = (window as unknown as { __serverH1?: Element | null }).__serverH1;
        return !!first && first === document.querySelector('#root h1');
      })
    ).toBe(true);
    // the page preloads the pinned build; the client reads that very URL, once, and not the latest export as well
    await expect(page.locator(`link[rel="preload"][as="fetch"][href="/api/v1/export?build=${build}"]`)).toHaveCount(1);
    expect(exportRequests.map((u) => new URL(u).pathname + new URL(u).search)).toEqual([`/api/v1/export?build=${build}`]);
    expect(problems).toEqual([]);
  });
}
