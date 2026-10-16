import React, { useState, useEffect } from 'react';

// third party
import { IntlProvider, MessageFormatElement } from 'react-intl';
import useConfig from 'hooks/useConfig';

// types
import { I18n } from 'types/config';

// English is bundled so the first render (and the prerendered HTML) has text; other locales load on demand
import en from 'utils/locales/en.json';

type Messages = Record<string, string> | Record<string, MessageFormatElement[]>;

// load locales files
function loadLocaleData(i18n: I18n) {
  switch (i18n) {
    case 'fr':
      return import('utils/locales/fr.json');
    case 'ro':
      return import('utils/locales/ro.json');
    case 'zh':
      return import('utils/locales/zh.json');
    default:
      return Promise.resolve({ default: en });
  }
}

// ==============================|| LOCALIZATION ||============================== //

interface LocalsProps {
  children: React.ReactNode;
}

export default function Locales({ children }: LocalsProps) {
  const { i18n } = useConfig();
  const [messages, setMessages] = useState<Messages>(en);

  useEffect(() => {
    loadLocaleData(i18n).then((d: { default: Messages }) => {
      setMessages(d.default);
    });
  }, [i18n]);

  return (
    <IntlProvider locale={i18n} defaultLocale="en" messages={messages}>
      {children}
    </IntlProvider>
  );
}
