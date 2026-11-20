import { useMemo } from 'react';

import { Locale } from './locale';
import { useLocale } from './LocaleProvider';

// null / undefined = not reported -> "—"; a real zero stays "0".
export const DASH = '—';

const toNumber = (value: string | number | null | undefined): number | null => {
  if (value === null || value === undefined) return null;
  const n = Number(value);
  return Number.isFinite(n) ? n : null;
};

// Locale-bound formatters built on Intl. Only the presentation depends on the locale; the numbers never change.
export function makeFormat(locale: Locale) {
  const integer = new Intl.NumberFormat(locale);
  const fixed2 = new Intl.NumberFormat(locale, { minimumFractionDigits: 2, maximumFractionDigits: 2 });
  const upTo2 = new Intl.NumberFormat(locale, { minimumFractionDigits: 0, maximumFractionDigits: 2 });
  const pctCache = new Map<number, Intl.NumberFormat>();
  const pctFormat = (digits: number) => {
    let f = pctCache.get(digits);
    if (!f) {
      f = new Intl.NumberFormat(locale, { minimumFractionDigits: digits, maximumFractionDigits: digits });
      pctCache.set(digits, f);
    }
    return f;
  };
  const dateTime = new Intl.DateTimeFormat(locale, {
    timeZone: 'UTC',
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false
  });
  const monthDayOnly = new Intl.DateTimeFormat(locale, { timeZone: 'UTC', month: 'short', day: 'numeric' });
  const dateOnly = new Intl.DateTimeFormat(locale, { timeZone: 'UTC', year: 'numeric', month: 'short', day: 'numeric' });
  const relative = new Intl.RelativeTimeFormat(locale, { numeric: 'always', style: 'narrow' });
  const unit = (u: 'second' | 'minute' | 'hour') =>
    new Intl.NumberFormat(locale, { style: 'unit', unit: u, unitDisplay: 'narrow', maximumFractionDigits: 1 });
  const [secondsFormat, minutesFormat, hoursFormat] = [unit('second'), unit('minute'), unit('hour')];

  // $1.23K / $4.56M / $7.89B: compact USD amounts, same thresholds as before.
  const shortUsd = (n: number): string => {
    if (n < 1000) return fixed2.format(n);
    if (n < 1e6) return `${fixed2.format(n / 1e3)}K`;
    if (n < 1e9) return `${fixed2.format(n / 1e6)}M`;
    return `${upTo2.format(n / 1e9)}B`;
  };

  return {
    locale,
    num: (value: number | string | null | undefined): string => {
      const n = toNumber(value);
      return n === null ? DASH : integer.format(n);
    },
    usd: (value: string | number | null | undefined): string => {
      const n = toNumber(value);
      if (n === null) return DASH;
      return n < 0 ? `-$${shortUsd(-n)}` : `$${shortUsd(n)}`;
    },
    pct: (value: string | number | null | undefined, digits = 2): string => {
      const n = toNumber(value);
      return n === null ? DASH : `${pctFormat(digits).format(n)}%`;
    },
    date: (iso: string | null | undefined): string => {
      if (!iso) return DASH;
      const d = new Date(iso);
      return Number.isNaN(d.getTime()) ? DASH : `${dateTime.format(d)} UTC`;
    },
    // 12.5 s / 9m 0s style: "4.2s", "9m 5s", "1h 3m" in the locale's narrow units
    duration: (seconds: number | string | null | undefined): string => {
      const n = toNumber(seconds);
      if (n === null || n < 0) return DASH;
      if (n < 60) return secondsFormat.format(Math.round(n * 10) / 10);
      const total = Math.round(n);
      if (total < 3600) return `${minutesFormat.format(Math.floor(total / 60))} ${secondsFormat.format(total % 60)}`;
      return `${hoursFormat.format(Math.floor(total / 3600))} ${minutesFormat.format(Math.floor((total % 3600) / 60))}`;
    },
    monthDay: (iso: string | null | undefined): string => {
      if (!iso) return DASH;
      const d = new Date(iso);
      return Number.isNaN(d.getTime()) ? DASH : monthDayOnly.format(d);
    },
    day: (iso: string | null | undefined): string => {
      if (!iso) return DASH;
      const d = new Date(iso);
      return Number.isNaN(d.getTime()) ? DASH : dateOnly.format(d);
    },
    // "5m ago" / "3h ago" / "2d ago" in English, "5分钟前" in Chinese: depends on the viewer's clock.
    age: (iso: string | null | undefined, now: Date = new Date()): string => {
      if (!iso) return DASH;
      const minutes = Math.round((now.getTime() - new Date(iso).getTime()) / 60000);
      if (!Number.isFinite(minutes)) return DASH;
      if (minutes < 60) return relative.format(-Math.max(minutes, 0), 'minute');
      if (minutes < 48 * 60) return relative.format(-Math.round(minutes / 60), 'hour');
      return relative.format(-Math.round(minutes / 1440), 'day');
    }
  };
}

export type Format = ReturnType<typeof makeFormat>;

const cache = new Map<Locale, Format>();

export const getFormat = (locale: Locale): Format => {
  let f = cache.get(locale);
  if (!f) {
    f = makeFormat(locale);
    cache.set(locale, f);
  }
  return f;
};

export function useFormat(): Format {
  const { locale } = useLocale();
  return useMemo(() => getFormat(locale), [locale]);
}
