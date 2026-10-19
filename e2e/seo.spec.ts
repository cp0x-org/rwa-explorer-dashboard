import { expect, test } from '@playwright/test';

import { ROUTES } from './helpers';

const SITE = 'https://rwa.cp0x.com';

const attr = (html: string, re: RegExp) => Array.from(html.matchAll(new RegExp(re.source, 'g')), (m) => m[1]);

test.describe('metadata', () => {
  test('titles and descriptions are unique per page and language', async ({ request }) => {
    const titles = new Set<string>();
    for (const route of ROUTES) {
      for (const path of [route, `/zh${route}`]) {
        const html = await (await request.get(path)).text();
        const title = html.match(/<title>([^<]*)<\/title>/)![1];
        expect(titles.has(title), `${path}: ${title}`).toBe(false);
        titles.add(title);
        expect(title).toMatch(/\| cp0x$/);
        const description = attr(html, /<meta name="description" content="([^"]*)"/)[0];
        expect(description.length).toBeGreaterThan(40);
        expect(attr(html, /<meta property="og:title" content="([^"]*)"/)[0]).toBe(title);
        expect(attr(html, /<meta name="twitter:card" content="([^"]*)"/)[0]).toBe('summary');
      }
    }
  });

  for (const route of ROUTES) {
    test(`${route}: canonical, hreflang and Open Graph agree in both languages`, async ({ request }) => {
      const en = await (await request.get(route)).text();
      const zh = await (await request.get(`/zh${route}`)).text();
      expect(attr(en, /<link rel="canonical" href="([^"]*)"/)).toEqual([`${SITE}${route}`]);
      expect(attr(zh, /<link rel="canonical" href="([^"]*)"/)).toEqual([`${SITE}/zh${route}`]);
      const expected = [
        ['en', `${SITE}${route}`],
        ['zh-CN', `${SITE}/zh${route}`],
        ['x-default', `${SITE}${route}`]
      ];
      for (const html of [en, zh]) {
        const links = Array.from(html.matchAll(/<link rel="alternate" hreflang="([^"]*)" href="([^"]*)"/g), (m) => [m[1], m[2]]);
        expect(links).toEqual(expected);
      }
      expect(attr(en, /<meta property="og:url" content="([^"]*)"/)).toEqual([`${SITE}${route}`]);
      expect(attr(zh, /<meta property="og:url" content="([^"]*)"/)).toEqual([`${SITE}/zh${route}`]);
      expect(attr(en, /<meta property="og:locale" content="([^"]*)"/)).toEqual(['en_US']);
      expect(attr(zh, /<meta property="og:locale" content="([^"]*)"/)).toEqual(['zh_CN']);
      expect(attr(zh, /<meta property="og:locale:alternate" content="([^"]*)"/)).toEqual(['en_US']);
    });
  }

  test('structured data is valid JSON-LD that matches the page', async ({ request }) => {
    for (const path of ['/assets', '/zh/issuers', `/zh${ROUTES[1]}`, ROUTES[3]]) {
      const html = await (await request.get(path)).text();
      const raw = html.match(/<script type="application\/ld\+json">(.*?)<\/script>/s)![1];
      const ld = JSON.parse(raw);
      expect(ld['@context']).toBe('https://schema.org');
      const types = ld['@graph'].map((n: { '@type': string }) => n['@type']);
      expect(types).toContain('WebSite');
      expect(types).toContain('Organization');
      const page = ld['@graph'].find((n: { '@type': string }) => n['@type'] === 'WebPage');
      expect(page.url).toBe(`${SITE}${path}`);
      expect(page.name).toBe(html.match(/<title>([^<]*)<\/title>/)![1].replace(/&amp;/g, '&'));
      if (path.split('/').filter(Boolean).length === (path.startsWith('/zh') ? 2 : 1)) {
        expect(types).toContain('Dataset');
        expect(types).toContain('ItemList');
      } else {
        expect(types).toContain('BreadcrumbList');
      }
      expect(raw).not.toContain('</script');
    }
  });
});
