# Tokenized Stocks in DeFi by cp0x

Research site: tokenized versions of stocks/ETFs (CoinMarketCap) and their usage in DeFi (Portals), with ratings of
versions and issuers. Based on the Berry MUI template (React 19 + Vite + MUI 7). Read-only: no wallet, no transactions.

## Data

The site reads one static file, `public/data/rwa.json`, produced by `../rwa-data-pipeline`
(`uv run rwa-pipeline sync` / `uv run rwa-pipeline build`). The browser never calls CMC or Portals.
Types of the file: `src/types/rwa.ts`; loading: `src/api/rwa.ts`.

## Pages

- `/assets` — underlying assets of the basket; `/assets/:ticker` — comparison of its tokenized versions
- `/tokenized` — all tokenized versions, one comparison table per underlying
- `/tokens/:cmcId` — one version: deployments and their Portals status, positions (direct and via LP → vault), breakdowns
- `/issuers` — issuer rating; `/issuers/:issuerId` — issuer's versions in the basket
- `/opportunities` — all token–position links with filters (asset, issuer, network, category, protocol, status, link)

## Getting Started

```bash
pnpm install
pnpm start        # dev server
pnpm build        # tsc + vite build
pnpm lint         # eslint
```

## Application Links
- Website: [pi.cp0x.com](https://pi.cp0x.com/)
- Twitter: [@cp0xdotcom](https://x.com/cp0xdotcom)
- Telegram: [@cp0xdotcom](https://t.me/cp0xdotcom)

## Contributions

For steps on local deployment, development, and code contribution, please see [CONTRIBUTING](./CONTRIBUTING.md).
