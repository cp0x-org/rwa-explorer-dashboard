import { readFileSync } from 'node:fs';
import path from 'node:path';
import { expect, Page, test } from '@playwright/test';

import en from '../src/i18n/messages/en';
import { hydrated, watchErrors } from './helpers';

interface Link {
  url: string;
  kind: 'position' | 'home';
}
interface Referral {
  url: string;
  deal: string;
  bonus: string | null;
  page_url: string | null;
  as_of: string;
}
interface Export {
  opportunities: { key: string; link: Link | null }[];
  tokens: {
    id: number;
    symbol: string | null;
    issuer_id: string | null;
    excluded_reason: string | null;
    relations: { key: string }[];
    links?: { kind: string; label: string | null; url: string; network: string }[];
  }[];
  issuers: { id: string; name: string | null; app?: { url: string; label: string | null } | null; referral?: Referral | null }[];
  summary: { links_deals_max_age_days?: number | null };
}

// The same export the build baked into dist/. An export from before the links layer has none of the link fields.
const DATA_PATH = path.resolve(__dirname, '..', 'public', 'data', 'rwa.json');
const data: Export = JSON.parse(readFileSync(DATA_PATH, 'utf8'));
const linkOf = new Map(data.opportunities.map((o) => [o.key, o.link]));

// A token with a swap link whose every position opens directly in its protocol.
const linked = data.tokens.find(
  (t) =>
    t.excluded_reason === null &&
    (t.links ?? []).length > 0 &&
    t.relations.length > 0 &&
    t.relations.every((r) => linkOf.get(r.key)?.kind === 'position')
);
// An issuer with its own app and a live cp0x deal, and one of its tokens.
const issuer = data.issuers.find((i) => i.app && i.referral);
const issuerToken = issuer && data.tokens.find((t) => t.issuer_id === issuer.id && t.excluded_reason === null);

// The referral is shown only while the deals cache is fresh: the tests pin the viewer's clock relative to the cache,
// so an unchanged snapshot does not fail as it ages.
const asOf = issuer ? Date.parse(issuer.referral!.as_of) : NaN;
const maxAgeDays = data.summary.links_deals_max_age_days ?? null;
const expiry = maxAgeDays === null ? NaN : asOf + maxAgeDays * 86_400_000; // the last moment the referral is fresh
const whileFresh = (page: Page) => page.clock.install({ time: new Date(asOf + 86_400_000) });

// Serves the page a changed export: the pages reached by in-site navigation are rendered from it.
const serveChanged = (page: Page, change: (body: Export) => void) =>
  page.route('**/data/rwa.json', async (route) => {
    const body: Export = JSON.parse(readFileSync(DATA_PATH, 'utf8'));
    change(body);
    await route.fulfill({ json: body });
  });

const openIssuerFromTheList = async (page: Page, id: string) => {
  await page.goto('/issuers');
  await hydrated(page);
  await page.locator(`a[href="/issuers/${id}"]`).first().click();
  await page.waitForURL(`**/issuers/${id}`);
};

const expectExternal = async (link: ReturnType<import('@playwright/test').Page['getByRole']>, href: string, sponsored = false) => {
  await expect(link).toHaveAttribute('href', href);
  await expect(link).toHaveAttribute('target', '_blank');
  const rel = (await link.getAttribute('rel')) ?? '';
  expect(rel.split(/\s+/)).toEqual(expect.arrayContaining(['noopener', 'noreferrer']));
  expect(rel.split(/\s+/).includes('sponsored')).toBe(sponsored);
};

