import AxeBuilder from '@axe-core/playwright';
import { expect, test } from '@playwright/test';

import { hydrated, ROUTES, SAMPLE } from './helpers';

const PAGES = [...ROUTES, ...ROUTES.map((r) => `/zh${r}`)];

test.describe('accessibility', () => {
  for (const path of PAGES) {
    test(`${path} has no critical or serious axe violations`, async ({ page }) => {
      await page.goto(path);
      await hydrated(page);
      const results = await new AxeBuilder({ page })
        .withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa', 'best-practice'])
        .analyze();
      const serious = results.violations.filter((v) => v.impact === 'critical' || v.impact === 'serious');
      expect(
        serious.map(
          (v) =>
            `${v.id} (${v.impact}): ${v.help}\n  ${v.nodes
              .slice(0, 3)
              .map((n) => n.html.slice(0, 120))
              .join('\n  ')}`
        ),
        path
      ).toEqual([]);
    });
  }

  test('document structure: one h1, landmarks, no duplicate active navigation', async ({ page }) => {
    for (const path of ['/assets', `/zh/tokens/${SAMPLE.token}`, '/summary']) {
      await page.goto(path);
      await hydrated(page);
      expect(await page.getByRole('heading', { level: 1 }).count(), path).toBe(1);
      expect(await page.getByRole('banner').count(), path).toBe(1);
      expect(await page.getByRole('main').count(), path).toBe(1);
      expect(await page.getByRole('contentinfo').count(), path).toBe(1);
      // the section navigation exists twice in the markup (desktop strip / phone grid) but only once per viewport
      const sections = page.getByRole('navigation', { name: path.startsWith('/zh') ? '栏目' : 'Sections' });
      expect(await sections.count(), path).toBe(1);
      await expect(sections.locator('a[aria-current="page"]')).toHaveCount(1);
      // metric values are not headings
      const headings = await page.getByRole('heading').allTextContents();
      expect(
        headings.filter((h) => /^[\d$.,%\s/—-]+$/.test(h.trim())),
        path
      ).toEqual([]);
    }
  });

  test('keyboard: skip link, section navigation and sortable headers', async ({ page }) => {
    await page.goto('/assets');
    await hydrated(page);
    await page.keyboard.press('Tab');
    await expect(page.getByRole('link', { name: 'Skip to content' })).toBeFocused();
    await page.keyboard.press('Enter');
    await expect(page.locator('main')).toHaveAttribute('id', 'main');

    const sections = page.getByRole('navigation', { name: 'Sections' });
    const issuers = sections.getByRole('link', { name: 'Issuers' });
    await issuers.focus();
    await page.keyboard.press('Enter');
    await page.waitForURL('**/issuers');
    await expect(issuers).toHaveAttribute('aria-current', 'page');

    const scoreHeader = page.getByRole('columnheader', { name: /^Score/ });
    const sortButton = scoreHeader.getByRole('button');
    await sortButton.focus();
    await page.keyboard.press('Enter');
    await expect(scoreHeader).toHaveAttribute('aria-sort', 'ascending');
    // the accessible name of the sort button is the column label, not the hint
    await expect(sortButton).toHaveAccessibleName(/^Score/);
  });

  test('tables, controls and charts have accessible names', async ({ page, isMobile }) => {
    await page.goto(`/tokens/${SAMPLE.token}`);
    await hydrated(page);
    if (isMobile) await page.getByRole('button', { name: /^Filters/ }).click(); // phones fold the filters
    for (const name of ['Contract deployments of the token', /^Positions:/]) {
      await expect(page.getByRole('table', { name })).toBeVisible();
    }
    await expect(page.getByRole('combobox', { name: 'Status' })).toBeVisible();
    await expect(page.getByRole('combobox', { name: 'Link' })).toBeVisible();
    await expect(page.getByRole('combobox', { name: 'Filter by network' })).toBeVisible();
    await expect(page.getByRole('button', { name: /^Copy address 0x/ }).first()).toBeAttached();
    await expect(page.getByRole('navigation', { name: 'Breadcrumb' }).getByRole('link', { name: 'Underlying assets' })).toBeVisible();

    await page.goto('/summary');
    await hydrated(page);
    expect(await page.locator('svg:not([aria-hidden="true"]) [tabindex]').count()).toBe(0);
    await expect(page.getByRole('region', { name: 'Protocol diversity' }).getByRole('link').first()).toBeVisible();
  });

  test('mobile menu is operable and named', async ({ page, isMobile }) => {
    test.skip(!isMobile, 'phone layout only');
    await page.goto('/zh/assets');
    await hydrated(page);
    await expect(page.locator('header').getByRole('navigation', { name: '语言' })).toBeVisible();
    const open = page.getByRole('button', { name: '打开菜单' });
    await open.click();
    const dialog = page.getByRole('dialog');
    await expect(dialog).toBeVisible();
    await expect(dialog.getByRole('heading', { level: 2 })).toHaveText('菜单');
    await dialog.getByRole('button', { name: '关闭菜单' }).click();
    await expect(dialog).toBeHidden();
    // the phone section grid is the only section navigation in the accessibility tree
    expect(await page.getByRole('navigation', { name: '栏目' }).count()).toBe(1);
  });
});
