import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';

import { buildIndex } from 'api/rwa';
import en from 'i18n/messages/en';
import zh from 'i18n/messages/zh';
import { REGION_NAMES } from 'i18n/regions';
import { RightsFact, RwaExport, Token } from 'types/rwa';
import {
  COMPARE_FACTS,
  countryName,
  factState,
  placesOf,
  reviewIsStale,
  reviewRecheckDelay,
  rightsMessageIds,
  SUMMARY_FACTS,
  tokenRights
} from './rights';

// The published export. It may carry no rights at all (no reviewed profiles yet, or an export from before the layer):
// the tests of the published data are skipped then, the tests of the functions use their own small export.
const data: RwaExport = JSON.parse(readFileSync(new URL('../../public/data/rwa.json', import.meta.url), 'utf8'));
const schema = data.rights_schema;
const profiles = data.rights_profiles ?? [];

const fixture = {
  underlyings: [],
  opportunities: [],
  issuers: [{ id: 'issuer-1', token_ids: [1, 2], rights_profile_ids: ['line-1'] }],
  tokens: [
    { id: 1, symbol: 'AAAx', issuer_id: 'issuer-1', deployments: [], rights_profile_id: 'line-1' },
    { id: 2, symbol: 'BBBx', issuer_id: 'issuer-1', deployments: [], rights_profile_id: null }
  ],
  rights_profiles: [{ id: 'line-1', issuer_id: 'issuer-1', token_ids: [1] }]
} as unknown as RwaExport;

describe('translations cover the whole rights schema', () => {
  it.skipIf(!schema)('every group, fact, value and audience of the schema has an English and a Chinese message', () => {
    const ids = rightsMessageIds(schema!);
    expect(ids.length).toBeGreaterThan(80);
    expect(new Set(ids).size).toBe(ids.length);
    for (const id of [
      'rights.group.loss',
      'rights.fact.loss_exploit',
      'rights.value.claim_scope.has_claim',
      'rights.who.platform_clients'
    ]) {
      expect(ids).toContain(id);
    }
    for (const id of [...ids, 'rights.value.unknown']) {
      expect(en, `en ${id}`).toHaveProperty([id]);
      expect(zh, `zh ${id}`).toHaveProperty([id]);
    }
  });

  it.skipIf(!schema)('a value that applies to somebody says to whom', () => {
    for (const fact of schema!.facts) {
      for (const value of fact.values) {
        const id = `rights.value.${fact.key}.${value}` as keyof typeof en;
        expect(en[id].includes('{who}'), `en ${id}`).toBe(fact.needs_who.includes(value));
        expect(zh[id].includes('{who}'), `zh ${id}`).toBe(fact.needs_who.includes(value));
      }
    }
  });

  it('the wording does not say more than the category means', () => {
    // "institutional" covers B2B customers of one issuer and authorised participants of another
    expect(en['rights.who.institutional']).toBe('institutions approved by the issuer');
    // "full" is full backing without a fixed ratio: the documents do not say the ratio differs from 1:1 today
    expect(en['rights.value.backing.full']).toBe('Fully backed; shares per token can change');
    expect(en['rights.value.backing.full_1to1']).toMatch(/1:1/);
  });

  it('conditional outcomes have their own wording in both languages', () => {
    // a payout only at maturity is not "redemption available"; an unsecured creditor has no claim on backing assets;
    // an issuer that may follow instructions does not pass voting through
    const expected: Record<string, string> = {
      'rights.value.redemption.at_events_only': 'Paid out to {who} only at maturity or a set event',
      'rights.value.loss_exploit.unsecured_claim': 'Unsecured claim against the issuer for {who}',
      'rights.value.loss_lost_tokens.unsecured_claim': 'Unsecured claim against the issuer for {who}',
      'rights.value.loss_issuer_insolvency.unsecured_claim': 'Unsecured claim against the issuer for {who}',
      'rights.value.loss_custodian_failure.unsecured_claim': 'Unsecured claim against the issuer for {who}',
      'rights.value.voting.discretionary': 'The issuer may follow instructions of {who}',
      'rights.conditions': 'Conditions apply',
      // documents that were read and contradict each other are not the same as documents that say nothing
      'rights.value.unresolved': 'Not established: the documents disagree or are incomplete'
    };
    for (const [id, text] of Object.entries(expected)) {
      expect((en as Record<string, string>)[id], `en ${id}`).toBe(text);
      const chinese = (zh as Record<string, string>)[id];
      expect(chinese, `zh ${id}`).toMatch(/[一-鿿]/);
      expect(chinese.includes('{who}'), `zh ${id}`).toBe(text.includes('{who}'));
    }
  });

  it.skipIf(!schema)('the short lists of the comparison table and of the token page are facts of the schema', () => {
    const keys = schema!.facts.map((f) => f.key);
    // the columns that tell the issuers apart: every reviewed issuer can freeze and every one has a restricted list
    expect([...COMPARE_FACTS]).toEqual([
      'instrument',
      'backing',
      'asset_protection',
      'primary_access',
      'redemption',
      'claim_scope',
      'loss_issuer_insolvency'
    ]);
    expect([...SUMMARY_FACTS]).toEqual(['instrument', 'claim_scope', 'redemption', 'restricted']);
    for (const key of [...COMPARE_FACTS, ...SUMMARY_FACTS]) expect(keys).toContain(key);
  });
});

