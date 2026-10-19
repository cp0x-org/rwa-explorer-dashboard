// Locales of the site and their place in the URL. The URL is the only source of the language: `/assets` is English,
// `/zh/assets` is Simplified Chinese. Nothing here depends on the browser.

export type Locale = 'en' | 'zh-CN';

export const DEFAULT_LOCALE: Locale = 'en';
export const LOCALES: readonly Locale[] = ['en', 'zh-CN'];

// URL prefix of each locale; the default locale has none, so every existing English URL stays valid.
export const LOCALE_PREFIX: Record<Locale, string> = { en: '', 'zh-CN': '/zh' };

// Short labels of the switcher ("EN | 中文") and the full name of each language in that language.
export const LOCALE_LABEL: Record<Locale, string> = { en: 'EN', 'zh-CN': '中文' };
export const LOCALE_NAME: Record<Locale, string> = { en: 'English', 'zh-CN': '简体中文' };

// BCP 47 tag for <html lang> and hreflang; Open Graph locale.
export const OG_LOCALE: Record<Locale, string> = { en: 'en_US', 'zh-CN': 'zh_CN' };

const prefixOf = (pathname: string): Locale | undefined =>
  LOCALES.find((l) => {
    const p = LOCALE_PREFIX[l];
    return p && (pathname === p || pathname.startsWith(`${p}/`));
  });

// Locale of a pathname: `/zh` and `/zh/...` are Chinese, everything else English.
export const localeFromPath = (pathname: string): Locale => prefixOf(pathname) ?? DEFAULT_LOCALE;

// The same pathname without its locale prefix: `/zh/assets/NVDA` -> `/assets/NVDA`, `/zh` -> `/`.
export function stripLocale(pathname: string): string {
  const locale = prefixOf(pathname);
  if (!locale) return pathname;
  return pathname.slice(LOCALE_PREFIX[locale].length) || '/';
}

// A locale-free path (with optional query and hash) in the given locale: `/assets?x=1` -> `/zh/assets?x=1`.
export function localizePath(path: string, locale: Locale): string {
  const prefix = LOCALE_PREFIX[locale];
  if (!prefix || !path.startsWith('/')) return path;
  if (path === '/') return prefix;
  return prefix + path;
}

// The equivalent of a pathname in another locale, keeping query and hash.
export const switchLocale = (pathname: string, locale: Locale, search = '', hash = ''): string =>
  localizePath(stripLocale(pathname), locale) + search + hash;
