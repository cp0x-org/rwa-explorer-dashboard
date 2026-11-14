import { readFileSync } from 'node:fs';
import path from 'node:path';
import AxeBuilder from '@axe-core/playwright';
import { expect, Locator, Page, test } from '@playwright/test';

import en from '../src/i18n/messages/en';
import { CJK, hydrated, watchErrors } from './helpers';

interface Evidence {
  quote: string;
  url: string;
  title: string;
}
interface Fact {
  value: string;
  who?: string;
  countries?: string[];
  zones?: string[];
  us_persons?: boolean;
  note?: { en?: string; zh?: string };
  evidence?: Evidence[];
}
interface Profile {
  id: string;
  issuer_id: string;
  product: string;
  token_ids: number[];
  reviewed_at: string;
  legal_issuer: { name: string; evidence: Evidence[] };
  facts: Record<string, Fact>;
  documents: { url: string; title: string }[];
}
interface Export {
  rights_profiles?: Profile[];
  rights_schema?: { facts: { key: string; group: string }[] };
  tokens: {
    id: number;
    ticker: string;
    symbol: string | null;
    issuer_id: string | null;
    excluded_reason: string | null;
    rights_profile_id?: string | null;
  }[];
  issuers: { id: string; token_ids: number[]; rights_profile_ids?: string[] }[];
  summary: { rights_review_valid_days?: number | null };
}

// The same export the build baked into dist/.
const DATA_PATH = path.resolve(__dirname, '..', 'public', 'data', 'rwa.json');
const data: Export = JSON.parse(readFileSync(DATA_PATH, 'utf8'));
const messages = en as Record<string, string>;
const COMPARE = ['instrument', 'backing', 'asset_protection', 'primary_access', 'redemption', 'claim_scope', 'loss_issuer_insolvency'];
const SUMMARY = ['instrument', 'claim_scope', 'redemption', 'restricted'];
// The profile the tests read: its short facts and the hack fact are established, the legal form has a quote.
// A profile where the documents are silent on those is valid too, it just cannot carry these checks.
const profile = data.rights_profiles?.find(
  (p) =>
    [...SUMMARY, 'loss_exploit'].every((key) => p.facts[key] && p.facts[key].value !== 'unknown') && p.facts.instrument.evidence?.length
);
const reviewed = profile && data.tokens.find((t) => t.id === profile.token_ids[0]);
// A token of the same underlying that nobody reviewed.
const unreviewed = reviewed && data.tokens.find((t) => t.ticker === reviewed.ticker && t.excluded_reason === null && !t.rights_profile_id);
// The widest profile: one set of documents for a whole product line.
const widest = [...(data.rights_profiles ?? [])].sort((a, b) => b.token_ids.length - a.token_ids.length)[0];
const factCount = data.rights_schema?.facts.length ?? 0;

const valueText = (key: string, fact: Fact) => {
  if (fact.value === 'unknown') return messages['rights.value.unknown'];
  const template = messages[`rights.value.${key}.${fact.value}`];
  return fact.who ? template.replace('{who}', messages[`rights.who.${fact.who}`]) : template;
};
const showAll = (messages['rights.full.show'] ?? '').replace('{n}', String(factCount));
// The value of a fact in a definition list: the <dd> that follows the <dt> with the name of the fact.
const valueOf = (scope: Locator, key: string) =>
  scope.locator('dt', { hasText: new RegExp(`^${messages[`rights.fact.${key}`]}$`) }).locator('xpath=following-sibling::dd[1]');
const openEverything = (section: Locator) =>
  section.evaluate((node) => node.querySelectorAll('details').forEach((details) => (details.open = true)));
const seriousViolations = async (page: Page, selector: string) => {
  const results = await new AxeBuilder({ page })
    .include(selector)
    .withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa', 'best-practice'])
    .analyze();
  return results.violations
    .filter((v) => v.impact === 'critical' || v.impact === 'serious')
    .map((v) => `${v.id}: ${v.nodes[0].html.slice(0, 120)}`);
};
// Serves the page a changed export: the pages reached by in-site navigation are rendered from it.
const serveChanged = (page: Page, change: (body: Export) => void) =>
  page.route('**/data/rwa.json', async (route) => {
    const body: Export = JSON.parse(readFileSync(DATA_PATH, 'utf8'));
    change(body);
    await route.fulfill({ json: body });
  });

