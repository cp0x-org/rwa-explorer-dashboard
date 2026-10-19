// Message registry: English is bundled (first render and prerender), Chinese is a separate chunk that index.tsx loads
// before it hydrates a Chinese page, and entry-server.tsx registers statically.
import en from './en';
import type { Locale } from '../locale';

export type MessageId = keyof typeof en;
export type Messages = Record<MessageId, string>;

const registry: Partial<Record<Locale, Messages>> = { en };

export const ENGLISH: Messages = en;

export const getMessages = (locale: Locale): Messages | undefined => registry[locale];

export const registerMessages = (locale: Locale, messages: Messages) => {
  registry[locale] = messages;
};

// Loads the dictionary of a locale (code-split) and registers it; resolves at once when it is already there.
export async function loadMessages(locale: Locale): Promise<Messages> {
  const loaded = registry[locale];
  if (loaded) return loaded;
  if (locale === 'zh-CN') {
    const { default: zh } = await import('./zh');
    registry[locale] = zh;
    return zh;
  }
  return en;
}
