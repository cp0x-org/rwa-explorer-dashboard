import { useEffect } from 'react';
import { useLocation } from 'react-router-dom';

import { useT } from 'i18n';
import { RwaExport } from 'types/rwa';
import { trackPageView } from 'utils/analytics';
import { SITE_NAME } from 'utils/pageMeta';
import { documentHead } from 'utils/seo';

interface HeadElement {
  attributes: Record<string, string>;
  textContent?: string;
}

// Reuse existing elements, create missing ones and remove stale entries and duplicates.
function syncHeadElements(tagName: string, selector: string, entries: HeadElement[]) {
  const existing = Array.from(document.head.querySelectorAll(selector));
  entries.forEach(({ attributes, textContent }, i) => {
    const element = existing[i] ?? document.createElement(tagName);
    Object.entries(attributes).forEach(([name, value]) => element.setAttribute(name, value));
    if (textContent !== undefined) element.textContent = textContent;
    if (!existing[i]) document.head.appendChild(element);
  });
  existing.slice(entries.length).forEach((element) => element.remove());
}

function syncMeta(attribute: 'name' | 'property', key: string, values: string[]) {
  syncHeadElements(
    'meta',
    `meta[${attribute}="${key}"]`,
    values.map((content) => ({ attributes: { [attribute]: key, content } }))
  );
}

// Keep the live head in sync with the server-rendered one, only after hydration; then count the page view.
export function useDocumentHead(data?: RwaExport) {
  const { pathname } = useLocation();
  const t = useT();
  useEffect(() => {
    if (!data) {
      document.title = `${t('notFound.page')} | ${SITE_NAME}`;
      trackPageView();
      return;
    }

    const head = documentHead(pathname, data, import.meta.env.VITE_SITE_URL);
    document.title = head.title;
    syncHeadElements('title', 'title', [{ attributes: {}, textContent: head.title }]);
    syncMeta('name', 'title', [head.title]);
    syncMeta('property', 'og:title', [head.title]);
    syncMeta('name', 'twitter:title', [head.title]);
    syncMeta('name', 'description', [head.description]);
    syncMeta('property', 'og:description', [head.description]);
    syncMeta('name', 'twitter:description', [head.description]);
    syncMeta('name', 'keywords', [head.keywords]);

    syncHeadElements(
      'link',
      'link[rel="canonical"]',
      head.canonical === null ? [] : [{ attributes: { rel: 'canonical', href: head.canonical } }]
    );
    syncMeta('property', 'og:url', head.canonical === null ? [] : [head.canonical]);
    syncMeta('property', 'og:locale', head.ogLocale === null ? [] : [head.ogLocale]);
    syncMeta('property', 'og:locale:alternate', head.ogLocaleAlternates);
    syncHeadElements(
      'link',
      'link[rel="alternate"][hreflang]',
      head.hreflang.map(({ lang, href }) => ({ attributes: { rel: 'alternate', hreflang: lang, href } }))
    );
    syncMeta('name', 'robots', head.noindex ? ['noindex'] : []);
    syncHeadElements(
      'script',
      'script[type="application/ld+json"]',
      head.jsonLd === null ? [] : [{ attributes: { type: 'application/ld+json' }, textContent: JSON.stringify(head.jsonLd) }]
    );
    // page view after the head is current, so it carries this page's title (Google Analytics, utils/analytics.ts)
    trackPageView();
  }, [pathname, data, t]);
}
