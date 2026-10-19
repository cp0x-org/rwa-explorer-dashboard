import { useCallback } from 'react';
import { useIntl } from 'react-intl';

import { ENGLISH, MessageId } from './messages';

export type Values = Record<string, string | number | boolean | null | undefined>;

// Typed translator: `t('nav.assets')`, `t('assets.showing', { n: 3 })`. Unknown ids are compile errors; a key missing
// from the current locale falls back to the English text.
export type T = (id: MessageId, values?: Values) => string;

export function useT(): T {
  const intl = useIntl();
  return useCallback<T>((id, values) => intl.formatMessage({ id, defaultMessage: ENGLISH[id] }, values), [intl]);
}