describe('tokenRights', () => {
  it('finds the profile of a reviewed token and nothing for any other token', () => {
    const index = buildIndex(fixture);
    const [reviewed, sibling] = fixture.tokens;
    expect(tokenRights(index, reviewed)?.id).toBe('line-1');
    // same issuer, but nobody reviewed this token: the issuer's profile must not leak onto it
    expect(tokenRights(index, sibling)).toBeUndefined();
    expect(tokenRights(index, { ...reviewed, rights_profile_id: 'no-such-profile' } as Token)).toBeUndefined();
    expect(tokenRights(index, { ...reviewed, rights_profile_id: null } as Token)).toBeUndefined();
    expect(tokenRights(index, { ...reviewed, rights_profile_id: undefined } as Token)).toBeUndefined();
  });

  it('an export from before the rights layer gives an empty index, not an error', () => {
    const old = { ...fixture, rights_profiles: undefined, rights_schema: undefined } as RwaExport;
    const oldIndex = buildIndex(old);
    expect(oldIndex.rightsById.size).toBe(0);
    expect(tokenRights(oldIndex, old.tokens[0])).toBeUndefined();
  });

  it.skipIf(!profiles.length)('every published profile is found through each of its tokens', () => {
    const index = buildIndex(data);
    for (const profile of profiles) {
      for (const id of profile.token_ids) expect(tokenRights(index, index.tokenById.get(id)!)?.id, `${profile.id} ${id}`).toBe(profile.id);
    }
  });
});

describe('reviewIsStale', () => {
  const reviewed = '2026-05-17';

  it('a review is stale only after the valid period has passed', () => {
    expect(reviewIsStale(reviewed, 180, new Date('2026-05-17T12:00:00Z'))).toBe(false);
    expect(reviewIsStale(reviewed, 180, new Date('2026-11-13T23:59:59Z'))).toBe(false); // day 180
    expect(reviewIsStale(reviewed, 180, new Date('2026-11-14T00:00:01Z'))).toBe(true);
    expect(reviewIsStale(reviewed, 180, new Date('2030-01-01T00:00:00Z'))).toBe(true);
  });

  it('an unreadable date is treated as stale; an export without a period leaves it to the build', () => {
    for (const bad of ['', 'yesterday', '2026-13-45', undefined, null]) {
      expect(reviewIsStale(bad as unknown as string, 180, new Date('2026-06-01T00:00:00Z'))).toBe(true);
    }
    expect(reviewIsStale(reviewed, null, new Date('2030-01-01T00:00:00Z'))).toBe(false);
    expect(reviewIsStale(reviewed, undefined, new Date('2030-01-01T00:00:00Z'))).toBe(false);
  });
});

describe('reviewRecheckDelay', () => {
  const reviewed = '2026-05-17';
  const lastFreshMinute = Date.parse('2026-11-13T23:59:00Z');

  it('is the time left until the review becomes old', () => {
    const delay = reviewRecheckDelay(reviewed, 180, new Date(lastFreshMinute))!;
    expect(delay).toBeGreaterThanOrEqual(60_000);
    expect(delay).toBeLessThanOrEqual(61_000);
    expect(reviewIsStale(reviewed, 180, new Date(lastFreshMinute + delay))).toBe(true);
    expect(reviewIsStale(reviewed, 180, new Date(lastFreshMinute + delay - 2_000))).toBe(false);
  });

  it('never exceeds what a browser timer can hold', () => {
    expect(reviewRecheckDelay(reviewed, 180, new Date('2026-05-18T00:00:00Z'))).toBe(2_147_483_647);
  });

  it('needs no timer when the review never expires or is already old', () => {
    expect(reviewRecheckDelay(reviewed, null, new Date('2026-06-01T00:00:00Z'))).toBeNull();
    expect(reviewRecheckDelay(reviewed, undefined, new Date('2026-06-01T00:00:00Z'))).toBeNull();
    expect(reviewRecheckDelay(reviewed, 180, new Date('2027-01-01T00:00:00Z'))).toBeNull();
    expect(reviewRecheckDelay('yesterday', 180, new Date('2026-06-01T00:00:00Z'))).toBeNull();
  });
});

describe('factState', () => {
  it('tells a found value, a documented silence and a missing fact apart', () => {
    const found: RightsFact = { value: 'yes', evidence: [] };
    const silent: RightsFact = { value: 'unknown', checked: ['https://example.org/terms'] };
    expect(factState(found)).toBe('value');
    expect(factState(silent)).toBe('unknown');
    expect(factState(undefined)).toBe('missing');
  });
});

