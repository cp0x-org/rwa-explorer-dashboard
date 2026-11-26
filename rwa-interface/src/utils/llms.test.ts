import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';

import { RwaExport } from 'types/rwa';
import { llmsText } from './llms';

const data: RwaExport = JSON.parse(readFileSync(new URL('../../public/data/rwa.json', import.meta.url), 'utf8'));
const SITE = 'https://rwa.cp0x.com';

describe('llmsText', () => {
  const text = llmsText(data, `${SITE}/`);

  it('follows the llms.txt layout: a title, a summary quote, then sections of links', () => {
    const lines = text.split('\n');
    expect(lines[0]).toBe('# RWA Explorer');
    expect(lines[2]).toMatch(/^> \S/);
    for (const section of ['## Data for programs', '## Sections', '## Underlying assets', '## Issuers', '## Optional'])
      expect(lines).toContain(section);
  });

  it('links every underlying asset and issuer with an absolute address', () => {
    for (const u of data.underlyings) expect(text).toContain(`](${SITE}/assets/${encodeURIComponent(u.ticker)})`);
    for (const i of data.issuers) expect(text).toContain(`](${SITE}/issuers/${encodeURIComponent(i.id)})`);
    expect(text).toContain(`(${SITE}/api/v1/openapi.yaml)`); // no API address given: the site's own /api/
    const links = Array.from(text.matchAll(/\]\(([^)]*)\)/g), (m) => m[1]);
    for (const href of links) expect(href.startsWith(`${SITE}/`), href).toBe(true);
  });

  it('sends programs to the public address of the API when there is one', () => {
    const withApi = llmsText(data, SITE, 'https://rwa-api.cp0x.com/');
    expect(withApi).toContain('(https://rwa-api.cp0x.com/api/v1/export)');
    expect(withApi).toContain('(https://rwa-api.cp0x.com/api/v1/openapi.yaml)');
    expect(withApi).not.toContain(`${SITE}/api/`);
    expect(withApi).toContain(`(${SITE}/assets)`);
  });

  it('fills every placeholder', () => {
    expect(text).not.toMatch(/undefined|NaN|\{\w+\}(?!\s*\(CoinMarketCap)/);
  });
});
