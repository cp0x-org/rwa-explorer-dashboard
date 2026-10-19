import { createContext, ReactNode, useContext, useEffect, useMemo, useState } from 'react';
import { IntlProvider } from 'react-intl';

import { DEFAULT_LOCALE, Locale, LOCALE_PREFIX, localizePath } from './locale';
import { ENGLISH, getMessages, loadMessages, Messages } from './messages';

export interface LocaleContextValue {
  locale: Locale;
  prefix: string; // '' or '/zh'
  path: (to: string) => string; // locale-free path -> path in this locale
}

const LocaleContext = createContext<LocaleContextValue>({ locale: DEFAULT_LOCALE, prefix: '', path: (to) => to });

export const useLocale = () => useContext(LocaleContext);

// Mounted once per locale in the route tree (routes/index.tsx), so the locale comes from the URL in the browser and in
// the prerender alike. Messages of the locale are registered before the first render (index.tsx, entry-server.tsx);
// the English fallback below only covers an unexpected client-side jump into another locale.
export function LocaleProvider({ locale, children }: { locale: Locale; children: ReactNode }) {
  const [messages, setMessages] = useState<Messages>(() => getMessages(locale) ?? ENGLISH);

  useEffect(() => {
    let cancelled = false;
    if (!getMessages(locale)) loadMessages(locale).then((m) => !cancelled && setMessages(m));
    return () => {
      cancelled = true;
    };
  }, [locale]);

  const value = useMemo<LocaleContextValue>(
    () => ({ locale, prefix: LOCALE_PREFIX[locale], path: (to) => localizePath(to, locale) }),
    [locale]
  );

  return (
    <LocaleContext value={value}>
      <IntlProvider
        locale={locale}
        defaultLocale={DEFAULT_LOCALE}
        messages={messages}
        // a missing id falls back to English (the default message passed by useT); only report in development
        onError={(err) => {
          if (import.meta.env.DEV && err.code !== 'MISSING_TRANSLATION') console.error(err);
        }}
      >
        {children}
      </IntlProvider>
    </LocaleContext>
  );
}
