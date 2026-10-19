import { expect, test } from '@playwright/test';

import { CJK, ROUTES, SAMPLE } from './helpers';

// Raw HTTP responses: what crawlers and users without JavaScript get.
test.describe('server-rendered HTML', () => {
  for (const route of ROUTES) {
    test(`${route} and /zh${route} are prerendered with real content`, async ({ request }) => {
      const en = await request.get(route);
      expect(en.status()).toBe(200);
      const enHtml = await en.text();
      expect(enHtml).toMatch(/<html lang="en">/);
      expect(enHtml).toMatch(/<h1[^>]*>[^<]+<\/h1>/);
      expect(enHtml).toContain(`<div id="root" data-path="${route}">`);
      expect(enHtml).toContain('application/ld+json');
      expect(enHtml).not.toMatch(/name="robots"/);

      const zh = await request.get(`/zh${route}`);
      expect(zh.status()).toBe(200);
      const zhHtml = await zh.text();
      expect(zhHtml).toMatch(/<html lang="zh-CN">/);
      const body = zhHtml.slice(zhHtml.indexOf('<div id="root"'));
      // translated content in the markup, not only in the metadata
      expect(body.match(/<h1[^>]*>([^<]+)<\/h1>/)?.[1]).toBeTruthy();
      expect(body).toMatch(CJK);
      expect(body.match(/<caption[^>]*>[^<]*<\/caption>/g)?.length ?? body.match(/<h2/g)?.length).toBeTruthy();
      // every internal link of a Chinese page stays in Chinese, except the language switcher
      const internal = Array.from(body.matchAll(/<a [^>]*href="(\/[^"]*)"/g), (m) => m[1]);
      expect(internal.length).toBeGreaterThan(5);
      const english = internal.filter((h) => !h.startsWith('/zh'));
      expect(english).toEqual([route]);
    });
  }

  test('main content is there without JavaScript', async ({ browser }) => {
    const context = await browser.newContext({ javaScriptEnabled: false });
    const page = await context.newPage();
    await page.goto(`/assets/${SAMPLE.ticker}`);
    await expect(page.getByRole('heading', { level: 1 })).toHaveText(SAMPLE.ticker);
    expect(await page.locator('table tbody tr').count()).toBeGreaterThan(3);
    await expect(page.getByRole('navigation', { name: 'Sections' })).toBeVisible();
    await expect(page.getByRole('navigation', { name: 'Language' })).toBeVisible();
    await expect(page.locator('footer')).toBeVisible();

    await page.goto('/zh/tokenized');
    await expect(page.getByRole('heading', { level: 1 })).toHaveText('代币化资产');
    expect(await page.locator('table tbody tr').count()).toBeGreaterThan(10);
    await context.close();
  });

  test('the root of each language shows the assets page', async ({ request }) => {
    for (const [root, lang] of [
      ['/', 'en'],
      ['/zh', 'zh-CN']
    ]) {
      const res = await request.get(root);
      expect(res.status()).toBe(200);
      const html = await res.text();
      expect(html).toContain(`<html lang="${lang}">`);
      expect(html).toContain(`rel="canonical" href="https://rwa.cp0x.com${lang === 'en' ? '/assets' : '/zh/assets'}"`);
    }
  });

  test('unknown records and paths return a real HTTP 404 page', async ({ request }) => {
    for (const path of [
      '/tokens/999999999',
      '/assets/NOPE',
      '/issuers/nope',
      '/zh/tokens/999999999',
      '/zh/assets/NOPE',
      '/nothing-here',
      '/zh/nothing'
    ]) {
      const res = await request.get(path);
      expect(res.status(), path).toBe(404);
      const html = await res.text();
      expect(html).toContain('<meta name="robots" content="noindex" />');
      expect(html).toMatch(/<h1[^>]*>Page not found<\/h1>/);
    }
  });

  test('the client shows "not found" in the language of the URL', async ({ page }) => {
    await page.goto('/zh/tokens/999999999');
    await expect(page.getByRole('heading', { level: 1 })).toHaveText(/当前数据中没有代币 999999999/);
    await expect(page.locator('html')).toHaveAttribute('lang', 'zh-CN');
    await page.goto('/tokens/999999999');
    await expect(page.getByRole('heading', { level: 1 })).toHaveText(/Token 999999999 not found/);
  });

  test('robots.txt and sitemap.xml cover both languages', async ({ request }) => {
    const robots = await (await request.get('/robots.txt')).text();
    expect(robots).toContain('Sitemap: https://rwa.cp0x.com/sitemap.xml');
    expect(robots).not.toMatch(/Disallow: \//);
    const res = await request.get('/sitemap.xml');
    expect(res.status()).toBe(200);
    const xml = await res.text();
    expect(xml).toContain('xmlns:xhtml="http://www.w3.org/1999/xhtml"');
    for (const route of ROUTES) {
      expect(xml).toContain(`<loc>https://rwa.cp0x.com${route}</loc>`);
      expect(xml).toContain(`<loc>https://rwa.cp0x.com/zh${route}</loc>`);
      expect(xml).toContain(`hreflang="zh-CN" href="https://rwa.cp0x.com/zh${route}"`);
    }
    expect(xml).not.toContain('__not_found__');
    expect(xml).not.toMatch(/<loc>[^<]*\?/); // no filter/query variants
    const locs = xml.match(/<loc>/g)?.length ?? 0;
    const lastmods = xml.match(/<lastmod>\d{4}-\d{2}-\d{2}<\/lastmod>/g)?.length ?? 0;
    expect(lastmods).toBe(locs);
  });
});
