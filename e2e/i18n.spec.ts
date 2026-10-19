import { expect, test } from '@playwright/test';

import { hydrated, SAMPLE, watchErrors } from './helpers';

test.describe('language', () => {
  test('English is the default on existing URLs, no redirect by browser language', async ({ browser }) => {
    const context = await browser.newContext({ locale: 'zh-CN' });
    const page = await context.newPage();
    const response = await page.goto('/assets');
    expect(response?.status()).toBe(200);
    expect(page.url()).toMatch(/\/assets$/);
    await expect(page.locator('html')).toHaveAttribute('lang', 'en');
    await expect(page.getByRole('heading', { level: 1 })).toHaveText('Underlying Assets');
    await context.close();
  });

  test('the switcher sits in the header, marks the active language and keeps the route and query', async ({ page }) => {
    const errors = watchErrors(page);
    await page.goto(`/opportunities?underlying=${SAMPLE.ticker}`);
    const nav = page.locator('header').getByRole('navigation', { name: 'Language' });
    await expect(nav).toBeVisible();
    const en = nav.getByRole('link', { name: 'English' });
    const zh = nav.getByRole('link', { name: '简体中文' });
    await expect(en).toHaveAttribute('aria-current', 'page');
    await expect(zh).not.toHaveAttribute('aria-current', 'page');
    await expect(zh).toHaveAttribute('hreflang', 'zh-CN');
    await expect(zh).toHaveAttribute('lang', 'zh-CN');
    // top right: the switcher's right edge is near the header's right edge
    const headerBox = (await page.locator('header').boundingBox())!;
    const navBox = (await nav.boundingBox())!;
    expect(headerBox.x + headerBox.width - (navBox.x + navBox.width)).toBeLessThan(120);

    await hydrated(page);
    await zh.click();
    await page.waitForURL(`**/zh/opportunities?underlying=${SAMPLE.ticker}`);
    await expect(page.locator('html')).toHaveAttribute('lang', 'zh-CN');
    await expect(page.getByRole('heading', { level: 1 })).toHaveText('DeFi 头寸');
    await expect(page.getByRole('navigation', { name: '语言' }).getByRole('link', { name: '简体中文' })).toHaveAttribute(
      'aria-current',
      'page'
    );
    // the filter from the query is applied in the Chinese page too
    await expect(page.getByRole('button', { name: SAMPLE.ticker }).first()).toBeVisible();

    await page.getByRole('navigation', { name: '语言' }).getByRole('link', { name: 'English' }).click();
    await page.waitForURL(`**/opportunities?underlying=${SAMPLE.ticker}`);
    await expect(page.locator('html')).toHaveAttribute('lang', 'en');
    expect(errors).toEqual([]);
  });

  test('Chinese pages hydrate without errors and stay Chinese on client-side navigation', async ({ page }) => {
    const errors = watchErrors(page);
    await page.goto(`/zh/assets/${SAMPLE.ticker}`);
    await hydrated(page);
    await expect(page.getByRole('heading', { level: 1 })).toHaveText(SAMPLE.ticker);
    await expect(page.getByRole('heading', { level: 2, name: '代币化资产' })).toBeVisible();
    // client-side navigation through the section nav
    await page.getByRole('navigation', { name: '栏目' }).getByRole('link', { name: '发行方' }).click();
    await page.waitForURL('**/zh/issuers');
    await expect(page.getByRole('heading', { level: 1 })).toHaveText('发行方');
    await expect(page).toHaveTitle(/发行方/);
    // a row link keeps the prefix
    const first = page.locator('table tbody tr').first().getByRole('link').first();
    const href = await first.getAttribute('href');
    expect(href).toMatch(/^\/zh\/issuers\//);
    await page.goBack();
    await page.waitForURL(`**/zh/assets/${SAMPLE.ticker}`);
    await expect(page.getByRole('heading', { level: 1 })).toHaveText(SAMPLE.ticker);
    expect(errors).toEqual([]);
  });

  test('tickers, symbols and addresses are not translated', async ({ page }) => {
    await page.goto(`/zh/tokens/${SAMPLE.token}`);
    await expect(page.getByRole('heading', { level: 1 })).not.toHaveText(/[一-鿿]/);
    const addresses = await page.locator('table').first().locator('tbody td:nth-child(3)').allTextContents();
    expect(addresses.length).toBeGreaterThan(0);
    for (const a of addresses) expect(a).toMatch(/^\S{6}\.\.\.\S{4}/); // shortened (EVM, Solana, TON...), never translated
  });
});
