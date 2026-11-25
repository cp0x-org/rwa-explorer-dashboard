import { expect, test } from '@playwright/test';

import { hydrated, SAMPLE, watchErrors } from './helpers';

// History charts load in the browser after hydration (mock-api: 10 daily points; `?points=N` is a mock-only switch that
// keeps the last N points, used here through a rewritten request to reach the one-point and no-point states).

const HISTORY = '**/api/v1/history/**';

async function limitPoints(page: import('@playwright/test').Page, points: number) {
  await page.route(HISTORY, (route) => {
    const url = new URL(route.request().url());
    if (url.pathname.endsWith('/metrics')) return route.continue();
    url.searchParams.set('points', String(points));
    return route.continue({ url: url.toString() });
  });
}

test.describe('history charts', () => {
  test('the server renders the chart cards as placeholders: no history request, no chart', async ({ request }) => {
    for (const path of ['/summary', `/tokens/${SAMPLE.token}`]) {
      const html = await (await request.get(path)).text();
      expect(html, path).toContain('data-history-state="loading"');
      expect(html, path).toContain('Loading history');
      expect(html, path).not.toContain('data-history-chart');
      expect(html, path).toContain('aria-label="Time range"');
    }
  });

  test('/summary: sparkline stat cards and a chart with legend, range and metric selector', async ({ page }) => {
    const errors = watchErrors(page);
    await page.goto('/summary');
    await hydrated(page);

    const trends = page.getByRole('region', { name: 'Trends' });
    await expect(trends.locator('dd')).toHaveCount(4);
    await expect(trends.locator('dt').first()).toHaveText('Eligible positions');
    await expect(trends.locator('dd svg[aria-hidden="true"]')).toHaveCount(4); // sparklines are decorative ...
    await expect(trends.locator('dd').first()).toContainText(/from [\d,]+ on .* to [\d,]+ on .*lowest/); // ... their text is real

    const card = page.getByRole('region', { name: 'History', exact: true });
    const chart = card.locator('svg[data-history-chart]');
    await expect(chart).toBeVisible();
    await expect(chart).toHaveAttribute('aria-label', /: line chart, 10 points$/);
    await expect(card.getByRole('list', { name: 'Legend' }).getByRole('listitem')).toHaveText(['Eligible positions']);
    await expect(card.locator('[data-history-state]')).toHaveAttribute('data-history-state', 'chart');
    // text alternative: first and last value, lowest and highest
    await expect(card.getByText(/^Eligible positions: from [\d,]+ on .+ to [\d,]+ on .+; lowest [\d,]+, highest [\d,]+\.$/)).toBeAttached();
    // one y axis for one line: formatted numbers; several lines say they have their own scales
    await card.getByRole('button', { name: 'Tokenized market cap' }).click();
    await expect(card.getByRole('button', { name: 'Tokenized market cap' })).toHaveAttribute('aria-pressed', 'true');
    await expect(card.getByRole('list', { name: 'Legend' }).getByRole('listitem')).toHaveCount(2);
    await expect(card.getByText(/^Left axis: Eligible positions\./)).toBeVisible();
    // the last selected metric cannot be switched off
    await card.getByRole('button', { name: 'Eligible positions' }).click();
    await card.getByRole('button', { name: 'Tokenized market cap' }).click(); // now the only one: stays on
    await expect(card.getByRole('button', { name: 'Tokenized market cap' })).toHaveAttribute('aria-pressed', 'true');
    await expect(card.getByRole('list', { name: 'Legend' }).getByRole('listitem')).toHaveCount(1);
    expect(errors).toEqual([]);
  });

  test('the range toggle changes the request', async ({ page }) => {
    await page.goto('/summary');
    await hydrated(page);
    const card = page.getByRole('region', { name: 'History', exact: true });
    await expect(card.locator('svg[data-history-chart]')).toBeVisible();
    const group = card.getByRole('group', { name: 'Time range' });
    await expect(group.getByRole('button', { name: '90 days' })).toHaveAttribute('aria-pressed', 'true');

    const request30 = page.waitForRequest((r) => /history\/summary/.test(r.url()) && new URL(r.url()).searchParams.has('from'));
    await group.getByRole('button', { name: '30 days' }).click();
    const from30 = new URL((await request30).url()).searchParams.get('from')!;
    expect(from30).toMatch(/^\d{4}-\d{2}-\d{2}$/);
    await expect(group.getByRole('button', { name: '30 days' })).toHaveAttribute('aria-pressed', 'true');

    // "all" is an explicit early date: the API's default `from` is a year back
    const requestAll = page.waitForRequest(
      (r) => /history\/summary/.test(r.url()) && new URL(r.url()).searchParams.get('from') === '2000-01-01T00:00:00Z'
    );
    await group.getByRole('button', { name: 'All time' }).click();
    await requestAll;
    await expect(card.locator('svg[data-history-chart]')).toBeVisible();
  });

  test('/tokens/:id: chart with points reachable by keyboard', async ({ page }) => {
    const errors = watchErrors(page);
    await page.goto(`/tokens/${SAMPLE.token}`);
    await hydrated(page);
    const card = page.getByRole('region', { name: 'History', exact: true });
    const chart = card.locator('svg[data-history-chart]');
    await expect(chart).toBeVisible();
    await expect(card.getByRole('button', { name: 'Market cap' })).toHaveAttribute('aria-pressed', 'true');
    for (const name of ['Price', 'Eligible positions', 'Max reported APY', 'Median reported APY'])
      await expect(card.getByRole('button', { name })).toBeVisible();

    // one Tab stop for the whole chart (the newest point), arrow keys move along the points
    const stops = chart.locator('[role="img"][tabindex="0"]');
    await expect(stops).toHaveCount(1);
    await stops.focus();
    const last = await stops.getAttribute('aria-label');
    expect(last).toMatch(/UTC: Market cap \$/);
    await page.keyboard.press('ArrowLeft');
    const previous = await chart.locator('[role="img"]:focus').getAttribute('aria-label');
    expect(previous).not.toBe(last);
    await page.keyboard.press('Home');
    await expect(chart.locator('[role="img"]:focus')).toHaveAttribute('aria-label', /Oct (9|10), 2026/);
    // the tooltip follows the focus (decorative copy of the label)
    await expect(card.locator('[aria-hidden="true"]').filter({ hasText: 'Market cap:' })).toBeVisible();
    expect(errors).toEqual([]);
  });

  test('/assets/:ticker and /issuers/:id carry their charts, in Chinese too', async ({ page }) => {
    await page.goto(`/assets/${SAMPLE.ticker}`);
    await hydrated(page);
    const asset = page.getByRole('region', { name: 'History', exact: true });
    await expect(asset.locator('svg[data-history-chart]')).toBeVisible();
    for (const name of ['Tokenized market cap', 'Eligible positions', 'Tokens in DeFi', 'Tokenized versions', 'Issuers'])
      await expect(asset.getByRole('button', { name })).toBeVisible();

    await page.goto(`/zh/issuers/${SAMPLE.issuer}`);
    await hydrated(page);
    const issuer = page.getByRole('region', { name: '历史', exact: true });
    await expect(issuer.locator('svg[data-history-chart]')).toBeVisible();
    await expect(issuer.getByRole('group', { name: '时间范围' }).getByRole('button', { name: '30 天' })).toBeVisible();
    await expect(issuer.getByRole('button', { name: '发行方评分' })).toBeVisible();
  });

  test('one day of history: no card, and no Trends section on /summary', async ({ page }) => {
    await limitPoints(page, 1);
    await page.goto(`/tokens/${SAMPLE.token}`);
    await hydrated(page);
    await expect(page.getByRole('region', { name: 'History', exact: true })).toHaveCount(0);
    await expect(page.getByText(/^History starts on/)).toHaveCount(0);

    await page.goto('/summary');
    await hydrated(page);
    await expect(page.getByRole('region', { name: 'History', exact: true })).toHaveCount(0);
    await expect(page.getByRole('heading', { name: 'Trends' })).toHaveCount(0);
  });

  test('no history at all: no card either, in Chinese too', async ({ page }) => {
    await limitPoints(page, 0);
    await page.goto('/zh/summary');
    await hydrated(page);
    await expect(page.getByRole('region', { name: '历史', exact: true })).toHaveCount(0);
    await page.goto(`/assets/${SAMPLE.ticker}`);
    await hydrated(page);
    await expect(page.getByRole('region', { name: 'History', exact: true })).toHaveCount(0);
    await expect(page.getByText('No history yet.')).toHaveCount(0);
  });

  test('a failed range switch shows the error, not the previous range under the new button', async ({ page }) => {
    await page.route(HISTORY, (route) => {
      if (new URL(route.request().url()).searchParams.get('from') === '2000-01-01T00:00:00Z') {
        return route.fulfill({ status: 503, json: { error: { code: 'unavailable', message: 'down' } } });
      }
      return route.continue();
    });
    await page.goto('/summary');
    await hydrated(page);
    const card = page.getByRole('region', { name: 'History', exact: true });
    await expect(card.locator('svg[data-history-chart]')).toBeVisible();
    await card.getByRole('group', { name: 'Time range' }).getByRole('button', { name: 'All time' }).click();
    await expect(card.getByText('The history is not available right now.')).toBeVisible();
    await expect(card.locator('svg[data-history-chart]')).toHaveCount(0);
  });

  test('the trend cards show their values in the server HTML (no dashes), only the sparklines come later', async ({ request }) => {
    const html = await (await request.get('/summary')).text();
    const trends = html.slice(html.indexOf('trends-title'), html.indexOf('trends-title') + 4000);
    expect(trends).not.toContain('>—<');
    expect(trends).toMatch(/Eligible positions/);
  });

  test('a failing history API shows a message and the page keeps working', async ({ page }) => {
    await page.route(HISTORY, (route) => route.fulfill({ status: 503, json: { error: { code: 'unavailable', message: 'down' } } }));
    await page.goto('/summary');
    await hydrated(page);
    const card = page.getByRole('region', { name: 'History', exact: true });
    await expect(card.getByText('The history is not available right now.')).toBeVisible();
    await expect(page.getByRole('heading', { level: 1 })).toBeVisible();
  });

  test('the position dialog shows liquidity and APY trends', async ({ page }) => {
    await page.goto('/opportunities');
    await hydrated(page);
    const first = page.getByRole('table', { name: 'DeFi positions' }).locator('tbody tr').first().getByRole('button').first();
    await first.click();
    const dialog = page.getByRole('dialog');
    const trend = dialog.getByRole('region', { name: 'Position history' });
    await expect(trend.locator('svg[aria-hidden="true"]')).toHaveCount(2);
    await expect(trend).toContainText('Liquidity');
    await expect(trend).toContainText('APY');
  });
});
