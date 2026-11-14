// Keep the original URL only when it is safe to use as an outbound HTTPS link.
export function safeUrl(value: unknown): string | undefined {
  if (typeof value !== 'string' || /[\s"'<>\\]/u.test(value)) return undefined;
  if (Array.from(value).some((char) => char.charCodeAt(0) <= 31 || (char.charCodeAt(0) >= 127 && char.charCodeAt(0) <= 159))) {
    return undefined;
  }
  const authority = /^https:\/\/([^/?#]+)/u.exec(value)?.[1];
  if (!authority || authority.includes('@')) return undefined;
  try {
    const url = new URL(value);
    return url.protocol === 'https:' && url.hostname && !url.username && !url.password ? value : undefined;
  } catch {
    return undefined;
  }
}

export function referralIsFresh(asOf: string | null | undefined, maxAgeDays: number | null | undefined, now: Date): boolean {
  const timestamp = asOf ? Date.parse(asOf) : NaN;
  if (!Number.isFinite(timestamp)) return false;
  return maxAgeDays == null || now.getTime() - timestamp <= maxAgeDays * 86400000;
}

// A details link must identify one deal on the exact cp0x host.
export function dealPageUrl(value: unknown): string | undefined {
  if (!safeUrl(value) || typeof value !== 'string' || !/^https:\/\/cp0x\.com\/deals\/[^/?#]+$/.test(value)) return undefined;
  const url = new URL(value);
  return /^\/deals\/[^/]+$/.test(url.pathname) ? value : undefined;
}

export function referralRecheckDelay(asOf: string | null | undefined, maxAgeDays: number | null | undefined, now: Date): number | null {
  if (maxAgeDays == null || !referralIsFresh(asOf, maxAgeDays, now)) return null;
  const firstExpired = Math.floor(Date.parse(asOf!) + maxAgeDays * 86400000) + 1;
  return Math.min(2_147_483_647, Math.max(1, Math.ceil(firstExpired - now.getTime())));
}
