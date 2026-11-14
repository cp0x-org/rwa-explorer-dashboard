import { describe, expect, it } from 'vitest';

import { dealPageUrl, referralIsFresh, referralRecheckDelay, safeUrl } from './links';

describe('safeUrl', () => {
  it('returns a clean https URL unchanged', () => {
    for (const url of [
      'https://app.uniswap.org',
      'https://app.uniswap.org/explore/pools/base/0xabc',
      'https://app.uniswap.org/swap?chain=base&outputCurrency=0xabc',
      'https://cp0x.com/deals/hyperliquid'
    ]) {
      expect(safeUrl(url)).toBe(url);
    }
  });

  it.each([
    'http://app.uniswap.org',
    'javascript:alert(1)',
    'JAVASCRIPT:alert(1)',
    'data:text/html,<script>alert(1)</script>',
    '//app.uniswap.org',
    '/assets',
    'app.uniswap.org',
    'https://',
    'https:// app.uniswap.org',
    'https://app.uniswap.org/ x',
    'https://app.uniswap.org/\n',
    ' https://app.uniswap.org',
    'https://user:pass@app.uniswap.org',
    'https://app.uniswap.org/"onmouseover="x',
    ''
  ])('rejects %j', (url) => {
    expect(safeUrl(url)).toBeUndefined();
  });

  it('rejects everything that is not a string', () => {
    for (const value of [null, undefined, 5, {}, ['https://app.uniswap.org']]) {
      expect(safeUrl(value)).toBeUndefined();
    }
  });
});

describe('referralIsFresh', () => {
  const asOf = '2026-11-01T00:00:00Z';

  it('is fresh up to and including the max age of the deals cache', () => {
    expect(referralIsFresh(asOf, 14, new Date('2026-11-01T00:00:00Z'))).toBe(true);
    expect(referralIsFresh(asOf, 14, new Date('2026-11-15T00:00:00Z'))).toBe(true);
    expect(referralIsFresh(asOf, 14, new Date('2026-11-15T00:00:01Z'))).toBe(false);
    expect(referralIsFresh(asOf, 14, new Date('2027-01-01T00:00:00Z'))).toBe(false);
  });

  it('a viewer whose clock is behind the build still sees it', () => {
    expect(referralIsFresh(asOf, 14, new Date('2026-10-31T00:00:00Z'))).toBe(true);
  });

  it('without a readable timestamp the referral is not shown', () => {
    for (const bad of [null, undefined, '', 'yesterday', '2026-13-45T00:00:00Z']) {
      expect(referralIsFresh(bad, 14, new Date('2026-11-02T00:00:00Z'))).toBe(false);
    }
  });

  it('an export without a max age leaves the decision to the build', () => {
    expect(referralIsFresh(asOf, null, new Date('2027-01-01T00:00:00Z'))).toBe(true);
    expect(referralIsFresh(asOf, undefined, new Date('2027-01-01T00:00:00Z'))).toBe(true);
  });
});

describe('dealPageUrl', () => {
  it('keeps a cp0x deal page unchanged', () => {
    for (const url of ['https://cp0x.com/deals/hyperliquid', 'https://cp0x.com/deals/ondo-perps']) {
      expect(dealPageUrl(url)).toBe(url);
    }
  });

  it('refuses everything that is not a page of a deal on cp0x.com', () => {
    for (const value of [
      'https://evil.example/deals/hyperliquid',
      'https://cp0x.com.evil.example/deals/hyperliquid',
      'https://deals.cp0x.com/deals/hyperliquid',
      'https://cp0x.com:8443/deals/hyperliquid',
      'https://user@cp0x.com/deals/hyperliquid',
      'http://cp0x.com/deals/hyperliquid',
      'https://cp0x.com',
      'https://cp0x.com/',
      'https://cp0x.com/deals',
      'https://cp0x.com/deals/',
      'https://cp0x.com/dealsx/hyperliquid',
      'https://cp0x.com/deals/hyperliquid?next=https://evil.example',
      'https://cp0x.com/deals/hyperliquid#top',
      'cp0x.com/deals/hyperliquid',
      '',
      null,
      undefined,
      42
    ]) {
      expect(dealPageUrl(value), String(value)).toBeUndefined();
    }
  });
});

describe('referralRecheckDelay', () => {
  const asOf = '2026-11-01T00:00:00Z';
  const expiry = Date.parse('2026-11-15T00:00:00Z'); // the last moment the referral is still fresh

  it('is the time left until the referral stops being fresh', () => {
    const delay = referralRecheckDelay(asOf, 14, new Date(expiry - 60_000))!;
    expect(delay).toBeGreaterThan(60_000);
    expect(delay).toBeLessThanOrEqual(61_000);
    // by the time the timer fires the referral is stale
    expect(referralIsFresh(asOf, 14, new Date(expiry - 60_000 + delay))).toBe(false);
  });

  it('never exceeds what a browser timer can hold', () => {
    const delay = referralRecheckDelay(asOf, 365, new Date('2026-11-01T00:00:00Z'))!;
    expect(delay).toBe(2_147_483_647);
  });

  it('needs no timer when the referral never expires, is already stale or has no readable date', () => {
    expect(referralRecheckDelay(asOf, null, new Date('2026-11-02T00:00:00Z'))).toBeNull();
    expect(referralRecheckDelay(asOf, undefined, new Date('2026-11-02T00:00:00Z'))).toBeNull();
    expect(referralRecheckDelay(asOf, 14, new Date('2026-12-01T00:00:00Z'))).toBeNull();
    for (const bad of [null, undefined, '', 'yesterday']) {
      expect(referralRecheckDelay(bad, 14, new Date('2026-11-02T00:00:00Z'))).toBeNull();
    }
  });
});
