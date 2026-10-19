import { readFileSync } from 'node:fs';
import { beforeAll, describe, expect, it } from 'vitest';

import { RwaExport } from 'types/rwa';
import { registerMessages } from 'i18n';
import zh from 'i18n/messages/zh';
import { pageMeta } from './pageMeta';
import { headLinks, jsonLd } from './seo';

const SITE = 'https://rwa.cp0x.com';
let data: RwaExport;

beforeAll(() => {
  registerMessages('zh-CN', zh);
  data = JSON.parse(readFileSync(new URL('../../public/data/rwa.json', import.meta.url), 'utf8'));
});

const routesOf = (d: RwaExport) => [
  '/assets',
  '/tokenized',
  '/issuers',
  '/opportunities',
  '/summary',
  ...d.underlyings.slice(0, 20).map((u) => `/assets/${u.ticker}`),
  ...d.tokens.slice(0, 40).map((t) => `/tokens/${t.id}`),
  ...d.issuers.map((i) => `/issuers/${i.id}`)
];

describe('pageMeta', () => {
  it('gives every page in both languages a unique title and description', () => {
    const titles = new Set<string>();
    const descriptions = new Set<string>();
    for (const route of routesOf(data)) {
      for (const path of [route, `/zh${route}`]) {
        const meta = pageMeta(path, data);
        expect(meta.found, path).toBe(true);
        expect(meta.title.length, path).toBeGreaterThan(10);
        expect(meta.title.length, path).toBeLessThan(120);
        expect(meta.description.length, path).toBeGreaterThan(40);
        expect(titles.has(meta.title), `duplicate title ${meta.title}`).toBe(false);
        expect(descriptions.has(meta.description), `duplicate description for ${path}`).toBe(false);
        titles.add(meta.title);
        descriptions.add(meta.description);
      }
    }
  });

  it('localizes by URL prefix and keeps tickers', () => {
    const en = pageMeta('/assets/NVDA', data);
    const zhMeta = pageMeta('/zh/assets/NVDA', data);
    expect(en.locale).toBe('en');
    expect(zhMeta.locale).toBe('zh-CN');
    expect(en.path).toBe('/assets/NVDA');
    expect(zhMeta.path).toBe('/assets/NVDA');
    expect(zhMeta.title).toMatch(/[一-鿿]/);
    expect(zhMeta.title).toContain('NVDA');
    expect(en.title).toContain('NVDA');
    expect(zhMeta.alternates).toEqual([
      { locale: 'en', path: '/assets/NVDA' },
      { locale: 'zh-CN', path: '/zh/assets/NVDA' }
    ]);
    expect(zhMeta.crumbs.map((c) => c.path)).toEqual(['/assets', '/assets/NVDA']);
  });

  it('marks unknown records and paths as not found', () => {
    expect(pageMeta('/assets/NOPE', data).found).toBe(false);
    expect(pageMeta('/tokens/0', data).found).toBe(false);
    expect(pageMeta('/zh/issuers/nope', data).found).toBe(false);
    expect(pageMeta('/nothing', data).found).toBe(false);
    expect(pageMeta('/assets/NVDA/extra', data).found).toBe(false);
  });
});

describe('seo', () => {
  it('builds self-canonical and bidirectional hreflang links', () => {
    const en = headLinks(pageMeta('/assets/NVDA', data), SITE);
    const zhLinks = headLinks(pageMeta('/zh/assets/NVDA', data), SITE);
    expect(en.canonical).toBe(`${SITE}/assets/NVDA`);
    expect(zhLinks.canonical).toBe(`${SITE}/zh/assets/NVDA`);
    expect(en.hreflang).toEqual(zhLinks.hreflang);
    expect(en.hreflang).toEqual([
      { lang: 'en', href: `${SITE}/assets/NVDA` },
      { lang: 'zh-CN', href: `${SITE}/zh/assets/NVDA` },
      { lang: 'x-default', href: `${SITE}/assets/NVDA` }
    ]);
    expect(zhLinks.ogLocale).toBe('zh_CN');
    expect(zhLinks.ogLocaleAlternates).toEqual(['en_US']);
  });

  it('describes listing pages as a data set with an item list and detail pages with breadcrumbs', () => {
    const list = jsonLd(pageMeta('/zh/assets', data), data, SITE) as any;
    const types = list['@graph'].map((n: any) => n['@type']);
    expect(types).toEqual(['Organization', 'WebSite', 'WebPage', 'Dataset', 'ItemList']);
    const items = list['@graph'][4].itemListElement;
    expect(items.length).toBeLessThanOrEqual(100);
    expect(items[0].url).toMatch(new RegExp(`^${SITE}/zh/assets/`));
    expect(list['@graph'][3].dateModified).toBe(data.summary.newest_fetched_at);

    const detail = jsonLd(pageMeta('/tokens/' + data.tokens[0].id, data), data, SITE) as any;
    const crumbs = detail['@graph'].find((n: any) => n['@type'] === 'BreadcrumbList');
    expect(crumbs.itemListElement.map((i: any) => i.position)).toEqual([1, 2, 3]);
    expect(crumbs.itemListElement[0].item).toBe(`${SITE}/assets`);

    expect(jsonLd(pageMeta('/tokens/0', data), data, SITE)).toBeNull();
    // nothing invented: no ratings, offers or prices
    expect(JSON.stringify(list)).not.toMatch(/AggregateRating|Offer|price/);
  });
});
