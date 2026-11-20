import { expect, test } from '@playwright/test';

import { hydrated, watchErrors } from './helpers';

// The mock API (mock-api/server.mjs) lists four runs: 4 mock-rebuild (rebuild), 3 mock-partial (partial, warnings, one failed
// asset), 2 mock-refresh (succeeded, new tokens, new issuer, details), 1 import-1x (import).

test.describe('/updates', () => {
  test('the server renders the page shell and a busy placeholder, no runs (they are loaded in the browser)', async ({ request }) => {
    const res = await request.get('/updates');
    expect(res.status()).toBe(200);
    const html = await res.text();
    expect(html).toMatch(/<h1[^>]*>Data updates<\/h1>/);
    expect(html).toContain('aria-busy="true"');
    expect(html).not.toContain('<table');
    expect(html).not.toMatch(/mock-partial|mock-refresh/);
    expect(html).toContain('application/ld+json');
    expect(html).not.toMatch(/name="robots"/);
    expect(html).toContain('rel="canonical" href="https://rwa.cp0x.com/updates"');
    // the footer link is in the server markup; "next collection" is client-only
    expect(html).toMatch(/<a [^>]*href="\/updates"[^>]*>Update history<\/a>/);
    expect(html).toContain('Data collected');
    expect(html).not.toContain('next collection');
  });

  test('lists the runs after hydration, hides rebuilds until asked, and links each date to its page', async ({ page }) => {
    const errors = watchErrors(page);
    await page.goto('/updates');
    await hydrated(page);
    const table = page.getByRole('table', { name: 'Data collection runs' });
    await expect(table).toBeVisible();
    const rows = table.locator('tbody tr');
    await expect(rows).toHaveCount(3); // the rebuild is hidden
    await expect(table.getByRole('row', { name: /Partial/ })).toBeVisible();
    await expect(table.getByRole('row', { name: /Succeeded/ }).first()).toContainText('+8'); // Δ eligible of run 2
    await expect(table.getByText('Rebuild')).toHaveCount(0);

    await page.getByRole('switch', { name: 'Show rebuilds' }).check();
    await expect(rows).toHaveCount(4);
    await expect(table.getByText('Rebuild · manual')).toBeVisible();
    await page.getByRole('switch', { name: 'Show rebuilds' }).uncheck();
    await expect(rows).toHaveCount(3);

    // the date is a real link
    const link = rows.first().getByRole('rowheader').getByRole('link');
    await expect(link).toHaveAttribute('href', '/updates/3');
    await link.click();
    await page.waitForURL('**/updates/3');
    await expect(page.getByRole('heading', { level: 1 })).toHaveText('Update #3');

    // the footer: collected + next collection (from /status), link to the list
    const footer = page.getByRole('contentinfo');
    await expect(footer).toContainText('Data collected');
    await expect(footer).toContainText('next collection ~');
    expect(errors).toEqual([]);
  });

  test('Chinese page: heading, table and links stay in Chinese', async ({ page }) => {
    const errors = watchErrors(page);
    await page.goto('/zh/updates');
    await hydrated(page);
    await expect(page.getByRole('heading', { level: 1 })).toHaveText('数据更新');
    const table = page.getByRole('table', { name: '数据采集记录' });
    await expect(table.locator('tbody tr')).toHaveCount(3);
    await expect(table.getByRole('row', { name: /部分成功/ })).toBeVisible();
    await expect(table.locator('tbody tr').first().getByRole('rowheader').getByRole('link')).toHaveAttribute('href', '/zh/updates/3');
    expect(errors).toEqual([]);
  });

  test('the menu has an Updates item', async ({ page, isMobile }) => {
    await page.goto('/assets');
    await hydrated(page);
    if (isMobile) await page.getByRole('button', { name: 'Open menu' }).click();
    const nav = page.getByRole('navigation', { name: 'Site menu' });
    await nav.getByRole('link', { name: 'Updates' }).click();
    await page.waitForURL('**/updates');
    await expect(page.getByRole('heading', { level: 1 })).toHaveText('Data updates');
  });

  test('the listing is in the sitemap, the detail pages are not', async ({ request }) => {
    const xml = await (await request.get('/sitemap.xml')).text();
    expect(xml).toContain('<loc>https://rwa.cp0x.com/updates</loc>');
    expect(xml).toContain('<loc>https://rwa.cp0x.com/zh/updates</loc>');
    expect(xml).not.toMatch(/\/updates\/\d/);
  });
});

