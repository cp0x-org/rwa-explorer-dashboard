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
        expect(title).toMatch(/\| RWA Explorer$/);
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

test.describe('for crawlers and AI agents', () => {
  test('robots.txt lets every crawler read the site, closes only the admin API and names the sitemap and llms.txt', async ({ request }) => {
    const res = await request.get('/robots.txt');
    expect(res.status()).toBe(200);
    const robots = await res.text();
    expect(robots).toMatch(/^User-agent: \*$/m);
    expect(robots).toMatch(/^Allow: \/$/m);
    expect(robots).toMatch(/^Disallow: \/api\/v1\/admin\/$/m);
    expect(robots).not.toMatch(/^Disallow: \/$/m);
    expect(robots).toContain(`Sitemap: ${SITE}/sitemap.xml`);
    expect(robots).toContain(`${SITE}/llms.txt`);
  });

  test('llms.txt maps the data, the API and every asset and issuer page with absolute links', async ({ request }) => {
    const res = await request.get('/llms.txt');
    expect(res.status()).toBe(200);
    expect(res.headers()['content-type']).toMatch(/^text\/plain/);
    const text = await res.text();
    expect(text.split('\n')[0]).toBe('# RWA Explorer');
    expect(text).toContain(`(${SITE}/api/v1/export)`);
    expect(text).toContain(`(${SITE}/api/v1/openapi.yaml)`);
    for (const route of ROUTES.filter((r) => /^\/(assets|issuers)\/./.test(r))) expect(text).toContain(`(${SITE}${route})`);
  });

  test('pages name the API description, and /favicon.ico is the icon they link', async ({ request }) => {
    const html = await (await request.get('/assets')).text();
    expect(html).toContain('<link rel="service-desc" type="application/yaml" href="/api/v1/openapi.yaml"');
    expect(html).not.toContain('This HTML file is a template');
    const icon = await request.get('/favicon.ico');
    expect(icon.status()).toBe(200);
    expect(icon.headers()['content-type']).toBe('image/png');
  });
});
