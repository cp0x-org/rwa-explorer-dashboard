import { expect, Page, test } from '@playwright/test';

import { hydrated, watchErrors } from './helpers';

const API_PORT = Number(process.env.E2E_API_PORT ?? 8180);

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
      expect(html, path).toMatch(/<h2[^>]*>Journal<\/h2>/); // the section heading is in the server shell, the rows are not
      expect(html, path).not.toMatch(/collect\.underlying|run started/);
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
    await expect(page.getByRole('listitem').filter({ hasText: 'Portals search for IREN returned 502 after 3 retries' })).toBeVisible();
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

// The run journal (GET /runs/{id}/events). Run 2 has a finished journal, run 3 an error, run 5 is the mock's live run: it
// grows by a few events every 5 s and turns succeeded after ~60 s of mock uptime (the test restarts that clock first).
test.describe('/updates/:id journal', () => {
  const journal = (page: Page) => page.getByRole('table', { name: 'Run journal: events of this run' });

  test('renders the rows; the level and step filters narrow them; tickers link to the asset', async ({ page }) => {
    const errors = watchErrors(page);
    await page.goto('/updates/2');
    await hydrated(page);
    await expect(page.getByRole('heading', { level: 2, name: 'Journal' })).toBeVisible();
    await expect(page.getByText(/^\d+ events?, 0 errors$/)).toBeVisible();
    const table = journal(page);
    await expect(table).toBeVisible();
    const rows = table.locator('tbody tr');
    const all = await rows.count();
    expect(all).toBeGreaterThan(20);
    await expect(table.getByRole('columnheader', { name: 'Time (UTC)' })).toBeVisible();
    await expect(rows.first().getByRole('cell').first()).toHaveText(/^\d\d:\d\d:\d\d$/);
    // the ticker of `underlying=<ticker>` links to the asset (the Go journal writes `underlying`, never `ticker`)
    const tickerLink = table.getByRole('link').first();
    await expect(tickerLink).toHaveAttribute('href', /^\/assets\/[A-Z]+$/);
    expect(`/assets/${await tickerLink.textContent()}`).toBe(await tickerLink.getAttribute('href'));
    await expect(table).not.toContainText('ticker=');
    await expect(table.getByText('networks=ethereum, base, arbitrum, optimism, polygon +2').first()).toBeVisible(); // arrays: 5 items and +N
    await expect(table.getByText('shares={"ethereum":0.5,"base":0.5}').first()).toBeVisible(); // objects as JSON
    await expect(page.getByRole('status').filter({ hasText: 'The run is over' })).toBeVisible();

    await page.getByRole('button', { name: 'Info', exact: true }).click();
    await expect(page.getByRole('button', { name: 'Info', exact: true })).toBeFocused(); // the toolbar stays mounted, focus stays
    await expect(async () => expect(await rows.count()).toBeLessThan(all)).toPass();
    await expect(table.locator('tbody tr[data-level="debug"]')).toHaveCount(0);
    await page.getByRole('button', { name: 'All', exact: true }).click();
    await expect(rows).toHaveCount(all);

    await page.getByRole('button', { name: 'collect.underlying', exact: true }).click();
    await expect(async () => expect(await rows.count()).toBeLessThan(all)).toPass();
    const steps = await rows.locator('td:nth-child(3)').allTextContents();
    expect(new Set(steps)).toEqual(new Set(['collect.underlying']));
    expect(errors).toEqual([]);
  });

  test('the failed asset of run 3 is an error row, and the header counts it', async ({ page }) => {
    await page.goto('/updates/3');
    await hydrated(page);
    await expect(page.getByText(/^\d+ events, 1 error$/)).toBeVisible();
    await page.getByRole('button', { name: 'Errors', exact: true }).click();
    const rows = journal(page).locator('tbody tr');
    await expect(rows).toHaveCount(1);
    await expect(rows.first()).toContainText('IREN failed');
    await expect(rows.first().getByText('error', { exact: true })).toBeVisible();
  });

  test('Load more follows the cursor: the next request carries after=<last id>', async ({ page }) => {
    const queries: string[] = [];
    await page.route('**/api/v1/runs/2/events**', (route) => {
      const u = new URL(route.request().url());
      queries.push(u.search);
      u.searchParams.set('limit', '10');
      return route.continue({ url: u.toString() });
    });
    await page.goto('/updates/2');
    await hydrated(page);
    const rows = journal(page).locator('tbody tr');
    await expect(rows).toHaveCount(10);
    await page.getByRole('button', { name: 'Load more' }).click();
    await expect(rows).toHaveCount(20);
    expect(queries[0]).not.toContain('after='); // the first page has no cursor
    expect(queries.some((q) => q.includes('after=10'))).toBe(true);
  });

  test('a running run: polls every 5 s (not reset by re-renders), keeps polling while journal_open, then stops', async ({
    page,
    request
  }) => {
    test.setTimeout(150_000);
    expect((await request.get(`http://127.0.0.1:${API_PORT}/__mock/restart-live`)).ok()).toBe(true);
    const events: number[] = [];
    const details: { at: number; cache: string | undefined }[] = [];
    page.on('request', (r) => {
      if (/\/api\/v1\/runs\/5\/events/.test(r.url())) events.push(Date.now());
      else if (/\/api\/v1\/runs\/5$/.test(r.url())) {
        const at = Date.now();
        void r.allHeaders().then((h) => details.push({ at, cache: h['cache-control'] })); // the headers the browser added (fetch cache: 'no-store')
      }
    });
    await page.goto('/updates/5');
    await hydrated(page);
    const rows = journal(page).locator('tbody tr');
    await expect(rows.first()).toBeVisible();
    const first = await rows.count();
    const state = page.getByRole('status').filter({ hasText: 'Collection in progress…' });
    await expect(state).toBeVisible();
    expect(await state.textContent()).toBe('Collection in progress…'); // only the state: no counts in the live region
    const t0 = events[0];

    // re-rendering (the Follow switch) must not reset the timer: the first poll comes within ~7 s
    const follow = page.getByRole('switch', { name: 'Follow the latest events' });
    await follow.check();
    await follow.uncheck();
    await follow.check();
    await expect(async () => expect(events.length).toBeGreaterThanOrEqual(2)).toPass({ timeout: 7000 });
    expect(events[1] - t0).toBeLessThan(7500);
    await expect(async () => expect(await rows.count()).toBeGreaterThan(first)).toPass({ timeout: 8000 });
    expect(new URL(page.url()).pathname).toBe('/updates/5');
    expect(await page.getByRole('status').first().textContent()).toBe('Collection in progress…'); // unchanged by the poll
    const requests = await page.evaluate(() =>
      performance
        .getEntriesByType('resource')
        .filter((e) => /runs\/5\/events/.test(e.name))
        .map((e) => e.name)
    );
    expect(requests[1]).toMatch(/after=\d+/); // later polls ask only for what is new
    expect(details.length).toBeGreaterThanOrEqual(1);

    // the status flips to succeeded after 60 s, the journal stays open for 10 s more: the UI keeps polling
    await expect(page.getByRole('status').filter({ hasText: 'last events are still being written' })).toBeVisible({ timeout: 70_000 });
    const flipped = Date.now();
    await expect(page.getByRole('status').filter({ hasText: 'The run is over.' })).toBeVisible({ timeout: 25_000 });
    const closed = Date.now();
    expect(closed - flipped).toBeGreaterThan(3000); // not ended at the first poll after the status change
    expect(events.filter((t) => t > flipped && t < closed).length).toBeGreaterThanOrEqual(1);
    await expect(rows.last()).toContainText('journal closed');
    // the detail was fetched with the browser cache off while polling, and once more when the journal closed
    expect(details.some((d) => d.cache === 'no-cache')).toBe(true);

    // closed: no more requests, neither for the events nor for the detail
    await page.waitForTimeout(1500);
    const seen = [events.length, details.length];
    await page.waitForTimeout(10_000);
    expect([events.length, details.length]).toEqual(seen);
    await expect(page.getByRole('switch', { name: 'Follow the latest events' })).toHaveCount(0);
  });

  test('an unknown run stays "not found" without a journal; Chinese labels', async ({ page }) => {
    await page.goto('/updates/999');
    await hydrated(page);
    await expect(page.getByRole('heading', { level: 1 })).toHaveText('Update #999 not found');
    await expect(page.getByRole('heading', { level: 2, name: 'Journal' })).toHaveCount(0);
    await page.goto('/zh/updates/2');
    await hydrated(page);
    await expect(page.getByRole('heading', { level: 2, name: '运行日志' })).toBeVisible();
    await expect(page.getByRole('table', { name: '运行日志：本次运行的事件' })).toBeVisible();
  });
});
