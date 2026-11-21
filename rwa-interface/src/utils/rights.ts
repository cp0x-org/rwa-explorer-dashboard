import { REGION_NAMES } from 'i18n/regions';
import type { RwaIndex } from 'api/rwa';
import type { RightsFact, RightsSchema, Token } from 'types/rwa';

export const COMPARE_FACTS = [
  'instrument',
  'backing',
  'asset_protection',
  'primary_access',
  'redemption',
  'claim_scope',
  'loss_issuer_insolvency'
] as const;
export const SUMMARY_FACTS = ['instrument', 'claim_scope', 'redemption', 'restricted'] as const;

export const tokenRights = (index: RwaIndex, token: Token) =>
  token.rights_profile_id ? index.rightsById.get(token.rights_profile_id) : undefined;

export function reviewIsStale(reviewedAt: string, validDays: number | null | undefined, now: Date): boolean {
  if (validDays == null) return false;
  const timestamp = reviewedAt ? Date.parse(reviewedAt) : NaN;
  if (!Number.isFinite(timestamp)) return true;
  const datePart = /^\d{4}-\d{2}-\d{2}/.exec(reviewedAt)?.[0];
  if (datePart && new Date(timestamp).toISOString().slice(0, 10) !== datePart) return true;
  // Reviews are dated in UTC calendar days; the entire last valid day remains fresh.
  return Math.floor(now.getTime() / 86400000) - Math.floor(timestamp / 86400000) > validDays;
}

export const factState = (fact: RightsFact | undefined): 'value' | 'unknown' | 'missing' =>
  !fact ? 'missing' : fact.value === 'unknown' ? 'unknown' : 'value';

export function rightsMessageIds(schema: RightsSchema): string[] {
  return [
    ...new Set([
      ...schema.facts.flatMap((fact) => [
        `rights.group.${fact.group}`,
        `rights.fact.${fact.key}`,
        ...fact.values.map((value) => `rights.value.${fact.key}.${value}`)
      ]),
      ...schema.who.map((who) => `rights.who.${who}`)
    ])
  ];
}

export function countryName(code: string, locale: string): string {
  return REGION_NAMES[locale === 'zh-CN' ? 'zh-CN' : 'en'][code.toUpperCase()] ?? code;
}

export function placesOf(
  key: string,
  fact: RightsFact | undefined,
  locale: string
): { kind: 'listed' | 'jurisdiction' | 'only'; entries: string[] } {
  const kind = key === 'restricted' ? 'listed' : key === 'governing_law' ? 'jurisdiction' : 'only';
  const countries = (fact?.countries ?? []).map((code) => countryName(code, locale));
  const zones = fact?.zones ?? [];
  const entries =
    kind === 'listed'
      ? [...countries, ...zones]
      : zones.length
        ? zones.map((zone) => (countries.length ? `${zone} (${countries.join(', ')})` : zone))
        : countries;
  return { kind, entries };
}

export function reviewRecheckDelay(reviewedAt: string | null | undefined, validDays: number | null | undefined, now: Date): number | null {
  if (validDays == null || !reviewedAt || reviewIsStale(reviewedAt, validDays, now)) return null;
  const firstStale = (Math.floor(Date.parse(reviewedAt) / 86400000) + Math.floor(validDays) + 1) * 86400000;
  return Math.min(2_147_483_647, Math.max(1, firstStale - now.getTime()));
}
