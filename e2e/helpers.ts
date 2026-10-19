import { readFileSync } from 'node:fs';
import path from 'node:path';
import type { Page } from '@playwright/test';

export interface Data {
  underlyings: { ticker: string }[];
  tokens: { id: number; ticker: string; symbol: string | null; excluded_reason: string | null }[];
  issuers: { id: string; name: string | null }[];
}

// The same export the build baked into dist/.
export const data: Data = JSON.parse(readFileSync(path.resolve(__dirname, '..', 'public', 'data', 'rwa.json'), 'utf8'));

export const SAMPLE = {
  ticker: 'NVDA',
  token: data.tokens.find((t) => t.ticker === 'NVDA' && t.excluded_reason === null)?.id ?? data.tokens[0].id,
  issuer: data.issuers[0].id
};

export const ROUTES = [
  '/assets',
  `/assets/${SAMPLE.ticker}`,
  '/tokenized',
  `/tokens/${SAMPLE.token}`,
  '/issuers',
  `/issuers/${SAMPLE.issuer}`,
  '/opportunities',
  '/summary'
];

export const CJK = /[一-鿿]/;

// Collects console errors and page errors (hydration mismatches are console errors in React 19).
export function watchErrors(page: Page): string[] {
  const errors: string[] = [];
  page.on('console', (msg) => {
    if (msg.type() === 'error') errors.push(msg.text());
  });
  page.on('pageerror', (err) => errors.push(err.message));
  return errors;
}

// Waits until React has hydrated the prerendered page: the language links keep working either way, but the sort
// buttons only respond after hydration, so the test waits for the data load + hydration to finish.
export async function hydrated(page: Page) {
  await page.waitForFunction(() => document.documentElement.dataset.hydrated === 'true', null, { timeout: 20_000 });
}
