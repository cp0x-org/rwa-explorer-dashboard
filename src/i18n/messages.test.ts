import { describe, expect, it } from 'vitest';
import { createIntl } from 'react-intl';

import en from './messages/en';
import zh from './messages/zh';

const ids = Object.keys(en) as (keyof typeof en)[];

// Top-level ICU argument names of a message: {n}, {n, plural, ...}, {date}; the branches of a plural are skipped.
function args(message: string): Set<string> {
  const names = new Set<string>();
  let depth = 0;
  for (let i = 0; i < message.length; i += 1) {
    const ch = message[i];
    if (ch === '{') {
      if (depth === 0) {
        const m = /^\{\s*([A-Za-z_]\w*)\s*[,}]/.exec(message.slice(i));
        if (m) names.add(m[1]);
      }
      depth += 1;
    } else if (ch === '}') depth -= 1;
  }
  return names;
}

// Values that make every argument renderable.
const sample = (message: string) => Object.fromEntries(Array.from(args(message), (a) => [a, 3]));

const TICKERS = ['NVDA', 'TSLA', 'AAPL', 'USDC', 'NVDAon', 'NVDAX', 'xStock', 'Ondo', 'Backed'];

describe('dictionaries', () => {
  it('Chinese defines exactly the English ids', () => {
    expect(Object.keys(zh).sort()).toEqual([...ids].sort());
  });

  it('no message is empty', () => {
    for (const id of ids) {
      expect(en[id].trim(), `en ${id}`).not.toBe('');
      expect(zh[id].trim(), `zh ${id}`).not.toBe('');
    }
  });

  it('Chinese messages use the same ICU arguments as English', () => {
    for (const id of ids) {
      expect(args(zh[id]), id).toEqual(args(en[id]));
    }
  });

  it('every message compiles and renders in its locale', () => {
    const intlEn = createIntl({ locale: 'en', messages: en });
    const intlZh = createIntl({ locale: 'zh-CN', messages: zh });
    for (const id of ids) {
      expect(() => intlEn.formatMessage({ id }, sample(en[id])), `en ${id}`).not.toThrow();
      expect(() => intlZh.formatMessage({ id }, sample(zh[id])), `zh ${id}`).not.toThrow();
    }
  });

  it('Chinese messages are translated (contain CJK) unless they are identifiers or symbols', () => {
    const cjk = /[一-鿿]/;
    const latinWords = (m: string) => /[a-z]{3,}/i.test(m.replace(/\{[^}]*\}/g, ''));
    const untranslated = ids.filter((id) => !cjk.test(zh[id]) && latinWords(zh[id]));
    // allowed: brand/product names and technical labels that stay in Latin script
    expect(untranslated.sort()).toEqual(['asset.isin', 'category.dex_lp', 'header.pi', 'token.cmc'].sort());
  });

  it('tickers, symbols and addresses never appear in UI messages', () => {
    for (const id of ids) {
      for (const ticker of TICKERS) {
        expect(en[id].split(/\W+/), `en ${id}`).not.toContain(ticker);
        expect(zh[id].split(/\W+/), `zh ${id}`).not.toContain(ticker);
      }
      expect(en[id]).not.toMatch(/0x[0-9a-fA-F]{6,}/);
    }
  });

  it('plural messages have an "other" branch', () => {
    for (const id of ids) {
      for (const m of [en[id], zh[id]]) {
        if (m.includes('plural')) expect(m, id).toMatch(/other\s*\{/);
      }
    }
  });
});
