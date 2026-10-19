import { describe, expect, it } from 'vitest';

import { localeFromPath, localizePath, stripLocale, switchLocale } from './locale';

describe('locale paths', () => {
  it('reads the locale from the URL only', () => {
    expect(localeFromPath('/assets')).toBe('en');
    expect(localeFromPath('/zh/assets')).toBe('zh-CN');
    expect(localeFromPath('/zh')).toBe('zh-CN');
    expect(localeFromPath('/zhx/assets')).toBe('en'); // only the exact prefix
    expect(localeFromPath('/tokens/zh')).toBe('en');
  });

  it('strips and adds the prefix', () => {
    expect(stripLocale('/zh/assets/NVDA')).toBe('/assets/NVDA');
    expect(stripLocale('/zh')).toBe('/');
    expect(stripLocale('/assets')).toBe('/assets');
    expect(localizePath('/assets/NVDA', 'zh-CN')).toBe('/zh/assets/NVDA');
    expect(localizePath('/assets/NVDA', 'en')).toBe('/assets/NVDA');
    expect(localizePath('/', 'zh-CN')).toBe('/zh');
    expect(localizePath('https://cp0x.com', 'zh-CN')).toBe('https://cp0x.com'); // external links untouched
  });

  it('switches language and keeps the route, query and hash', () => {
    expect(switchLocale('/assets/NVDA', 'zh-CN')).toBe('/zh/assets/NVDA');
    expect(switchLocale('/zh/tokens/42461', 'en', '?x=1', '#top')).toBe('/tokens/42461?x=1#top');
    expect(switchLocale('/zh/opportunities', 'zh-CN', '?issuer=abc')).toBe('/zh/opportunities?issuer=abc');
  });
});