test.describe('/updates/:id', () => {
  test('the server renders the shell with status 200 and noindex, for any numeric id', async ({ request }) => {
    for (const path of ['/updates/3', '/updates/12345']) {
      const res = await request.get(path);
      expect(res.status(), path).toBe(200);
      const html = await res.text();
      expect(html, path).toMatch(/<meta name="robots" content="noindex" \/>/);
      expect(html, path).not.toContain('rel="canonical"');
      expect(html, path).not.toContain('application/ld+json');
      expect(html, path).toMatch(/<h1[^>]*>Update #\d+<\/h1>/);
      expect(html, path).toContain('aria-busy="true"');
      expect(html, path).not.toContain('<table');
    }
    expect((await request.get('/updates/abc')).status()).toBe(404);
    expect((await request.get('/zh/updates/abc')).status()).toBe(404);
  });

  test('a partial run: warnings, per-asset results with a filter, no changes', async ({ page }) => {
    const errors = watchErrors(page);
    await page.goto('/updates/3');
    await hydrated(page);
    await expect(page.getByRole('heading', { level: 1 })).toHaveText('Update #3');
    await expect(page.getByText('Run key mock-partial')).toBeVisible();
    await expect(page.getByText('Partial', { exact: true }).first()).toBeVisible();
    const stats = page.locator('dl').first();
    await expect(stats).toContainText('Underlyings selected');
    await expect(stats).toContainText('CMC credits');

    await expect(page.getByRole('heading', { level: 2, name: /Warnings \(2\)/ })).toBeVisible();
    await expect(page.getByText('Portals search for IREN returned 502 after 3 retries')).toBeVisible();
    await expect(page.getByText('This run recorded no changes.')).toBeVisible();

    const table = page.getByRole('table', { name: 'Underlying assets in this run' });
    await expect(table.locator('tbody tr')).toHaveCount(4);
    await page.getByRole('switch', { name: 'Show only assets that are not ok' }).check();
    await expect(table.locator('tbody tr')).toHaveCount(1);
    await expect(table.locator('tbody tr')).toContainText('IREN');
    await expect(table.locator('tbody tr')).toContainText('portals: HTTP 502');
    expect(errors).toEqual([]);
  });

  test('a run with changes: new tokens and issuers are links, details are listed', async ({ page }) => {
    const errors = watchErrors(page);
    await page.goto('/updates/2');
    await hydrated(page);
    await expect(page.getByRole('heading', { level: 2, name: 'What changed' })).toBeVisible();
    const newTokens = page.getByRole('heading', { level: 3, name: 'New tokenized assets' }).locator('xpath=following-sibling::ul[1]');
    await expect(newTokens.getByRole('link')).toHaveCount(2);
    await expect(newTokens.getByRole('link').first()).toHaveAttribute('href', /^\/tokens\/\d+$/);
    await expect(page.getByRole('heading', { level: 3, name: 'Removed tokenized assets' })).toBeVisible();
    await expect(page.getByText('CMC #999999')).toBeVisible(); // removed token that is no longer in the export
    const issuers = page.getByRole('heading', { level: 3, name: 'New issuers' }).locator('xpath=following-sibling::ul[1]');
    await expect(issuers.getByRole('link').first()).toHaveAttribute('href', /^\/issuers\//);
    const byNetwork = page
      .getByRole('heading', { level: 3, name: 'Eligible positions by network (change)' })
      .locator('xpath=following-sibling::ul[1]');
    await expect(byNetwork.getByRole('listitem').first()).toContainText(/: [+-]\d/);
    await expect(page.getByRole('heading', { level: 3, name: 'Eligible positions by protocol (change)' })).toBeVisible();
    await expect(page.getByRole('heading', { level: 3, name: 'Reclassified tokenized assets' })).toBeVisible();
    await expect(page.getByText(/eligible_by|reclassified|by_network/)).toHaveCount(0); // API keys are never printed
    await expect(page.getByText('No warnings.')).toBeVisible();
    // a skipped asset is not "ok"
    await page.getByRole('switch', { name: 'Show only assets that are not ok' }).check();
    await expect(page.getByRole('table', { name: 'Underlying assets in this run' }).locator('tbody tr')).toHaveCount(1);
    // tickers of the export are links
    await page.getByRole('switch', { name: 'Show only assets that are not ok' }).uncheck();
    await expect(page.getByRole('table', { name: 'Underlying assets in this run' }).getByRole('link').first()).toHaveAttribute(
      'href',
      /^\/assets\//
    );
    expect(errors).toEqual([]);
  });

  test('Chinese detail page', async ({ page }) => {
    await page.goto('/zh/updates/2');
    await hydrated(page);
    await expect(page.getByRole('heading', { level: 1 })).toHaveText('更新 #2');
    await expect(page.getByRole('heading', { level: 2, name: '变化' })).toBeVisible();
    await expect(page.getByRole('heading', { level: 3, name: '新增代币化资产' })).toBeVisible();
    await expect(page.getByRole('navigation', { name: '面包屑' }).getByRole('link', { name: '数据更新' })).toHaveAttribute(
      'href',
      '/zh/updates'
    );
  });

  test('an unknown run: 200 + noindex from the server, "not found" in the browser (documented behaviour)', async ({ page }) => {
    await page.goto('/updates/999');
    await hydrated(page);
    await expect(page.getByRole('heading', { level: 1 })).toHaveText('Update #999 not found');
    await page.goto('/zh/updates/999');
    await hydrated(page);
    await expect(page.getByRole('heading', { level: 1 })).toHaveText('未找到更新 #999');
  });

  test('the run list answers an API failure with a message, not an empty table', async ({ page }) => {
    await page.route('**/api/v1/runs?*', (route) =>
      route.fulfill({ status: 503, json: { error: { code: 'unavailable', message: 'down' } } })
    );
    await page.goto('/updates');
    await hydrated(page);
    await expect(page.getByRole('alert').filter({ hasText: 'The update history could not be loaded.' })).toBeVisible();
    await expect(page.getByRole('table')).toHaveCount(0);
  });
});
