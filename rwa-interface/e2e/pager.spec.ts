import { readFileSync } from 'node:fs';
import path from 'node:path';
import { expect, test } from '@playwright/test';

import { hydrated, watchErrors } from './helpers';

interface Export {
  tokens: {
    id: number;
    excluded_reason: string | null;
    relations: unknown[];
    by_category: Record<string, { raw: number; eligible: number }>;
  }[];
}

const data: Export = JSON.parse(readFileSync(path.resolve(__dirname, '..', 'public', 'data', 'rwa.json'), 'utf8'));

// A token with more than two pages of positions of which only a handful are vaults.
const token = data.tokens.find(
  (t) =>
    t.excluded_reason === null &&
    t.relations.length > 20 &&
    t.by_category.vault &&
    t.by_category.vault.raw >= 1 &&
    t.by_category.vault.raw <= 10
);

test('a filter that shrinks the list brings the positions table back to a page that has rows', async ({ page }) => {
  test.skip(!token, 'the export has no token with a small vault category');
  const errors = watchErrors(page);
  await page.goto(`/tokens/${token!.id}`);
  await hydrated(page);

  const table = page.getByRole('table', { name: /^Positions: / });
  const pager = page.getByRole('navigation', { name: 'Table pages' }).first();
  await pager.getByRole('button', { name: 'Go to page 2' }).click();
  await expect(pager.getByRole('button', { name: 'Page 2' })).toHaveAttribute('aria-current', 'page');

  await page.getByRole('combobox', { name: 'Filter by category' }).click();
  await page.getByRole('option', { name: 'Vault', exact: true }).click();
  await page.keyboard.press('Escape');

  // every vault position of the token is listed; the "nothing found" row is not shown while rows exist
  await expect(table.locator('tbody tr')).toHaveCount(token!.by_category.vault.raw);
  await expect(table.getByText('No confirmed positions in current coverage')).toHaveCount(0);
  expect(errors).toEqual([]);
});
