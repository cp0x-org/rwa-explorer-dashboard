// /llms.txt (https://llmstxt.org): a Markdown map of the site for language models and AI agents. Built from the same export
// and the same English strings as the pages, so it never says more than the site shows. Served by server/index.mjs.
import { RwaExport } from 'types/rwa';
import { ENGLISH, getFormat, MessageId } from 'i18n';
import { intlFor } from './pageMeta';

// a Markdown link text: brackets would end it early
const label = (s: string) => s.replace(/[[\]]/g, '');

// `apiUrl`: public address of the API (https://rwa-api.cp0x.com); the site's own /api/ without one
export function llmsText(data: RwaExport, siteUrl: string, apiUrl?: string): string {
  const site = siteUrl.replace(/\/+$/, '');
  const api = apiUrl?.replace(/\/+$/, '') || site;
  const intl = intlFor('en');
  const t = (id: MessageId, values?: Record<string, string | number>) => intl.formatMessage({ id, defaultMessage: ENGLISH[id] }, values);
  const f = getFormat('en');
  const s = data.summary;
  const n = (v: number) => f.num(v);

  const basket = t('meta.basket', {
    underlyings: n(s.underlyings_count),
    tokens: n(s.tokens_count),
    issuers: n(s.issuers_count),
    positions: n(s.eligible_opportunities_count),
    date: f.day(s.newest_fetched_at)
  });
  const method = t('method.text', {
    depth: data.settings.max_graph_depth,
    liquidity: f.usd(data.settings.min_eligible_liquidity_usd),
    hours: data.settings.max_position_age_hours
  });

  const underlyings = [...data.underlyings]
    .sort((a, b) => b.eligible_opportunities_count - a.eligible_opportunities_count || a.ticker.localeCompare(b.ticker))
    .map(
      (u) =>
        `- [${label(`${u.ticker}: ${u.cmc_name ?? u.name}`)}](${site}/assets/${encodeURIComponent(u.ticker)}): ` +
        `${n(u.versions_count)} tokenized assets from ${n(u.issuers_count)} issuers, ${n(u.eligible_opportunities_count)} eligible DeFi positions`
    );
  const issuers = [...data.issuers]
    .sort((a, b) => b.issuer_usefulness_score - a.issuer_usefulness_score || a.id.localeCompare(b.id))
    .map(
      (i) =>
        `- [${label(i.name ?? i.id)}](${site}/issuers/${encodeURIComponent(i.id)}): ` +
        `${n(i.basket_tokens_count)} tokenized assets of ${n(i.basket_underlyings_count)} stocks and ETFs, ` +
        `${n(i.issuer_usefulness_score)} unique eligible DeFi positions` +
        (i.rights_profile_ids?.length ? '; holder rights reviewed from its documents' : '')
    );

  return [
    `# ${t('site.title')}`,
    '',
    `> ${t('meta.dataset.description')} Published by cp0x (https://cp0x.com).`,
    '',
    basket,
    '',
    `${t('method.how')}: ${method}`,
    '',
    'Every page is server-rendered HTML, in English and in Simplified Chinese (the same path under /zh). ' +
      `Collections run on a schedule (${api}/api/v1/status gives the last and the next one); the pages and the dataset change together after each.`,
    '',
    '## Data for programs',
    '',
    `- [Dataset (JSON)](${api}/api/v1/export): everything behind the pages: underlying assets, tokenized assets, issuers, DeFi positions, holder rights`,
    `- [API description (OpenAPI 3)](${api}/api/v1/openapi.yaml): the dataset, collection status and runs, history of metrics per asset, token and issuer`,
    `- [Collection status](${api}/api/v1/status): the last run and the next scheduled one`,
    '',
    '## Sections',
    '',
    `- [Underlying assets](${site}/assets): stocks and ETFs with tokenized versions, ranked by eligible DeFi positions`,
    `- [Tokenized assets](${site}/tokenized): every tokenized asset with its issuer, networks and DeFi positions; one page per token at /tokens/{id} (CoinMarketCap id)`,
    `- [Issuers](${site}/issuers): issuers ranked by unique eligible DeFi positions, with the holder rights their documents give`,
    `- [Positions](${site}/opportunities): DEX pools, lending markets, vaults and yield markets that hold tokenized assets, with liquidity and reported APY`,
    `- [Summary](${site}/summary): the most DeFi-ready issuer and the highest reported yields`,
    `- [Data updates](${site}/updates): every collection run and what it changed`,
    '',
    '## Underlying assets',
    '',
    ...underlyings,
    '',
    '## Issuers',
    '',
    ...issuers,
    '',
    '## Optional',
    '',
    `- [Sitemap](${site}/sitemap.xml): every page in both languages`,
    `- [Chinese version](${site}/zh/assets): the same pages in Simplified Chinese`,
    ''
  ].join('\n');
}
