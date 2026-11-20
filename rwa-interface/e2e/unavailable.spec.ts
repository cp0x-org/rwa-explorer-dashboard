import { expect, test } from '@playwright/test';

// The "down" server renders from a mock API started with --fail: it never gets an export.
const DOWN = `http://localhost:${process.env.E2E_DOWN_PORT ?? 4322}`;

const apiDown = { status: 502, contentType: 'application/json', body: JSON.stringify({ error: { code: 'unavailable', message: 'down' } }) };

test.describe('API unavailable', () => {
  test('the server answers 503 with a plain "Data unavailable" page in the language of the URL', async ({ request }) => {
    const en = await request.get(`${DOWN}/assets/NVDA`);
    expect(en.status()).toBe(503);
    expect(en.headers()['retry-after']).toBeTruthy();
    const html = await en.text();
    expect(html).toContain('<html lang="en">');
    expect(html).toContain('Data unavailable');
    expect(html).not.toContain('http-equiv="refresh"');
    expect(html).toMatch(/<a href="\/assets\/NVDA">Reload<\/a>/);

    const zh = await request.get(`${DOWN}/zh/tokens/36992`);
    expect(zh.status()).toBe(503);
    const zhHtml = await zh.text();
    expect(zhHtml).toContain('<html lang="zh-CN">');
    expect(zhHtml).toContain('数据不可用');

    expect((await request.get(`${DOWN}/healthz`)).status()).toBe(503);
    expect((await request.get(`${DOWN}/sitemap.xml`)).status()).toBe(503);
  });

  test('a served page stays readable when the export cannot be loaded afterwards', async ({ page }) => {
    await page.route(/\/api\/v1\/export/, (route) => route.fulfill(apiDown));
    await page.goto('/assets/NVDA');
    await page.waitForTimeout(1500); // the client has tried (and failed) by now
    await expect(page.getByRole('heading', { level: 1 })).toHaveText('NVDA');
    expect(await page.locator('table tbody tr').count()).toBeGreaterThan(3);
    await expect(page.getByRole('alert')).toHaveCount(0);
  });

  test('without server markup the data gate shows a localized error', async ({ page }) => {
    // the same page, but its markup is moved out of #root: the client starts from nothing, like the dev server does
    await page.route(
      (url) => url.pathname === '/zh/assets',
      async (route) => {
        const response = await route.fetch();
        const html = (await response.text())
          .replace('<div id="root"', '<div id="server-markup" hidden')
          .replace('<body>', '<body><div id="root"></div>');
        await route.fulfill({ response, body: html });
      }
    );
    await page.route(/\/api\/v1\/export/, (route) => route.fulfill(apiDown));
    await page.goto('/zh/assets');
    const alert = page.getByRole('alert');
    await expect(alert).toBeVisible();
    await expect(alert.getByRole('heading', { level: 1 })).toHaveText('数据不可用');
    await expect(alert).toContainText('HTTP 502');
    await expect(alert).not.toContainText('rwa-pipeline');
  });
});