test.describe('holder rights', () => {
  test.skip(!profile || !reviewed, 'the export has no reviewed rights profile');

  test('the issuer page opens with the key facts and keeps the full profile one click away', async ({ page }) => {
    const errors = watchErrors(page);
    await page.goto(`/issuers/${profile!.issuer_id}`);
    await hydrated(page);

    const section = page.locator(`#rights-${profile!.id}`);
    await expect(section).toBeVisible();
    await expect(section.getByRole('heading', { name: /Holder rights/ })).toBeVisible();
    await expect(section.getByText(profile!.product, { exact: false }).first()).toBeVisible();

    // who stands behind the token, with the words of the document that says so
    const legal = page.locator(`#rights-issuer-${profile!.id}`);
    await expect(legal.getByText(profile!.legal_issuer.name, { exact: false }).first()).toBeVisible();
    const legalSources = legal.locator('details');
    await expect(legalSources.locator('summary')).toHaveText(`Sources (${profile!.legal_issuer.evidence.length})`);
    await legalSources.locator('summary').click();
    await expect(legal.locator('blockquote').first()).toHaveText(profile!.legal_issuer.evidence[0].quote);

    // the key facts read without a click: label + value, the value says to whom the right belongs
    const full = page.locator(`#rights-full-${profile!.id}`);
    await expect(full).not.toHaveAttribute('open');
    for (const key of SUMMARY) {
      const value = valueOf(section, key).first();
      await expect(value).toBeVisible();
      await expect(value).toContainText(valueText(key, profile!.facts[key]));
    }
    await expect(section.getByText('Hack or exploit', { exact: true })).toBeHidden();

    // every fact, the quotes and the documents are behind one named control
    await expect(full.locator('> summary')).toHaveText(showAll);
    await full.locator('> summary').click();
    await expect(full.getByRole('heading', { name: 'If something breaks' })).toBeVisible();
    await expect(valueOf(full, 'loss_exploit')).toContainText(valueText('loss_exploit', profile!.facts.loss_exploit));
    expect(await full.locator('dt').count()).toBe(factCount);

    const instrument = profile!.facts.instrument.evidence![0];
    const sources = valueOf(full, 'instrument').locator('details');
    await expect(sources.locator('summary')).toHaveText(`Sources (${profile!.facts.instrument.evidence!.length})`);
    await sources.locator('summary').click();
    await expect(sources.locator('blockquote').first()).toHaveText(instrument.quote);
    const source = sources.locator(`a[href="${instrument.url}"]`).first();
    await expect(source).toHaveAttribute('target', '_blank');
    await expect(source).toHaveAttribute('rel', /noopener/);
    await expect(full.getByRole('heading', { name: 'Documents' })).toBeVisible();
    await expect(full.locator(`a[href="${profile!.documents[0].url}"]`).last()).toBeVisible();

    await expect(section.getByText(en['rights.disclaimer'], { exact: false })).toBeVisible();
    await expect(section.getByRole('link', { name: 'Suggest a correction' })).toHaveAttribute(
      'href',
      'https://github.com/cp0x-org/rwa-explorer-dashboard/issues/new'
    );
    expect(errors).toEqual([]);
  });

  test('a note and a long list of places belong to the full profile, the short views stay short', async ({ page }) => {
    const noted = COMPARE.find((key) => profile!.facts[key].note?.en) ?? SUMMARY.find((key) => profile!.facts[key].note?.en);
    test.skip(!noted, 'the first profile has no note on a compared fact');
    const note = profile!.facts[noted!].note!.en!;

    await page.goto(`/issuers/${profile!.issuer_id}`);
    await hydrated(page);
    const section = page.locator(`#rights-${profile!.id}`);
    const full = page.locator(`#rights-full-${profile!.id}`);
    await full.locator('> summary').click();
    await expect(valueOf(full, noted!)).toContainText(note);
    // outside the full profile the same fact is one or two lines
    for (const text of await section.locator(`:scope > *:not(#rights-full-${profile!.id})`).allInnerTexts())
      expect(text).not.toContain(note);

    // the list of a restricted offer is the list itself, not a list of where the right exists
    const restricted = profile!.facts.restricted;
    if (restricted.value === 'listed') {
      const value = valueOf(full, 'restricted');
      await expect(value).not.toContainText('only in:');
      if (restricted.us_persons) await expect(value).toContainText('US persons');
      const entries = (restricted.us_persons ? 1 : 0) + (restricted.countries?.length ?? 0) + (restricted.zones?.length ?? 0);
      if (entries > 3) await expect(valueOf(section, 'restricted').first()).toContainText(`${entries} entries`);
    }
    // a right that exists in some places says "only in"; a zone inside a country narrows it, it is not a second place
    const located = Object.keys(profile!.facts).find(
      (key) =>
        !['restricted', 'governing_law'].includes(key) && (profile!.facts[key].countries?.length || profile!.facts[key].zones?.length)
    );
    if (located) {
      await expect(valueOf(full, located)).toContainText('only in:');
      const { countries, zones } = profile!.facts[located];
      if (countries?.length && zones?.length) await expect(valueOf(full, located)).toContainText(`only in: ${zones[0]} (`);
    }
    // the governing law names its jurisdiction without "only in"
    const law = profile!.facts.governing_law;
    if (law.value !== 'unknown') {
      await expect(valueOf(full, 'governing_law')).not.toContainText('only in:');
      if (law.zones?.length)
        await expect(valueOf(full, 'governing_law')).toContainText(law.countries?.length ? `${law.zones[0]} (` : law.zones[0]);
    }

    await page.goto(`/assets/${reviewed!.ticker}`);
    await hydrated(page);
    const table = page.getByRole('table', { name: en['rights.compare.caption'] });
    await expect(table).toBeVisible();
    await expect(table.getByText(note, { exact: false })).toHaveCount(0);
    await expect(table.locator('details')).toHaveCount(0);
  });

  test('a fact with conditions says so in the short views instead of dropping them silently', async ({ page }) => {
    const established = (p: Profile, key: string) => p.facts[key] && p.facts[key].value !== 'unknown' && p.facts[key].note?.en;
    const compared = (data.rights_profiles ?? []).flatMap((p) =>
      COMPARE.filter((key) => established(p, key)).map((key) => ({ p, key }))
    )[0];
    test.skip(!compared, 'no compared fact has a note');
    const token = data.tokens.find((t) => t.id === compared.p.token_ids[0])!;
    await page.goto(`/assets/${token.ticker}`);
    await hydrated(page);
    const table = page.getByRole('table', { name: en['rights.compare.caption'] });
    const row = table.locator('tbody tr', { has: page.locator(`a[href$="/issuers/${compared.p.issuer_id}#rights-${compared.p.id}"]`) });
    await expect(row.getByText('Conditions apply').first()).toBeVisible();
    await expect(row.getByText(compared.p.facts[compared.key].note!.en!, { exact: false })).toHaveCount(0);

    const summarised = (data.rights_profiles ?? []).flatMap((p) =>
      SUMMARY.filter((key) => established(p, key)).map((key) => ({ p, key }))
    )[0];
    if (summarised) {
      await page.goto(`/tokens/${summarised.p.token_ids[0]}`);
      await hydrated(page);
      await expect(valueOf(page.locator('main'), summarised.key).first()).toContainText('Conditions apply');
    }
    // a fact without a note carries no such mark
    const plain = (data.rights_profiles ?? []).flatMap((p) =>
      SUMMARY.filter((key) => p.facts[key] && !p.facts[key].note).map((key) => ({ p, key }))
    )[0];
    if (plain) {
      await page.goto(`/tokens/${plain.p.token_ids[0]}`);
      await hydrated(page);
      await expect(valueOf(page.locator('main'), plain.key).first()).not.toContainText('Conditions apply');
    }
  });

  test('an unresolved fact with an explanation is not shown as plain silence of the documents', async ({ page }) => {
    const unresolved = (data.rights_profiles ?? []).flatMap((p) =>
      COMPARE.filter((key) => p.facts[key]?.value === 'unknown' && p.facts[key].note?.en).map((key) => ({ p, key }))
    )[0];
    test.skip(!unresolved, 'no compared fact is unresolved with a note');
    const token = data.tokens.find((t) => t.id === unresolved.p.token_ids[0])!;
    await page.goto(`/assets/${token.ticker}`);
    await hydrated(page);
    const table = page.getByRole('table', { name: en['rights.compare.caption'] });
    const row = table.locator('tbody tr', { has: page.locator(`a[href$="/issuers/${unresolved.p.issuer_id}#rights-${unresolved.p.id}"]`) });
    // the row is: token (header cell), then one cell per compared fact
    const cell = row.locator('td').nth(COMPARE.indexOf(unresolved.key));
    await expect(cell).toContainText(messages['rights.value.unresolved']);
    await expect(cell).toContainText('Conditions apply');
    await expect(cell).not.toContainText(en['rights.value.unknown']);

    await page.goto(`/issuers/${unresolved.p.issuer_id}#rights-${unresolved.p.id}`);
    await hydrated(page);
    const full = page.locator(`#rights-full-${unresolved.p.id}`);
    await expect(valueOf(full, unresolved.key)).toContainText(messages['rights.value.unresolved']);
    await expect(valueOf(full, unresolved.key)).toContainText(unresolved.p.facts[unresolved.key].note!.en!);
    // a fact the documents are simply silent on keeps the plain wording
    const silent = Object.keys(unresolved.p.facts).find(
      (key) => unresolved.p.facts[key].value === 'unknown' && !unresolved.p.facts[key].note
    );
    if (silent) {
      await expect(valueOf(full, silent)).toContainText(en['rights.value.unknown']);
      await expect(valueOf(full, silent)).not.toContainText(messages['rights.value.unresolved']);
    }
  });

  test('a link to a profile opens the full profile and brings it into view', async ({ page }) => {
    // straight from the address bar
    await page.goto(`/issuers/${profile!.issuer_id}#rights-${profile!.id}`);
    await hydrated(page);
    const full = page.locator(`#rights-full-${profile!.id}`);
    await expect(full).toHaveAttribute('open', '');
    await expect(page.locator(`#rights-${profile!.id}`).getByRole('heading', { name: /Holder rights/ })).toBeInViewport();

    // and from another page of the site
    await page.goto(`/assets/${reviewed!.ticker}`);
    await hydrated(page);
    await page
      .getByRole('table', { name: en['rights.compare.caption'] })
      .locator(`a[href$="/issuers/${profile!.issuer_id}#rights-${profile!.id}"]`)
      .click();
    await page.waitForURL(`**/issuers/${profile!.issuer_id}#rights-${profile!.id}`);
    await expect(page.locator(`#rights-full-${profile!.id}`)).toHaveAttribute('open', '');
    await expect(page.locator(`#rights-${profile!.id}`).getByRole('heading', { name: /Holder rights/ })).toBeInViewport();
  });

  test('the link from an unreviewed token lands on the rights of its issuer, below the fixed header', async ({ page }) => {
    const sibling = data.tokens.find(
      (t) =>
        !t.rights_profile_id && t.excluded_reason === null && data.issuers.some((i) => i.id === t.issuer_id && i.rights_profile_ids?.length)
    );
    test.skip(!sibling, 'every token of the reviewed issuers is reviewed');
    await page.goto(`/tokens/${sibling!.id}`);
    await hydrated(page);
    await page.getByRole('link', { name: /^Reviewed for other tokens of / }).click();
    await page.waitForURL(`**/issuers/${sibling!.issuer_id}#rights`);
    const heading = page
      .locator('#rights')
      .getByRole('heading', { name: /Holder rights/ })
      .first();
    await expect(heading).toBeInViewport();
    const top = await heading.evaluate((node) => node.getBoundingClientRect().top);
    const header = await page.getByRole('banner').evaluate((node) => node.getBoundingClientRect().bottom);
    expect(top).toBeGreaterThanOrEqual(header);
  });

  test('the asset page compares the reviewed tokens and never shows one token under the rights of another', async ({ page }) => {
    const errors = watchErrors(page);
    await page.goto(`/assets/${reviewed!.ticker}`);
    await hydrated(page);

    const table = page.getByRole('table', { name: en['rights.compare.caption'] });
    await expect(table).toBeVisible();
    for (const key of COMPARE) {
      await expect(table.getByRole('columnheader', { name: messages[`rights.fact.${key}`], exact: true })).toBeVisible();
    }
    const sameAsset = data.tokens.filter((t) => t.ticker === reviewed!.ticker && t.excluded_reason === null);
    await expect(table.locator('tbody tr')).toHaveCount(sameAsset.filter((t) => t.rights_profile_id).length);

    const row = table.locator('tbody tr', { has: page.locator(`a[href$="/issuers/${profile!.issuer_id}#rights-${profile!.id}"]`) });
    await expect(row).toHaveCount(1);
    await expect(row.getByText(valueText('instrument', profile!.facts.instrument), { exact: false }).first()).toBeVisible();
    await expect(row.getByText(valueText('claim_scope', profile!.facts.claim_scope), { exact: false }).first()).toBeVisible();

    if (unreviewed) {
      // an unreviewed token is named below the table and has no row of somebody else's rights
      await expect(table.locator(`a[href$="/tokens/${unreviewed.id}"]`)).toHaveCount(0);
      const rest = page.locator('#rights-unreviewed');
      await expect(rest).toContainText('Not reviewed yet');
      await expect(rest.locator(`a[href$="/tokens/${unreviewed.id}"]`)).toBeVisible();
    }
    expect(errors).toEqual([]);
  });

  test('the token page summarises its own rights and links to the full profile', async ({ page }) => {
    await page.goto(`/tokens/${reviewed!.id}`);
    await hydrated(page);
    await expect(page.getByRole('heading', { name: 'Holder rights' })).toBeVisible();
    await expect(page.getByText(valueText('claim_scope', profile!.facts.claim_scope), { exact: false }).first()).toBeVisible();
    await expect(page.getByRole('link', { name: 'Full profile and sources' })).toHaveAttribute(
      'href',
      new RegExp(`/issuers/${profile!.issuer_id}#rights-${profile!.id}$`)
    );
  });

  test('an unreviewed token says so instead of borrowing a profile', async ({ page }) => {
    test.skip(!unreviewed, 'every token of the underlying is reviewed');
    await page.goto(`/tokens/${unreviewed!.id}`);
    await hydrated(page);
    await expect(page.getByText(en['rights.notReviewed.token'], { exact: false })).toBeVisible();
    await expect(page.getByRole('link', { name: 'Full profile and sources' })).toHaveCount(0);
  });

  test('a review older than the valid period is marked as old once the page is live', async ({ page }) => {
    await page.clock.install({ time: new Date('2031-01-01T00:00:00Z') });
    await page.goto(`/tokens/${reviewed!.id}`);
    await hydrated(page);
    await expect(page.getByText(/Reviewed long ago/).first()).toBeVisible();
  });

  test('a review turns old on a page left open across the end of its valid period', async ({ page }) => {
    const validDays = data.summary.rights_review_valid_days;
    test.skip(!validDays, 'the export has no valid period for reviews');
    const firstStaleMoment = Date.parse(`${profile!.reviewed_at}T00:00:00Z`) + (validDays! + 1) * 86_400_000;
    await page.clock.install({ time: new Date(firstStaleMoment - 60_000) });
    await page.goto(`/tokens/${reviewed!.id}`);
    await hydrated(page);
    await expect(page.getByText(/^Reviewed /).first()).toBeVisible();
    await expect(page.getByText(/Reviewed long ago/)).toHaveCount(0);
    await page.clock.fastForward(120_000);
    await expect(page.getByText(/Reviewed long ago/).first()).toBeVisible();
  });

  test('a reviewed token outside the roster of its issuer is still listed under its profile', async ({ page }) => {
    await serveChanged(page, (body) => {
      const issuer = body.issuers.find((i) => i.id === profile!.issuer_id)!;
      issuer.token_ids = issuer.token_ids.filter((id) => id !== reviewed!.id);
    });
    await page.goto('/issuers');
    await hydrated(page);
    await page.locator(`a[href="/issuers/${profile!.issuer_id}"]`).first().click();
    await page.waitForURL(`**/issuers/${profile!.issuer_id}`);
    const section = page.locator(`#rights-${profile!.id}`);
    await expect(section).toBeVisible();
    await openEverything(section);
    await expect(section.locator(`a[href$="/tokens/${reviewed!.id}"]`).first()).toBeVisible();
    const sameIssuer = data.tokens.filter((t) => t.issuer_id === profile!.issuer_id).length;
    await expect(section.getByText(`Reviewed for ${profile!.token_ids.length} of ${sameIssuer} tokens of this issuer`)).toBeVisible();
  });

  test('on a phone the token stays in sight while the comparison scrolls sideways', async ({ page, isMobile }) => {
    test.skip(!isMobile, 'phone layout only');
    await page.goto(`/assets/${reviewed!.ticker}`);
    await hydrated(page);
    const table = page.getByRole('table', { name: en['rights.compare.caption'] });
    const name = table.locator(`a[href$="/issuers/${profile!.issuer_id}#rights-${profile!.id}"]`);
    await name.scrollIntoViewIfNeeded();
    const scrolled = await table.evaluate((node) => {
      let box: HTMLElement | null = node.parentElement;
      while (box && box.scrollWidth <= box.clientWidth) box = box.parentElement;
      if (!box) return 0;
      box.scrollLeft = box.scrollWidth;
      return box.scrollLeft;
    });
    expect(scrolled).toBeGreaterThan(100); // the table is wider than the phone
    await expect(name).toBeInViewport();
    // the control: an ordinary cell of the same row did scroll out of sight
    const row = table.locator('tbody tr', { has: name });
    await expect(row.locator('td').first()).not.toBeInViewport();
  });

  test('the opened profiles keep every link and control usable', async ({ page }) => {
    for (const target of [profile!, widest]) {
      await page.goto(`/issuers/${target.issuer_id}`);
      await hydrated(page);
      const section = page.locator(`#rights-${target.id}`);
      await openEverything(section);
      await expect(section.getByRole('heading', { name: 'Documents' })).toBeVisible();
      expect(await seriousViolations(page, `#rights-${target.id}`), target.id).toEqual([]);
    }
  });

  test('an export from before the rights layer shows nothing borrowed and breaks nothing', async ({ page }) => {
    const failures: string[] = [];
    page.on('pageerror', (error) => failures.push(error.message));
    await serveChanged(page, (body) => {
      delete body.rights_profiles;
      delete body.rights_schema;
      for (const token of body.tokens) delete token.rights_profile_id;
      for (const issuer of body.issuers) delete issuer.rights_profile_ids;
    });
    await page.goto('/issuers');
    await hydrated(page);
    await page.locator(`a[href="/issuers/${profile!.issuer_id}"]`).first().click();
    await page.waitForURL(`**/issuers/${profile!.issuer_id}`);
    await expect(page.getByText(en['rights.notReviewed'], { exact: false })).toBeVisible();
    await expect(page.locator(`#rights-${profile!.id}`)).toHaveCount(0);
    expect(failures).toEqual([]);

    // opened directly, the static page still carries the rights of the build: the live page must drop them
    await page.goto(`/tokens/${reviewed!.id}`);
    await hydrated(page);
    await expect(page.getByText(en['rights.notReviewed.token'], { exact: false })).toBeVisible();
    await expect(page.getByRole('link', { name: 'Full profile and sources' })).toHaveCount(0);
  });

  test('the Chinese page carries the same facts and sources', async ({ page }) => {
    await page.goto(`/zh/issuers/${profile!.issuer_id}`);
    await hydrated(page);
    const section = page.locator(`#rights-${profile!.id}`);
    await expect(section).toBeVisible();
    expect(await section.innerText()).toMatch(CJK);
    await expect(section.locator(`a[href="${profile!.facts.instrument.evidence![0].url}"]`).first()).toBeAttached();
  });
});
