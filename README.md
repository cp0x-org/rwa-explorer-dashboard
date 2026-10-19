# RWA Explorer — Tokenized Stocks in DeFi by cp0x

Research site about tokenized stocks and ETFs: which on-chain tokens exist for an underlying asset (CoinMarketCap),
where they are used in DeFi (Portals), and how tokenized assets and their issuers rate against each other.
Read-only: no wallet, no transactions. Live at [rwa.cp0x.com](https://rwa.cp0x.com/), in English and Simplified Chinese.

This repository holds both halves of the product:

| Folder                                     | What it is                                                                                        | Stack                                |
|--------------------------------------------|---------------------------------------------------------------------------------------------------|--------------------------------------|
| [`rwa-data-pipeline/`](rwa-data-pipeline/) | Collects tokenized assets and their DeFi positions, computes ratings, publishes one JSON file      | Python 3.12, uv, httpx, typer        |
| [`pi-rwa-interface/`](pi-rwa-interface/)   | The site: reads that JSON, prerenders every page in both languages, served as static files         | React 19, Vite, MUI 7, react-intl    |
| [`docs/`](docs/)                           | Cross-project documentation: the end-to-end workflow and design notes                             |                                      |

## How it works

```
rwa-data-pipeline/                                        pi-rwa-interface/
config/assets.json ──► sync (CMC + Portals) ──► data/snapshots/*.json
                                                          │
                       logos (CMC, websites) ──► data/logos/**
                                                          │
                       build (no network) ──────────► public/data/rwa.json ──► pnpm start   (dev)
                                                 └──► public/logos/**     ──► pnpm build   (dist/ with prerendered pages)
                       export (no network) ─────► data/exports/rwa.xlsx, csv/*.csv   (manual analysis)
```

- The pipeline writes straight into the frontend folder; nothing is copied by hand.
- The browser never calls CoinMarketCap or Portals. API keys live only in `rwa-data-pipeline/.env`.
- `pnpm build` bakes the data into `dist/` (static HTML for every page, sitemap, robots), so a data refresh is
  followed by a site rebuild.

Step-by-step commands from collection to deploy: [docs/workflow.md](docs/workflow.md).

## Quick start

Prerequisites: [uv](https://docs.astral.sh/uv/) (installs Python itself), Node.js 22 with pnpm (`corepack enable`
picks the version from `package.json`), API keys for CoinMarketCap and Portals.

```bash
# 1. data pipeline
cd rwa-data-pipeline
uv sync
cp .env.example .env            # put CMC_API_KEY and PORTALS_API_KEY into .env
uv run rwa-pipeline sync        # fetch underlyings without a snapshot, then build -> ../pi-rwa-interface/public/data/rwa.json
uv run pytest

# 2. site
cd ../pi-rwa-interface
pnpm install
pnpm start                      # dev server, reads public/data/rwa.json
pnpm build                      # tsc + vite build + prerender into dist/
pnpm test && pnpm lint
```

The repository already ships snapshots and the published JSON, so the site runs without API keys; keys are needed
only to refresh or extend the data.

## Pipeline in short

```bash
uv run rwa-pipeline sync                         # add underlyings that have no snapshot yet
uv run rwa-pipeline sync --refresh               # re-fetch every enabled underlying
uv run rwa-pipeline sync --refresh --symbols NVDA
uv run rwa-pipeline build                        # recompute ratings from saved snapshots, no network
uv run rwa-pipeline export                       # flat tables: data/exports/rwa.xlsx (or --format csv)
```

Underlyings are listed in `rwa-data-pipeline/config/assets.json`, all rating rules in `config/settings.json`.
What each command does, how the numbers are computed and the known limitations: [rwa-data-pipeline/README.md](rwa-data-pipeline/README.md).

## Site in short

| Path             | Page                                                                    |
|------------------|-------------------------------------------------------------------------|
| `/assets`        | underlying assets of the basket; `/assets/:ticker` compares its tokens  |
| `/tokenized`     | all tokenized assets in one sortable, filterable table                  |
| `/tokens/:cmcId` | one tokenized asset: deployments, positions, breakdowns                 |
| `/issuers`       | issuer rating; `/issuers/:issuerId` lists the issuer's tokens           |
| `/opportunities` | every token–position link with filters                                  |
| `/summary`       | charts: issuer shares, yield, breakdown per underlying                  |

Every page also exists under `/zh/...`. Prerendering, i18n, SEO and accessibility rules:
[pi-rwa-interface/README.md](pi-rwa-interface/README.md) and [docs/i18n-ssr-seo-plan.md](docs/i18n-ssr-seo-plan.md).

## Deployment

The site is a Docker image with `dist/` served as plain static files (clean URLs, real 404s):

```bash
cd pi-rwa-interface
./build.sh <version>            # multi-arch image cp0x/pi-rwa-interface:<version>, pushed to the registry
docker compose up -d            # or run it locally on http://localhost:3301
```

Rebuild the image after every `rwa-pipeline sync`, because the data is part of the build.

## Links

- Website: [rwa.cp0x.com](https://rwa.cp0x.com/)
- Twitter: [@cp0xdotcom](https://x.com/cp0xdotcom)
- Telegram: [@cp0xdotcom](https://t.me/cp0xdotcom)

## Contributing and license

Development setup, checks to run before a pull request and project conventions: [CONTRIBUTING.md](CONTRIBUTING.md).
Reporting a vulnerability or a leaked key: [SECURITY.md](SECURITY.md).

Both projects are released under the GNU General Public License v3.0, see [LICENSE](LICENSE).
