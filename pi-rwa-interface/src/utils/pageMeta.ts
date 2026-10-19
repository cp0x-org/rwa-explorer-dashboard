import { createIntl, createIntlCache, IntlShape } from 'react-intl';

import { RwaExport } from 'types/rwa';
import {
  DEFAULT_LOCALE,
  ENGLISH,
  getFormat,
  getMessages,
  Locale,
  localeFromPath,
  LOCALES,
  localizePath,
  MessageId,
  stripLocale
} from 'i18n';

export const SITE_NAME = 'RWA Explorer'; // title suffix and site name; the publisher stays cp0x

export interface Crumb {
  name: string;
  path: string; // locale-free
}

export interface PageMeta {
  locale: Locale;
  path: string; // locale-free pathname, e.g. /assets/NVDA
  title: string;
  description: string;
  keywords: string;
  // false for an unknown asset, token or issuer: the page exists in the app but is not a real page
  found: boolean;
  crumbs: Crumb[]; // breadcrumb trail, last item is the page itself (empty on top-level pages)
  // the same page in every locale, for hreflang and the switcher
  alternates: { locale: Locale; path: string }[];
}

const cache = createIntlCache();
const intls = new Map<Locale, IntlShape>();

// A react-intl instance outside React: the prerender and `document.title` on navigation use the same strings as the UI.
export function intlFor(locale: Locale): IntlShape {
  let intl = intls.get(locale);
  if (!intl) {
    intl = createIntl({ locale, defaultLocale: DEFAULT_LOCALE, messages: getMessages(locale) ?? ENGLISH, onError: () => {} }, cache);
    intls.set(locale, intl);
  }
  return intl;
}

// Title, description and breadcrumbs of a page: written into the prerendered <head> and set as document.title on
// navigation. `pathname` may carry the locale prefix (/zh/assets) or not.
export function pageMeta(pathname: string, data: RwaExport): PageMeta {
  const locale = localeFromPath(pathname);
  const path = stripLocale(pathname);
  const intl = intlFor(locale);
  const f = getFormat(locale);
  const t = (id: MessageId, values?: Record<string, string | number>) => intl.formatMessage({ id, defaultMessage: ENGLISH[id] }, values);

  const s = data.summary;
  const basket = t('meta.basket', {
    underlyings: f.num(s.underlyings_count),
    tokens: f.num(s.tokens_count),
    issuers: f.num(s.issuers_count),
    positions: f.num(s.eligible_opportunities_count),
    date: f.day(s.newest_fetched_at)
  });
  const alternates = LOCALES.map((l) => ({ locale: l, path: localizePath(path, l) }));
  const page = (title: string, description: string, crumbs: Crumb[] = [], found = true): PageMeta => ({
    locale,
    path,
    title: `${title} | ${SITE_NAME}`,
    description,
    keywords: t('meta.keywords'),
    found,
    crumbs,
    alternates
  });
  const [section, id, ...rest] = path.split('/').filter(Boolean);
  if (rest.length) return page(t('meta.notFound.title'), basket, [], false); // deeper paths are never pages

  if (section === 'assets' && id) {
    const u = data.underlyings.find((x) => x.ticker === decodeURIComponent(id));
    if (u) {
      const name = u.cmc_name ?? u.name;
      return page(
        t('meta.asset.title', { name, ticker: u.ticker }),
        t('meta.asset.description', {
          versions: f.num(u.versions_count),
          name,
          ticker: u.ticker,
          issuers: f.num(u.issuers_count),
          positions: f.num(u.eligible_opportunities_count),
          protocols: f.num(u.eligible_protocols_count)
        }),
        [
          { name: t('nav.assets'), path: '/assets' },
          { name: u.ticker, path }
        ]
      );
    }
  }
  if (section === 'tokens' && id) {
    const tk = data.tokens.find((x) => String(x.id) === id);
    if (tk) {
      const symbol = tk.symbol ?? `CMC #${tk.id}`;
      const issuer = tk.issuer_name ?? t('meta.token.unknownIssuer');
      return page(
        t('meta.token.title', { symbol, issuer, ticker: tk.ticker }),
        t('meta.token.description', {
          symbol,
          name: tk.name ? t('meta.token.nameSuffix', { name: tk.name }) : '',
          ticker: tk.ticker,
          issuer,
          positions: f.num(tk.counts.eligible_opportunities_count),
          protocols: f.num(tk.counts.eligible_protocols_count)
        }),
        [
          { name: t('nav.assets'), path: '/assets' },
          { name: tk.ticker, path: `/assets/${encodeURIComponent(tk.ticker)}` },
          { name: symbol, path }
        ]
      );
    }
  }
  if (section === 'issuers' && id) {
    const i = data.issuers.find((x) => x.id === id);
    if (i) {
      const name = i.name ?? t('common.unknownIssuer');
      return page(
        t('meta.issuer.title', { name }),
        t('meta.issuer.description', {
          name,
          tokens: f.num(i.basket_tokens_count),
          underlyings: f.num(i.basket_underlyings_count),
          positions: f.num(i.issuer_usefulness_score)
        }),
        [
          { name: t('nav.issuers'), path: '/issuers' },
          { name, path }
        ]
      );
    }
  }

  switch (section) {
    case undefined:
      return page(t('meta.home.title'), basket);
    case 'assets':
      if (!id) return page(t('meta.assets.title'), t('meta.assets.description', { basket }));
      break;
    case 'tokenized':
      if (!id) return page(t('meta.tokenized.title'), t('meta.tokenized.description', { basket }));
      break;
    case 'issuers':
      if (!id) return page(t('meta.issuers.title'), t('meta.issuers.description', { basket }));
      break;
    case 'opportunities':
      if (!id) return page(t('meta.positions.title'), t('meta.positions.description', { basket }));
      break;
    case 'summary':
      if (!id) return page(t('meta.summary.title'), t('meta.summary.description', { basket }));
      break;
  }
  // unknown asset, token, issuer or path: the page says "not found" and must not be indexed
  return page(t('meta.notFound.title'), basket, [], false);
}
