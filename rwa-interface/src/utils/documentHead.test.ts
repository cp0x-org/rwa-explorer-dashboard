import { readFileSync } from 'node:fs';
import { beforeAll, describe, expect, it } from 'vitest';

import { RwaExport } from 'types/rwa';
import { registerMessages } from 'i18n';
import zh from 'i18n/messages/zh';
import { pageMeta } from './pageMeta';
import { documentHead, headLinks, jsonLd } from './seo';

// The live <head> after in-app navigation must equal what scripts/prerender.mjs writes for the page. These are the
// prerender's rules (render() in src/entry-server.tsx and headTags() in the script), stated once more as the oracle.
const SITE = 'https://rwa.example';
let data: RwaExport;

beforeAll(() => {
  registerMessages('zh-CN', zh);
  data = JSON.parse(readFileSync(new URL('../../public/data/rwa.json', import.meta.url), 'utf8'));
});

const prerendered = (path: string, siteUrl: string | undefined) => {
  const meta = pageMeta(path, data);
  const links = siteUrl && meta.found ? headLinks(meta, siteUrl) : null;
  return {
    title: meta.title,
    description: meta.description,
    keywords: meta.keywords,
    canonical: links?.canonical ?? null,
    hreflang: links?.hreflang ?? [],
    ogLocale: links?.ogLocale ?? null,
    ogLocaleAlternates: links?.ogLocaleAlternates ?? [],
    noindex: !meta.found,
    jsonLd: siteUrl ? jsonLd(meta, data, siteUrl) : null
  };
};

const pages = () => {
  const ticker = data.underlyings[0].ticker;
  const token = data.tokens.find((t) => t.excluded_reason === null)!;
  return [
    '/assets',
    `/assets/${ticker}`,
    `/tokens/${token.id}`,
    `/issuers/${encodeURIComponent(data.issuers[0].id)}`,
    '/zh/assets',
    `/zh/assets/${ticker}`
  ];
};

describe('documentHead', () => {
  it('is the prerendered head of every kind of page, in both languages', () => {
    for (const path of pages()) {
      expect(documentHead(path, data, SITE), path).toEqual(prerendered(path, SITE));
    }
  });

  it('changes with the page: two pages never share a canonical link or JSON-LD', () => {
    const [list, asset, tokenPage] = pages().map((p) => documentHead(p, data, SITE));
    expect(new Set([list.canonical, asset.canonical, tokenPage.canonical]).size).toBe(3);
    expect(asset.jsonLd).not.toEqual(list.jsonLd);
    expect(asset.hreflang.map((h) => h.href)).not.toEqual(list.hreflang.map((h) => h.href));
  });

  it('reads the site address the way the prerender does: trailing slashes do not matter', () => {
    const path = pages()[1];
    expect(documentHead(path, data, `${SITE}/`)).toEqual(documentHead(path, data, SITE));
    expect(documentHead(path, data, `${SITE}//`)).toEqual(prerendered(path, SITE));
  });

  it('without a site address has no canonical, hreflang, og:url, og:locale or JSON-LD, like the prerender', () => {
    for (const siteUrl of [undefined, '']) {
      for (const path of pages()) {
        const head = documentHead(path, data, siteUrl);
        expect(head, path).toEqual(prerendered(path, undefined));
        expect(head.canonical).toBeNull();
        expect(head.hreflang).toEqual([]);
        expect(head.jsonLd).toBeNull();
      }
    }
  });

  it('a page that is not found is noindex and has no canonical, hreflang or JSON-LD even with a site address', () => {
    for (const path of ['/assets/NO-SUCH-TICKER', '/tokens/999999999', '/assets/NVDA/deeper', '/zh/issuers/no-such-issuer']) {
      const head = documentHead(path, data, SITE);
      expect(head, path).toEqual(prerendered(path, SITE));
      expect(head.noindex).toBe(true);
      expect(head.canonical).toBeNull();
      expect(head.hreflang).toEqual([]);
      expect(head.jsonLd).toBeNull();
    }
    expect(documentHead('/assets/NO-SUCH-TICKER', data, SITE).title).toBe('Page not found | RWA Explorer');
  });

  it('a found page is never noindex', () => {
    for (const path of pages()) expect(documentHead(path, data, SITE).noindex, path).toBe(false);
  });
});
