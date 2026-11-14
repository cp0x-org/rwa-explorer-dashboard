# Tokenized Stocks in DeFi by cp0x

Research site: tokenized assets (on-chain tokens of stocks/ETFs, CoinMarketCap) and their usage in DeFi (Portals), with
ratings of tokenized assets and issuers. Based on the Berry MUI template (React 19 + Vite + MUI 7). Read-only: no wallet, no transactions.

## Data

The site reads one static file, `public/data/rwa.json`, produced by `../rwa-data-pipeline`
(`uv run rwa-pipeline sync` / `uv run rwa-pipeline build`). The browser never calls CMC or Portals.
Types of the file: `src/types/rwa.ts`; loading: `src/api/rwa.ts`.
Token and issuer logos are published by the pipeline to `public/logos/`.
The pipeline writes both files into this folder itself; the full flow is in [`docs/workflow.md`](../docs/workflow.md).

## Outbound links

The pipeline supplies direct position/protocol links, issuer apps, token swap links and cp0x referrals in
`public/data/rwa.json`; older exports without these optional fields still work. Direct links are the primary action.
A referral is a separate, labeled action with a commission disclosure, deal details and the date of its terms.
Referral offers are absent from the static page and appear only in the browser after checking their `as_of` timestamp
against `summary.links_deals_max_age_days`;
direct links remain available. Prerendering and the first browser render show the same markup. No browser requests
are made to resolve links or fetch deal terms.

## Pages

- `/assets` — underlying assets of the basket; `/assets/:ticker` — comparison of its tokenized assets
- `/tokenized` — all tokenized assets in one sortable, filterable table
- `/tokens/:cmcId` — one tokenized asset: deployments and their Portals status, positions (direct and via LP → vault), breakdowns
- `/issuers` — issuer rating; `/issuers/:issuerId` — issuer's tokenized assets in the basket
- `/opportunities` — all token–position links with filters (asset, issuer, network, category, protocol, status, link)
- `/summary` — charts: issuers' share of eligible positions, yield (APY > 0 only), breakdown per underlying

Every page exists in English (the paths above) and Simplified Chinese under `/zh/...` (`/zh/assets/NVDA`). The language
comes from the URL only; the switcher in the header (`EN | 中文`) links to the same page in the other language.

## Languages

