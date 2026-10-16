import { RwaExport } from 'types/rwa';
import { fmtDate, fmtNum } from 'utils/rwa';

export const SITE_NAME = 'Tokenized Stocks in DeFi by cp0x';

export interface PageMeta {
  title: string;
  description: string;
}

// Title and description of a page: written into the prerendered <head> and set as document.title on navigation.
export function pageMeta(pathname: string, data: RwaExport): PageMeta {
  const s = data.summary;
  const basket =
    `${fmtNum(s.underlyings_count)} stocks and ETFs, ${fmtNum(s.tokens_count)} tokenized assets from ${fmtNum(s.issuers_count)} ` +
    `issuers, ${fmtNum(s.eligible_opportunities_count)} eligible DeFi positions. Data from CoinMarketCap and Portals, collected ` +
    `${fmtDate(s.newest_fetched_at)}.`;
  const page = (title: string, description: string): PageMeta => ({ title: `${title} · ${SITE_NAME}`, description });
  const [section, id] = pathname.split('/').filter(Boolean);

  if (section === 'assets' && id) {
    const u = data.underlyings.find((x) => x.ticker === decodeURIComponent(id));
    if (u) {
      const name = u.cmc_name ?? u.name;
      return page(
        `${u.ticker} (${name}) tokenized`,
        `${fmtNum(u.versions_count)} tokenized assets of ${name} (${u.ticker}) from ${fmtNum(u.issuers_count)} issuers: ` +
          `${fmtNum(u.eligible_opportunities_count)} eligible DeFi positions in ${fmtNum(u.eligible_protocols_count)} protocols, ` +
          `ranked by DeFi usage.`
      );
    }
  }
  if (section === 'tokens' && id) {
    const t = data.tokens.find((x) => String(x.id) === id);
    if (t) {
      const symbol = t.symbol ?? `CMC #${t.id}`;
      const issuer = t.issuer_name ?? 'unknown issuer';
      return page(
        `${symbol} by ${issuer} (${t.ticker})`,
        `${symbol}${t.name ? ` (${t.name})` : ''}: tokenized ${t.ticker} by ${issuer}. ` +
          `${fmtNum(t.counts.eligible_opportunities_count)} eligible DeFi positions in ${fmtNum(t.counts.eligible_protocols_count)} protocols.`
      );
    }
  }
  if (section === 'issuers' && id) {
    const i = data.issuers.find((x) => x.id === id);
    if (i) {
      const name = i.name ?? 'Unknown issuer';
      return page(
        `${name}: tokenized stocks in DeFi`,
        `${name} issues ${fmtNum(i.basket_tokens_count)} tokenized assets of ${fmtNum(i.basket_underlyings_count)} stocks and ETFs ` +
          `in the basket, used in ${fmtNum(i.issuer_usefulness_score)} unique eligible DeFi positions.`
      );
    }
  }

  if (id) return { title: SITE_NAME, description: basket }; // unknown asset, token or issuer: the page says "not found"

  switch (section) {
    case 'tokenized':
      return page('Tokenized assets', `Tokenized stocks and ETFs ranked by eligible DeFi positions. ${basket}`);
    case 'issuers':
      return page('Issuers', `Issuers of tokenized stocks ranked by unique eligible DeFi positions. ${basket}`);
    case 'opportunities':
      return page('DeFi positions', `DEX pools, lending markets, vaults and yield markets that hold tokenized stocks. ${basket}`);
    case 'summary':
      return page(
        'Summary: DeFi readiness and yield',
        `Which issuer is the most DeFi-ready and which tokenized stocks yield the most (APY > 0 only). ${basket}`
      );
    case 'assets':
      return page('Underlying assets', `Stocks and ETFs ranked by DeFi usage of their tokenized assets. ${basket}`);
    default:
      return { title: SITE_NAME, description: basket };
  }
}
