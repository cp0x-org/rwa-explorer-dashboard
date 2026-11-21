# RWA Explorer — Tokenized Stocks in DeFi by cp0x

Research site about tokenized stocks and ETFs: which on-chain tokens exist for an underlying asset (CoinMarketCap),
where they are used in DeFi (Portals), and how tokenized assets and their issuers rate against each other.
Read-only: no wallet, no transactions. Live at [rwa.cp0x.com](https://rwa.cp0x.com/), in English and Simplified Chinese.

This repository holds both halves of the product:

| Folder                                     | What it is                                                                                    | Stack                             |
|--------------------------------------------|-----------------------------------------------------------------------------------------------|-----------------------------------|
| [`rwa-data-pipeline/`](rwa-data-pipeline/) | Collects tokenized assets and their DeFi positions, computes ratings, publishes one JSON file | Python 3.12, uv, httpx, typer     |
| [`rwa-interface/`](rwa-interface/)         | The site: reads that JSON, prerenders every page in both languages, served as static files    | React 19, Vite, MUI 7, react-intl |
| [`docs/`](docs/)                           | Cross-project documentation: the end-to-end workflow and design notes                         |                                   |

## How it works

```
rwa-data-pipeline/                                    rwa-interface/
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
uv run rwa-pipeline sync        # fetch underlyings without a snapshot, then build -> ../rwa-interface/public/data/rwa.json
uv run pytest

# 2. site
cd ../rwa-interface
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
[rwa-interface/README.md](rwa-interface/README.md) and [docs/i18n-ssr-seo-plan.md](docs/i18n-ssr-seo-plan.md).

## Deployment

The site is a Docker image with `dist/` served as plain static files (clean URLs, real 404s):

```bash
cd rwa-interface
./build.sh <version>            # pushes multi-arch cp0x/pi-rwa-interface:<version> and cp0x/pi-rwa-interface:latest
docker compose up -d            # or run it locally on http://localhost:3301
```

Rebuild the image after every `rwa-pipeline sync`, because the data is part of the build.

## Automation

[`ci.yml`](.github/workflows/ci.yml) runs on every pull request and every push to `main` with read-only
repository permissions. It installs locked dependencies, runs the Python pipeline tests, and checks the site
with lint, unit tests, a production build and Chromium Playwright tests. A failed run uploads the Playwright
report and test results as an artifact.

[`update-data.yml`](.github/workflows/update-data.yml) runs daily at 05:17 UTC and can also be started manually
with `workflow_dispatch`. Updates run one at a time on the default branch. Add repository Actions secrets
`CMC_API_KEY` and `PORTALS_API_KEY` to enable `uv run rwa-pipeline sync --refresh --format all`. If either key
is missing, the job only logs a notice and skips the update and publishing steps successfully. Exit codes 0
and 2 are accepted: a partial refresh keeps the previous snapshots for assets that could not be updated.
Before committing or publishing, the workflow runs the Python pipeline tests except those marked `shipped_data`,
which compare committed data with the config and run on pull requests. It also runs site unit tests, a production
build and Chromium Playwright e2e tests against the refreshed data. Any failed check stops the job before
data is committed or an image is published.
Changed pipeline data (excluding `data/raw`) and frontend public files are committed by `github-actions[bot]`
to the default branch with the message `data: daily update <YYYY-MM-DD>`. Before committing, the workflow fetches
the latest default branch. If it moved during the update, nothing is committed or published; the next run
will refresh data using the new code.

To publish the Docker image, also add `DOCKERHUB_USERNAME` and `DOCKERHUB_TOKEN`. After the data refresh, the
workflow builds `rwa-interface` for `linux/amd64` and `linux/arm64` and pushes
`cp0x/pi-rwa-interface:<YYYY-MM-DD>` (the UTC run date) and `cp0x/pi-rwa-interface:latest`. Missing Docker Hub
credentials only produce a notice and skip image publishing; the data update still completes.

For the live site to update, the server must pull and restart the `latest` image, for example with Watchtower
or a timer that runs `docker compose pull && docker compose up -d` in the deployment directory.

## Links

- Website: [rwa.cp0x.com](https://rwa.cp0x.com/)
- Twitter: [@cp0xdotcom](https://x.com/cp0xdotcom)
- Telegram: [@cp0xdotcom](https://t.me/cp0xdotcom)

## Contributing and license

Development setup, checks to run before a pull request and project conventions: [CONTRIBUTING.md](CONTRIBUTING.md).
Reporting a vulnerability or a leaked key: [SECURITY.md](SECURITY.md).

Both projects are released under the GNU General Public License v3.0, see [LICENSE](LICENSE).