`src/i18n/` holds the whole system: `messages/en.ts` is the source of truth (its keys are the `MessageId` type),
`messages/zh.ts` must define exactly the same keys (a missing or extra key fails `tsc`). Components use `useT()`
(typed `t(id, values)`, ICU plurals and interpolation through react-intl) and `useFormat()` (numbers, percentages,
money, dates and ages through `Intl` in the page's locale). Internal links use `LocaleLink` / `useLocaleNavigate`
with locale-free paths (`/tokens/42461`), which add the `/zh` prefix on Chinese pages. Tickers, symbols, addresses,
protocol, network and issuer names are data and never go through the dictionaries. Chinese text uses the system
CJK fonts (`themes/typography.tsx`), no extra font download.

## Getting Started

```bash
pnpm install
pnpm start        # dev server
pnpm build        # tsc + vite build + prerender of every page in both languages into dist/
pnpm lint         # eslint
pnpm test         # vitest: dictionaries (key and ICU-argument parity), locale paths, page metadata, JSON-LD
pnpm test:e2e     # playwright against dist/ (run `pnpm build` first): rendering, SEO tags, i18n, interactions, axe
pnpm serve        # serves dist/ like the Dockerfile does (clean URLs, real 404s), http://localhost:4173
```

`pnpm test:e2e` needs a Chromium for Playwright once: `npx playwright install chromium`.

## Prerender

`pnpm build` also writes static HTML for every page in both languages (`dist/<path>/index.html`, `dist/zh/<path>/index.html`;
`dist/index.html` and `dist/zh/index.html` are the assets pages), so pages are readable without JavaScript — crawlers,
AI agents, link previews — and show the data before the script loads. `vite build --ssr src/entry-server.tsx` bundles the
app for Node into `dist-ssr/`, `scripts/prerender.mjs` renders each route with the data from `dist/data/rwa.json`.
The `<head>` of every page comes from `src/utils/pageMeta.ts` (title, description, breadcrumbs) and `src/utils/seo.ts`
(canonical, `hreflang` en / zh-CN / x-default, Open Graph, JSON-LD: `WebSite`, `Organization`, `WebPage`, `Dataset` +
`ItemList` on listing pages, `BreadcrumbList` on detail pages). The browser then hydrates the page (`src/index.tsx`)
once the data and the dictionary of the URL's language are loaded. With `VITE_SITE_URL` (`.env.production`) the
prerender also writes `dist/sitemap.xml` (every page in both languages with `xhtml:link` alternates, `lastmod` = newest
data collection) and `dist/robots.txt` pointing to it.
During in-app navigation, `documentHead` and `useDocumentHead` update the live `<head>` (canonical, hreflang, og:url,
descriptions and JSON-LD) to match the destination page's prerendered metadata.

- The data is baked in at build time: after `rwa-pipeline sync` rebuild, as before.
- Serve `dist/` as plain static files, not as a single-page app: `serve dist -c ../serve.json` (Dockerfile), not
  `serve -s`, which would answer every path with `dist/index.html`. Unknown paths and unknown ids get `dist/404.html`
  with status 404: a prerendered "Page not found" page (`noindex`); the client router then shows "not found" in the
  language of the URL.
- Output that depends on the viewer (relative "2d ago" times, media queries) must not differ between the prerender and
  the first browser render: use `hooks/useHydrated` and responsive `sx` values instead of `useMediaQuery` for layout.
- A row that opens a page gets a real link in its name cell (`RowLink`, chart rows via `to`): crawlers and agents
  follow links, not `onClick`.
- Accessibility rules kept by the e2e tests (axe, WCAG 2.2 AA): one `h1` per page, `header` / `nav` / `main` / `footer`
  landmarks, tables with a visually hidden `<caption>` and `aria-sort` on sortable headers, hints as `describeChild`
  tooltips plus visually hidden text (`ui-component/VisuallyHidden`), charts with an accessible legend and text summary,
  every control with an accessible name. Details and the audit: [`docs/i18n-ssr-seo-plan.md`](../docs/i18n-ssr-seo-plan.md).

## Application Links
- Website: [rwa.cp0x.com](https://rwa.cp0x.com/)
- Twitter: [@cp0xdotcom](https://x.com/cp0xdotcom)
- Telegram: [@cp0xdotcom](https://t.me/cp0xdotcom)

## Contributions

For steps on local deployment, development, and code contribution, please see [CONTRIBUTING](../CONTRIBUTING.md) in the repository root.

## Holder rights

Holder rights come from the pipeline's manually reviewed legal profiles in `public/data/rwa.json`, including the
ordered fact schema and source documents. Each token's `rights_profile_id` selects its own profile; profiles of
other tokens from the same issuer never apply automatically. Older exports without rights fields remain supported.

A sourced value, a fact not found in the reviewed documents (`unknown`, with the documents checked), and an
unreviewed token are separate states. Issuer pages show compact key facts and a closed full profile with all facts,
notes, quotes and document links. Token summaries and the asset comparison use compact values without notes or
expandable lists; only reviewed tokens appear in the comparison table, with unreviewed tokens linked below it.
Short views mark facts with conditions and direct readers to the full profile for details.
An `unknown` fact with a note is unresolved because the documents disagree or are incomplete, with the explanation shown in full views and a conditions marker in short views; without a note, it means the fact was not found in the documents.
Links to `/issuers/<issuer-id>#rights-<profile-id>` open the full profile after hydration and scroll to its heading,
including direct visits and fragment-only navigation. Old reviews are marked using the build's stale flag and,
after hydration, the viewer's current date; timers and tab wake events recheck expiry while a page remains open.

Country names come from the committed `src/i18n/regions.ts` table, generated by `node scripts/gen-regions.mjs`
using Node's `Intl.DisplayNames` for English and Simplified Chinese (short names for HK and MO). Rendering uses
this table rather than the browser's ICU data, keeping the prerender and hydration consistent.