test.describe('outbound links', () => {
  test('a position opens in its protocol from the table and from the details dialog', async ({ page }) => {
    test.skip(!linked, 'the export has no token with direct position links');
    const errors = watchErrors(page);
    await page.goto(`/tokens/${linked!.id}`);
    await hydrated(page);

    const table = page.getByRole('table', { name: /^Positions: / });
    const first = table.locator('tbody tr').first();
    const rowLink = first.getByRole('link', { name: /^Open position in / });
    await expect(rowLink).toBeVisible();
    const href = await rowLink.getAttribute('href');
    expect([...linkOf.values()].some((l) => l?.url === href)).toBe(true);
    await expectExternal(rowLink, href!);

    // the row link is a link, not a way into the dialog
    await expect(page.getByRole('dialog')).toBeHidden();
    await first.getByRole('button').first().click();
    const dialog = page.getByRole('dialog');
    await expect(dialog).toBeVisible();
    await expectExternal(dialog.getByRole('link', { name: /^Open position in / }), href!);
    await page.keyboard.press('Escape');

    // the token is obtainable where it has a pool
    const swap = linked!.links![0];
    await expectExternal(page.getByRole('link', { name: new RegExp(`^Swap on ${swap.label}`) }).first(), swap.url);
    expect(errors).toEqual([]);
  });

  test('issuer app is a plain link and the cp0x referral is a separate, marked one', async ({ page }) => {
    test.skip(!issuer, 'the export has no issuer with an app and a deal');
    const errors = watchErrors(page);
    await whileFresh(page);
    await page.goto(`/issuers/${issuer!.id}`);
    await hydrated(page);

    await expectExternal(page.getByRole('link', { name: new RegExp(`^Open ${issuer!.app!.label}`) }), issuer!.app!.url);
    const referral = page.getByRole('link', { name: /cp0x referral link/ });
    await expectExternal(referral, issuer!.referral!.url, true);
    if (issuer!.referral!.bonus) await expect(page.getByText(issuer!.referral!.bonus, { exact: false }).first()).toBeVisible();
    if (issuer!.referral!.page_url) {
      await expect(page.getByRole('link', { name: /Deal details on cp0x\.com/ })).toHaveAttribute('href', issuer!.referral!.page_url);
    }
    // next to a direct link the note says the commission is optional
    await expect(page.getByText(en['link.referral.note'], { exact: true })).toBeVisible();
    expect(errors).toEqual([]);
  });

  test('the static page carries no referral: the offer appears only after the live freshness check', async ({ page, request }) => {
    test.skip(!issuer, 'the export has no issuer with an app and a deal');
    // what a crawler or a visitor without JavaScript gets, however old the build is
    const html = await (await request.get(`/issuers/${issuer!.id}`)).text();
    expect(html).toContain(issuer!.app!.url);
    expect(html).not.toContain(issuer!.referral!.url);
    expect(html).not.toContain('cp0x referral link');
    // the live page adds it once the viewer's clock says the deals cache is fresh
    await whileFresh(page);
    await page.goto(`/issuers/${issuer!.id}`);
    await hydrated(page);
    await expect(page.getByRole('link', { name: /cp0x referral link/ })).toBeVisible();
  });

  test('a referral disappears at the moment its deals cache expires, on a page left open', async ({ page }) => {
    test.skip(!issuer || !Number.isFinite(expiry), 'the export has no dated referral with a max age');
    await page.clock.install({ time: new Date(expiry - 60_000) });
    await page.goto(`/issuers/${issuer!.id}`);
    await hydrated(page);
    const referral = page.getByRole('link', { name: /cp0x referral link/ });
    await expect(referral).toBeVisible();
    await page.clock.fastForward(120_000);
    await expect(referral).toHaveCount(0);
    await expect(page.getByText(en['link.referral.note'], { exact: true })).toHaveCount(0);
    await expect(page.getByRole('link', { name: new RegExp(`^Open ${issuer!.app!.label}`) })).toBeVisible();
  });

  test('a tab that slept through the expiry drops the referral when it wakes up', async ({ page }) => {
    test.skip(!issuer || !Number.isFinite(expiry), 'the export has no dated referral with a max age');
    await whileFresh(page);
    await page.goto(`/issuers/${issuer!.id}`);
    await hydrated(page);
    const referral = page.getByRole('link', { name: /cp0x referral link/ });
    await expect(referral).toBeVisible();
    // the machine slept: the clock jumped, no timer fired
    await page.clock.setSystemTime(new Date(expiry + 60_000));
    await page.evaluate(() => document.dispatchEvent(new Event('visibilitychange')));
    await expect(referral).toHaveCount(0);
  });

  test('the deal details link is shown only for a page of the deal on cp0x.com', async ({ page }) => {
    test.skip(!issuer, 'the export has no issuer with an app and a deal');
    await whileFresh(page);
    await serveChanged(page, (body) => {
      body.issuers.find((i) => i.id === issuer!.id)!.referral!.page_url = 'https://evil.example/deals/hyperliquid';
    });
    await openIssuerFromTheList(page, issuer!.id);
    await expect(page.getByRole('link', { name: /cp0x referral link/ })).toBeVisible();
    await expect(page.getByRole('link', { name: /Deal details on cp0x\.com/ })).toHaveCount(0);
    await expect(page.locator('a[href*="evil.example"]')).toHaveCount(0);
  });

  test('a referral with no direct link next to it does not promise one', async ({ page }) => {
    test.skip(!issuer, 'the export has no issuer with an app and a deal');
    await whileFresh(page);
    await serveChanged(page, (body) => {
      body.issuers.find((i) => i.id === issuer!.id)!.app = null;
    });
    await openIssuerFromTheList(page, issuer!.id);
    await expect(page.getByRole('link', { name: /cp0x referral link/ })).toBeVisible();
    await expect(page.getByRole('link', { name: new RegExp(`^Open ${issuer!.app!.label}`) })).toHaveCount(0);
    await expect(page.getByText(en['link.referral.note.only'], { exact: true })).toBeVisible();
    await expect(page.getByText(/direct link works without it/)).toHaveCount(0);
  });

  test('a referral whose deals cache has aged since the build disappears; the direct link stays', async ({ page }) => {
    test.skip(!issuer, 'the export has no issuer with an app and a deal');
    await page.clock.install({ time: new Date('2031-01-01T00:00:00Z') });
    await page.goto(`/issuers/${issuer!.id}`);
    await hydrated(page);
    await expect(page.getByRole('link', { name: /cp0x referral link/ })).toHaveCount(0);
    await expect(page.getByRole('link', { name: new RegExp(`^Open ${issuer!.app!.label}`) })).toBeVisible();
  });

  test('a token page links to the issuer app', async ({ page }) => {
    test.skip(!issuerToken, 'the export has no token of an issuer with an app');
    await whileFresh(page);
    await page.goto(`/tokens/${issuerToken!.id}`);
    await hydrated(page);
    const name = new RegExp(`^Get ${issuerToken!.symbol} at ${issuer!.app!.label}`);
    await expectExternal(page.getByRole('link', { name }), issuer!.app!.url);
    await expect(page.getByRole('link', { name: /cp0x referral link/ })).toBeVisible();
  });

  test('the Chinese pages carry the same links', async ({ page }) => {
    test.skip(!issuer, 'the export has no issuer with an app and a deal');
    await whileFresh(page);
    await page.goto(`/zh/issuers/${issuer!.id}`);
    await hydrated(page);
    await expect(page.locator(`a[href="${issuer!.app!.url}"]`).first()).toBeVisible();
    await expect(page.locator(`a[href="${issuer!.referral!.url}"]`).first()).toHaveAttribute('rel', /sponsored/);
  });
});
