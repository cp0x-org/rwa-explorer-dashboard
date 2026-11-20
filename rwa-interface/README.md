# Tokenized Stocks in DeFi by cp0x

Research site: tokenized assets (on-chain tokens of stocks/ETFs, CoinMarketCap) and their usage in DeFi (Portals), with
ratings of tokenized assets and issuers. Based on the Berry MUI template (React 19 + Vite + MUI 7). Read-only: no wallet, no transactions.

## Data

The site reads the API of `rwa-backend` (2.0): `GET /api/v1/export` is the same document as the 1.x `rwa.json` plus
`build_id` and `run_id` (`?build=<id>` returns one build for ever, immutable). The browser calls only our API and our
logos (`/logos/...`), never CMC or Portals. Types: `src/types/rwa.ts` (export), `src/types/api.ts` (status, runs,
history); client: `src/api/rwa.ts` (`API_BASE`, `DATA_URL`, `exportUrl(buildId)`, `logoUrl`, SWR options: refresh every
10 minutes, no revalidation on focus), `src/api/runs.ts` (`useStatus`, `useRuns`, `useRun`), `src/api/history.ts`
(`useHistory`, `useHistoryMetrics`). The last three fetch in the browser only.

`API_BASE` is `VITE_API_URL` (empty = same origin, as in production behind the reverse proxy).
Until the backend runs, `pnpm mock:api` (`mock-api/server.mjs`, no dependencies) serves `public/data/rwa.json` as build 1
plus synthetic status, runs and history: `--port 8080`, `--fail` (every `/api/*` answers 503), `--delay-ms`.

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
pnpm mock:api     # mock API on http://localhost:8080 (or run rwa-backend there)
pnpm start        # dev server, client render only; /api is proxied to http://localhost:8080 (or set VITE_API_URL=http://localhost:8080)
pnpm build        # tsc + vite build (client, dist/) + vite build --ssr (server bundle, dist-ssr/)
pnpm serve        # the SSR server on http://localhost:4173 (API_URL=http://127.0.0.1:8080 by default); pnpm build first
pnpm lint         # eslint
pnpm test         # vitest: dictionaries (key and ICU-argument parity), locale paths, page metadata, JSON-LD, API types and keys
pnpm test:e2e     # playwright: starts the mock API and the server itself (run `pnpm build` first): rendering, SEO tags, i18n, hydration, interactions, axe, API down
```

`pnpm test:e2e` needs a Chromium for Playwright once: `npx playwright install chromium`.
Server environment: `PORT` (4173), `API_URL` (`http://127.0.0.1:8080`), `SITE_URL` (public address for canonical links
and the sitemap; default `VITE_SITE_URL` from `.env.production`). Docker: `docker-compose.yml` passes `API_URL` and `SITE_URL`.

## Server rendering

Data changes with every collection run, so pages are rendered per request, not at build time. `vite build --ssr
src/entry-server.tsx` bundles the app for Node into `dist-ssr/`; `server/index.mjs` (plain Node `http`, no dependencies)
renders each page with the export it keeps in memory and fills the template `dist/index.html`:

- `server/data.mjs` — cache of `/api/v1/export` (`If-None-Match`, refreshed every 60 s, retried every 10 s until the
  first success, the cached export keeps being served when the API fails later) and, lazily with a 60 s TTL, of `/api/v1/status`.
- `server/render.mjs` — `render(path, data)` from the bundle; the `<head>` gets `<meta name="rwa-build" content="N">`
  and `<link rel="preload" as="fetch" href="/api/v1/export?build=N">` (same-origin; with `VITE_API_URL` set the client calls an absolute URL and the preload goes unused, which is fine for the dev server). The page ETag covers the build id, the template (bundle hashes) and `SITE_URL`. A page that is not found (unknown ticker, token,
  issuer or path) is rendered in the language of the URL with HTTP 404 and `noindex`. Before the first export loads,
  every page is a plain "Data unavailable" page with HTTP 503 (`Retry-After: 10`); no restart is needed afterwards.
- `server/static.mjs` — `dist/assets/**` with `Cache-Control: public, max-age=31536000, immutable`, other files of `dist/`
  with a short cache; `dist/index.html` is never served for an unknown path.
- `server/sitemap.mjs` — `/sitemap.xml` (every page in both languages with `xhtml:link` alternates, `lastmod` = newest
  data collection) and `/robots.txt`, regenerated when the build id changes.
- `/healthz` — 200 once an export is cached, 503 before. `/api/*` and `/logos/*` are forwarded to `API_URL` (in
  production a reverse proxy sends them to the API directly; the forwarding makes the server usable on its own).

The browser (`src/index.tsx`) reads `rwa-build`, loads exactly `/api/v1/export?build=N` (immutable, preloaded by the page),
hydrates the page when it was rendered for this location, and does not download the export again: `/api/v1/status` is polled every minute and a newer
build is then fetched once and replaces the data without a reload. If the export cannot be loaded at all, the server
markup stays as it is and the load is retried with a growing delay. `window.__RWA_BUILD__` holds the build the page hydrated with (e2e tests).
Without server markup (dev server) the app renders on the client.

The `<head>` of every page comes from `src/utils/pageMeta.ts` (title, description, breadcrumbs) and `src/utils/seo.ts`
(canonical, `hreflang` en / zh-CN / x-default, Open Graph, JSON-LD: `WebSite`, `Organization`, `WebPage`, `Dataset` +
`ItemList` on listing pages, `BreadcrumbList` on detail pages).

- Output that depends on the viewer (relative "2d ago" times, media queries) must not differ between the server render and
  the first browser render: use `hooks/useHydrated` and responsive `sx` values instead of `useMediaQuery` for layout.
  Data hooks that are not part of the first render (`useStatus`, `useRuns`, `useHistory`) fetch only in the browser.
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
