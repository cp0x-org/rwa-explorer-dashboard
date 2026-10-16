# Tokenized Stocks in DeFi by cp0x

Research site: tokenized assets (on-chain tokens of stocks/ETFs, CoinMarketCap) and their usage in DeFi (Portals), with
ratings of tokenized assets and issuers. Based on the Berry MUI template (React 19 + Vite + MUI 7). Read-only: no wallet, no transactions.

## Data

The site reads one static file, `public/data/rwa.json`, produced by `../rwa-data-pipeline`
(`uv run rwa-pipeline sync` / `uv run rwa-pipeline build`). The browser never calls CMC or Portals.
Types of the file: `src/types/rwa.ts`; loading: `src/api/rwa.ts`.
Token and issuer logos are published by the pipeline to `public/logos/`.

## Pages

- `/assets` — underlying assets of the basket; `/assets/:ticker` — comparison of its tokenized assets
- `/tokenized` — all tokenized assets in one sortable, filterable table
- `/tokens/:cmcId` — one tokenized asset: deployments and their Portals status, positions (direct and via LP → vault), breakdowns
- `/issuers` — issuer rating; `/issuers/:issuerId` — issuer's tokenized assets in the basket
- `/opportunities` — all token–position links with filters (asset, issuer, network, category, protocol, status, link)
- `/summary` — charts: issuers' share of eligible positions, yield (APY > 0 only), breakdown per underlying

## Getting Started

```bash
pnpm install
pnpm start        # dev server
pnpm build        # tsc + vite build + prerender of every page into dist/
pnpm lint         # eslint
```

## Prerender

`pnpm build` also writes static HTML for every page (`dist/<path>/index.html`; `dist/index.html` is the assets page), so
pages are readable without JavaScript — crawlers, AI agents, link previews — and show the data before the script loads.
`vite build --ssr src/entry-server.tsx` bundles the app for Node into `dist-ssr/`, `scripts/prerender.mjs` renders each
route with the data from `dist/data/rwa.json`; titles and descriptions come from `src/utils/pageMeta.ts`. The browser
then hydrates the page (`src/index.tsx`) once the data is loaded. With `VITE_SITE_URL` (`.env.production`) it also
writes canonical links, `dist/sitemap.xml` (every prerendered page) and `dist/robots.txt` pointing to it.

- The data is baked in at build time: after `rwa-pipeline sync` rebuild, as before.
- Serve `dist/` as plain static files, not as a single-page app: `serve dist -c ../serve.json` (Dockerfile), not
  `serve -s`, which would answer every path with `dist/index.html`. Unknown paths get `dist/404.html`, the app shell.
- Output that depends on the viewer (relative "2d ago" times, media queries) must not differ between the prerender and
  the first browser render: use `hooks/useHydrated` and responsive `sx` values instead of `useMediaQuery` for layout.
- A row that opens a page gets a real link in its name cell (`RowLink`, chart rows via `to`): crawlers and agents
  follow links, not `onClick`.

## Application Links
- Website: [pi.cp0x.com](https://pi.cp0x.com/)
- Twitter: [@cp0xdotcom](https://x.com/cp0xdotcom)
- Telegram: [@cp0xdotcom](https://t.me/cp0xdotcom)

## Contributions

For steps on local deployment, development, and code contribution, please see [CONTRIBUTING](./CONTRIBUTING.md).
