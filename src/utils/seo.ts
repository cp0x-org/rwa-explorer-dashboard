// Structured data (schema.org JSON-LD) and hreflang links of a page, written into the prerendered <head>
// (scripts/prerender.mjs). Everything comes from the same export that renders the page, nothing is invented:
// no ratings, prices or legal details, only what the page shows.
import { RwaExport } from 'types/rwa';
import { getFormat, localizePath, OG_LOCALE } from 'i18n';
import { intlFor, PageMeta, SITE_NAME } from './pageMeta';

const ORGANIZATION = {
  '@type': 'Organization',
  '@id': 'https://cp0x.com/#organization',
  name: SITE_NAME,
  url: 'https://cp0x.com',
  sameAs: ['https://x.com/cp0xdotcom', 'https://t.me/cp0xdotcom', 'https://github.com/cp0x-org']
};

const ITEM_LIST_MAX = 100;

export interface HeadLinks {
  canonical: string;
  hreflang: { lang: string; href: string }[]; // en, zh-CN and x-default
  ogLocale: string;
  ogLocaleAlternates: string[];
}

export function headLinks(meta: PageMeta, siteUrl: string): HeadLinks {
  const href = (path: string) => siteUrl + path;
  const en = meta.alternates.find((a) => a.locale === 'en')!;
  return {
    canonical: href(localizePath(meta.path, meta.locale)),
    hreflang: [...meta.alternates.map((a) => ({ lang: a.locale, href: href(a.path) })), { lang: 'x-default', href: href(en.path) }],
    ogLocale: OG_LOCALE[meta.locale],
    ogLocaleAlternates: meta.alternates.filter((a) => a.locale !== meta.locale).map((a) => OG_LOCALE[a.locale])
  };
}

// JSON-LD graph of a page. Listing pages describe the data set and list their rows; detail pages carry breadcrumbs.
export function jsonLd(meta: PageMeta, data: RwaExport, siteUrl: string): object | null {
  if (!meta.found) return null;
  const { locale, path } = meta;
  const url = (p: string) => siteUrl + localizePath(p, locale);
  const intl = intlFor(locale);
  const t = (id: Parameters<typeof intl.formatMessage>[0]['id']) => intl.formatMessage({ id });
  const f = getFormat(locale);
  const inLanguage = locale;
  const modified = data.summary.newest_fetched_at ?? data.generated_at;

  const website = {
    '@type': 'WebSite',
    '@id': `${siteUrl}/#website`,
    url: siteUrl + localizePath('/', locale),
    name: t('site.name' as const),
    inLanguage,
    publisher: { '@id': ORGANIZATION['@id'] }
  };

  const webPage = {
    '@type': 'WebPage',
    '@id': url(path),
    url: url(path),
    name: meta.title,
    description: meta.description,
    inLanguage,
    isPartOf: { '@id': website['@id'] },
    dateModified: modified
  };

  const graph: object[] = [ORGANIZATION, website, webPage];

  if (meta.crumbs.length) {
    graph.push({
      '@type': 'BreadcrumbList',
      itemListElement: meta.crumbs.map((c, i) => ({
        '@type': 'ListItem',
        position: i + 1,
        name: c.name,
        item: url(c.path)
      }))
    });
  }

  const [section, id] = path.split('/').filter(Boolean);
  if (!id) {
    const dataset = {
      '@type': 'Dataset',
      '@id': `${siteUrl}/#dataset`,
      name: t('meta.dataset.name' as const),
      description: t('meta.dataset.description' as const),
      url: siteUrl + localizePath('/assets', locale),
      inLanguage,
      creator: { '@id': ORGANIZATION['@id'] },
      dateModified: modified,
      keywords: meta.keywords.split(',').map((k) => k.trim()),
      isAccessibleForFree: true,
      distribution: [{ '@type': 'DataDownload', encodingFormat: 'application/json', contentUrl: `${siteUrl}/data/rwa.json` }],
      variableMeasured: ['eligible DeFi positions', 'reported APY', 'liquidity (USD)', 'market cap (USD)']
    };
    graph.push(dataset);

    const list = (items: { name: string; path: string; detail?: string }[]) => ({
      '@type': 'ItemList',
      itemListOrder: 'https://schema.org/ItemListOrderDescending',
      numberOfItems: items.length,
      itemListElement: items.slice(0, ITEM_LIST_MAX).map((item, i) => ({
        '@type': 'ListItem',
        position: i + 1,
        name: item.name,
        url: url(item.path),
        ...(item.detail && { description: item.detail })
      }))
    });
    if (section === 'assets' || section === undefined) {
      const rows = [...data.underlyings].sort((a, b) => b.eligible_opportunities_count - a.eligible_opportunities_count);
      graph.push(
        list(
          rows.map((u) => ({
            name: `${u.ticker} · ${u.cmc_name ?? u.name}`,
            path: `/assets/${encodeURIComponent(u.ticker)}`,
            detail: `${f.num(u.versions_count)} / ${f.num(u.issuers_count)} / ${f.num(u.eligible_opportunities_count)}`
          }))
        )
      );
    } else if (section === 'tokenized') {
      const rows = data.tokens.filter((tk) => tk.excluded_reason === null).sort((a, b) => b.usefulness_score - a.usefulness_score);
      graph.push(
        list(
          rows.map((tk) => ({
            name: `${tk.symbol ?? `CMC #${tk.id}`} (${tk.ticker})`,
            path: `/tokens/${tk.id}`,
            detail: tk.issuer_name ?? undefined
          }))
        )
      );
    } else if (section === 'issuers') {
      const rows = [...data.issuers].sort((a, b) => b.issuer_usefulness_score - a.issuer_usefulness_score);
      graph.push(list(rows.map((i) => ({ name: i.name ?? i.id, path: `/issuers/${encodeURIComponent(i.id)}` }))));
    }
  }

  return { '@context': 'https://schema.org', '@graph': graph };
}
