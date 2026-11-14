import { expect, Page, test } from '@playwright/test';

import { data, hydrated, SAMPLE } from './helpers';

// Everything the prerender writes into <head> for one page, read the same way from a live document and from a static file.
const EXTRACT = `(doc) => {
  const attr = (selector, name) => doc.querySelector(selector)?.getAttribute(name) ?? null;
  const many = (selector, name) => [...doc.querySelectorAll(selector)].map((n) => n.getAttribute(name) ?? '').sort();
  const ld = doc.querySelector('script[type="application/ld+json"]')?.textContent ?? null;
  return {
    title: doc.title,
    lang: doc.documentElement.getAttribute('lang'),
    metaTitle: attr('meta[name="title"]', 'content'),
    description: attr('meta[name="description"]', 'content'),
    keywords: attr('meta[name="keywords"]', 'content'),
    ogTitle: attr('meta[property="og:title"]', 'content'),
    ogDescription: attr('meta[property="og:description"]', 'content'),
    twitterTitle: attr('meta[name="twitter:title"]', 'content'),
    twitterDescription: attr('meta[name="twitter:description"]', 'content'),
    canonical: attr('link[rel="canonical"]', 'href'),
    ogUrl: attr('meta[property="og:url"]', 'content'),
    ogLocale: attr('meta[property="og:locale"]', 'content'),
    ogLocaleAlternates: many('meta[property="og:locale:alternate"]', 'content'),
    hreflang: [...doc.querySelectorAll('link[rel="alternate"][hreflang]')]
      .map((n) => n.getAttribute('hreflang') + ' ' + n.getAttribute('href'))
      .sort(),
    robots: attr('meta[name="robots"]', 'content'),
    jsonLd: ld ? JSON.parse(ld) : null,
    jsonLdCount: doc.querySelectorAll('script[type="application/ld+json"]').length,
    canonicalCount: doc.querySelectorAll('link[rel="canonical"]').length
  };
}`;

type Head = Record<string, unknown>;

const liveHead = (page: Page): Promise<Head> => page.evaluate(`(${EXTRACT})(document)`);

const staticHead = async (page: Page, path: string): Promise<Head> => {
  const html = await (await page.request.get(path)).text();
  return page.evaluate(`(${EXTRACT})(new DOMParser().parseFromString(${JSON.stringify(html)}, 'text/html'))`);
};

// After an in-app navigation the head must be the one the prerender wrote for that page (canonical, hreflang, og:url,
// descriptions, titles, robots, JSON-LD), not the one of the first page opened.
const expectHeadOf = async (page: Page, path: string) => {
  const expected = await staticHead(page, path);
  expect(expected.canonical, `the prerendered ${path} has a canonical link`).toBeTruthy();
  await expect.poll(() => liveHead(page), { message: path }).toEqual(expected);
};

const token = data.tokens.find((t) => t.id === SAMPLE.token)!;

test.describe('page head on in-app navigation', () => {
  test('an English visit through the site keeps the head of every page', async ({ page }) => {
    await page.goto('/assets');
    await hydrated(page);
    await expectHeadOf(page, '/assets');

    await page.locator(`a[href="/assets/${SAMPLE.ticker}"]`).first().click();
    await page.waitForURL(`**/assets/${SAMPLE.ticker}`);
    await expectHeadOf(page, `/assets/${SAMPLE.ticker}`);

    await page.locator(`a[href="/tokens/${token.id}"]`).first().click();
    await page.waitForURL(`**/tokens/${token.id}`);
    await expectHeadOf(page, `/tokens/${token.id}`);

    await page.goBack();
    await page.waitForURL(`**/assets/${SAMPLE.ticker}`);
    await expectHeadOf(page, `/assets/${SAMPLE.ticker}`);
  });

  test('a Chinese visit keeps the Chinese head', async ({ page }) => {
    await page.goto('/zh/assets');
    await hydrated(page);
    await page.locator(`a[href="/zh/assets/${SAMPLE.ticker}"]`).first().click();
    await page.waitForURL(`**/zh/assets/${SAMPLE.ticker}`);
    await expectHeadOf(page, `/zh/assets/${SAMPLE.ticker}`);
  });

  test('switching the language keeps the head of the same page in the other language', async ({ page }) => {
    await page.goto(`/assets/${SAMPLE.ticker}`);
    await hydrated(page);
    await page.locator('header').getByRole('link', { name: '中文' }).first().click();
    await page.waitForURL(`**/zh/assets/${SAMPLE.ticker}`);
    await expectHeadOf(page, `/zh/assets/${SAMPLE.ticker}`);
  });

  test('an in-app move to a page that is not found drops the links of the previous page and says noindex', async ({ page }) => {
    await page.goto(`/assets/${SAMPLE.ticker}`);
    await hydrated(page);
    // no link leads to a missing token: move there through the router's own history listener
    await page.evaluate(() => {
      window.history.pushState({}, '', '/tokens/999999999');
      window.dispatchEvent(new PopStateEvent('popstate'));
    });
    await page.waitForURL('**/tokens/999999999');
    await expect
      .poll(() => liveHead(page))
      .toMatchObject({
        title: 'Page not found | RWA Explorer',
        robots: 'noindex',
        canonical: null,
        canonicalCount: 0,
        ogUrl: null,
        ogLocale: null,
        ogLocaleAlternates: [],
        hreflang: [],
        jsonLd: null,
        jsonLdCount: 0
      });
  });
});
