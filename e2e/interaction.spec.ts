import { expect, test } from '@playwright/test';

import { hydrated, SAMPLE, watchErrors } from './helpers';

test.describe('hydrated pages keep working', () => {
  test('sorting, filters, paging and the details dialog', async ({ page }) => {
    const errors = watchErrors(page);
    await page.goto('/opportunities');
    await hydrated(page);
    expect(errors, 'no hydration errors').toEqual([]);

    const table = page.getByRole('table', { name: 'DeFi positions' });
    const apyHeader = table.getByRole('columnheader', { name: /^Reported APY/ });
    await expect(apyHeader).toHaveAttribute('aria-sort', 'descending');
    await apyHeader.getByRole('button').click();
    await expect(apyHeader).toHaveAttribute('aria-sort', 'ascending');
    const liquidityHeader = table.getByRole('columnheader', { name: /^Liquidity/ });
    await liquidityHeader.getByRole('button').click();
    await expect(liquidityHeader).toHaveAttribute('aria-sort', 'descending');
    await expect(apyHeader).not.toHaveAttribute('aria-sort', /.+/);

    // status select has an accessible name and changes the result count
    const status = page.getByRole('combobox', { name: 'Status' });
    const before = await page.getByRole('status').textContent();
    await status.click();
    await page.getByRole('option', { name: 'All confirmed (raw)' }).click();
    await expect(page.getByRole('status')).not.toHaveText(before!);

    // paging
    const pager = page.getByRole('navigation', { name: 'Table pages' });
    await pager.getByRole('button', { name: 'Go to page 2' }).click();
    await expect(pager.getByRole('button', { name: 'Page 2' })).toHaveAttribute('aria-current', 'page');

    // the position name is a button that opens the details dialog; Escape closes it
    const first = table.locator('tbody tr').first().getByRole('button').first();
    await first.focus();
    await page.keyboard.press('Enter');
    const dialog = page.getByRole('dialog');
    await expect(dialog).toBeVisible();
    await expect(dialog.getByRole('heading', { level: 2 })).toBeVisible();
    await expect(dialog.getByText('Reported APY', { exact: true })).toBeVisible();
    await page.keyboard.press('Escape');
    await expect(dialog).toBeHidden();
    expect(errors).toEqual([]);
  });

  test('charts on the summary page work by keyboard and link to pages', async ({ page }) => {
    const errors = watchErrors(page);
    await page.goto('/summary');
    await hydrated(page);
    // donut legend rows are links
    const share = page.getByRole('region', { name: 'Share of eligible positions' });
    const legendLink = share.getByRole('link').first();
    await expect(legendLink).toHaveAttribute('href', /^\/issuers\//);
    // a hidden text summary exists for the chart
    await expect(share.getByText(/eligible positions: \d/)).toBeAttached();
    // stacked bar rows are buttons that select the asset
    const stacked = page.getByRole('region', { name: 'Eligible positions by issuer' });
    const second = stacked.getByRole('button').nth(1);
    const ticker = (await second.textContent())!.match(/[A-Z.]{2,8}/)![0];
    await second.focus();
    await page.keyboard.press('Enter');
    await page.waitForURL(`**/summary?asset=${ticker}`);
    await expect(second).toHaveAttribute('aria-pressed', 'true');
    await expect(page.getByRole('heading', { level: 3, name: ticker, exact: true })).toBeVisible();
    // deep link
    await page.goto(`/summary?asset=${SAMPLE.ticker}`);
    await hydrated(page);
    await expect(page.getByRole('heading', { level: 3, name: SAMPLE.ticker, exact: true })).toBeVisible();
    expect(errors).toEqual([]);
  });

  test('methodology is a disclosure, not a hover-only tooltip', async ({ page }) => {
    await page.goto('/zh/assets');
    await hydrated(page);
    const button = page.getByRole('button', { name: '评分方法说明' });
    await expect(button).toHaveAttribute('aria-expanded', 'false');
    await button.focus();
    await page.keyboard.press('Enter');
    await expect(button).toHaveAttribute('aria-expanded', 'true');
    await expect(page.getByText(/评分 = 包含该代币的符合条件的 DeFi 头寸数量/)).toBeVisible();
  });

  test('every prerendered page hydrates without console errors', async ({ page }) => {
    for (const path of [
      '/assets',
      `/assets/${SAMPLE.ticker}`,
      '/tokenized',
      `/tokens/${SAMPLE.token}`,
      '/issuers',
      `/issuers/${SAMPLE.issuer}`,
      '/summary',
      '/zh/summary',
      `/zh/tokens/${SAMPLE.token}`
    ]) {
      const errors = watchErrors(page);
      await page.goto(path);
      await hydrated(page);
      expect(errors, path).toEqual([]);
    }
  });
});