describe('placesOf', () => {
  it('a zone narrows the country it lies in: the right exists in the zone, not in the whole country', () => {
    const adgm: RightsFact = { value: 'available', who: 'verified', countries: ['AE'], zones: ['ADGM'] };
    expect(placesOf('primary_access', adgm, 'en')).toEqual({ kind: 'only', entries: ['ADGM (United Arab Emirates)'] });
    const two: RightsFact = { value: 'available', who: 'verified', countries: ['AE'], zones: ['ADGM', 'DIFC'] };
    expect(placesOf('redemption', two, 'en').entries).toEqual(['ADGM (United Arab Emirates)', 'DIFC (United Arab Emirates)']);
    expect(placesOf('redemption', { value: 'available', who: 'anyone', countries: ['CH', 'JE'] }, 'en')).toEqual({
      kind: 'only',
      entries: ['Switzerland', 'Jersey']
    });
    expect(placesOf('redemption', { value: 'available', who: 'anyone', zones: ['ADGM'] }, 'en').entries).toEqual(['ADGM']);
  });

  it('the governing law names a jurisdiction, it is not a place where a right exists', () => {
    expect(placesOf('governing_law', { value: 'stated', countries: ['US'], zones: ['New York'] }, 'en')).toEqual({
      kind: 'jurisdiction',
      entries: ['New York (United States)']
    });
    expect(placesOf('governing_law', { value: 'stated', countries: ['JE', 'CH'] }, 'en').entries).toEqual(['Jersey', 'Switzerland']);
  });

  it('a restricted list keeps countries and zones as separate entries', () => {
    const listed: RightsFact = { value: 'listed', us_persons: true, countries: ['US', 'CU'], zones: ['Crimea'] };
    expect(placesOf('restricted', listed, 'en')).toEqual({ kind: 'listed', entries: ['United States', 'Cuba', 'Crimea'] });
  });

  it('a fact without places has no entries, in either language', () => {
    expect(placesOf('freeze', { value: 'yes' }, 'en').entries).toEqual([]);
    expect(placesOf('freeze', undefined, 'zh-CN').entries).toEqual([]);
    expect(placesOf('redemption', { value: 'available', who: 'anyone', countries: ['US'] }, 'zh-CN').entries).toEqual(['美国']);
  });
});

describe('countryName', () => {
  it('names a country in the language of the page and falls back to the code', () => {
    expect(countryName('AE', 'en')).toBe('United Arab Emirates');
    expect(countryName('ae', 'en')).toBe('United Arab Emirates');
    expect(countryName('US', 'zh-CN')).toBe('美国');
    expect(countryName('AE', 'zh-CN')).toMatch(/[一-鿿]/);
    expect(countryName('ZZZ', 'en')).toBe('ZZZ');
    expect(countryName('ZZ', 'en')).toBe('ZZ');
    expect(countryName('', 'en')).toBe('');
  });

  it('reads the names from the table shipped with the site, not from the ICU data of whoever renders the page', () => {
    // The prerender (Node) and the browsers carry different CLDR versions: a name taken from Intl at render time
    // differs between the static HTML and the hydrated page for some viewers.
    const intl = Intl as unknown as { DisplayNames: unknown };
    const original = intl.DisplayNames;
    intl.DisplayNames = class {
      constructor() {
        throw new Error('no region names on this machine');
      }
    };
    try {
      expect(countryName('AE', 'en')).toBe('United Arab Emirates');
      expect(countryName('HK', 'en')).toBe(REGION_NAMES.en.HK);
      expect(countryName('US', 'zh-CN')).toBe('美国');
    } finally {
      intl.DisplayNames = original;
    }
  });

  it('the table names the whole ISO list in both languages', () => {
    expect(Object.keys(REGION_NAMES.en).length).toBeGreaterThanOrEqual(240);
    expect(Object.keys(REGION_NAMES['zh-CN']).sort()).toEqual(Object.keys(REGION_NAMES.en).sort());
    expect(REGION_NAMES.en).not.toHaveProperty('ZZ');
    for (const code of ['AE', 'US', 'GB', 'CH', 'JE', 'VG', 'KY', 'SV', 'HK', 'AX', 'CD', 'KP', 'MM', 'UM', 'VI']) {
      expect(REGION_NAMES.en[code], `en ${code}`).toBeTruthy();
      expect(REGION_NAMES['zh-CN'][code], `zh-CN ${code}`).toMatch(/[一-鿿]/);
    }
  });

  it.skipIf(!profiles.length)('every country of the published profiles has a name', () => {
    const used = new Set(profiles.flatMap((p) => [p.legal_issuer.country, ...Object.values(p.facts).flatMap((f) => f.countries ?? [])]));
    for (const code of used) {
      if (code) expect(REGION_NAMES.en[code], `en ${code}`).toBeTruthy();
    }
  });
});
