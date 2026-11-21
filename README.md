# RWA Explorer — Tokenized Stocks in DeFi by cp0x

Research site about tokenized stocks and ETFs: which on-chain tokens exist for an underlying asset (CoinMarketCap),
where they are used in DeFi (Portals), and how tokenized assets and their issuers rate against each other.
Read-only: no wallet, no transactions. Live at [rwa.cp0x.com](https://rwa.cp0x.com/), in English and Simplified Chinese.

Version 2.0: a Go collector writes to PostgreSQL on a schedule, an API serves the data, and the site renders every page
on a Node server from that API. History of every number is kept and shown as charts and on `/updates`.

| Folder                                     | What it is                                                                          | Stack                                  |
|--------------------------------------------|-------------------------------------------------------------------------------------|----------------------------------------|
| [`rwa-backend/`](rwa-backend/)             | Collector (CMC + Portals, scheduled), ratings, history, logos, HTTP API             | Go 1.26, PostgreSQL 16                 |
| [`rwa-interface/`](rwa-interface/)         | The site: server-side rendering from the API, English and Chinese                   | React 19, Vite, MUI 7, Node SSR server |
| [`rwa-data-pipeline/`](rwa-data-pipeline/) | Legacy 1.x pipeline; used for `rwa import-v1` and the parity test (`make parity`)   | Python 3.12, uv                        |
| [`docs/`](docs/)                           | Cross-project documentation: the workflow and design notes                          |                                        |

## How it works

```
CoinMarketCap ─┐
               ├─► rwa collector ──► PostgreSQL ──► rwa api ──► site (Node SSR server) ──► browser
Portals ───────┘   (cron, default     snapshots                /api/v1/export            pages in EN / ZH,
                    0 6 * * * UTC)    current state            /api/v1/status, runs,     history charts,
                                      history                  history                   /updates
                                      builds (export)          /logos/...
                                      logos
```

- The collector fetches every underlying, stores the raw snapshots, computes ratings, publishes a new build (one export
  document) with history rows, then fetches missing logos.
- The browser never calls CoinMarketCap or Portals. API keys live only on the server and in developers' local `.env`.
- A data refresh needs no site rebuild: the site reads the newest build from the API.

Step by step, including manual runs: [docs/workflow.md](docs/workflow.md).

**Short runbook for both ways (local and Docker, with the real backend): [docs/run-locally.md](docs/run-locally.md).**

## Run everything with Docker (local)

Runs the whole project from source on your machine: database, import of the 1.x data, API, site and the collector, so
you can watch a collection run, the parsing and the result on the site. Not the production stack.

Prerequisites: Docker Desktop (Compose v2), a CoinMarketCap and a Portals API key.

```bash
cp .env.example .env            # put CMC_API_KEY and PORTALS_API_KEY into .env (ignored by Git, never commit it)
docker compose up --build       # first build takes a few minutes; `make up` does the same in the background
```

Then open http://localhost:3301 (API on http://localhost:3302). Other ports: `WEB_PORT` / `API_PORT` in `.env`.

What happens, in order:

1. `postgres` starts; `migrate` applies the migrations and exits.
2. `import` loads the 1.x data from `rwa-data-pipeline/data` as the first run (about 125 snapshots, no keys needed).
   It is idempotent: the next `up` prints "import skipped".
3. `api` and `web` come up; the site already shows the October data.
4. `collector` starts a live run at once, because the imported data is stale (`COLLECT_ON_START=true`). It takes about
   7 minutes; the site then switches to the new build.

Where to look:

- site: http://localhost:3301; `/updates` shows the runs and the journal of each run
- API status: http://localhost:3302/api/v1/status
- the run itself: `docker compose logs -f collector` (or `make logs` for collector, api and web)

Another run: `make run` (`docker compose exec collector rwa run-once`; a subset:
`docker compose exec collector rwa run-once --symbols NVDA`), or through the admin API with the dev token
(`ADMIN_TOKEN`, default `devtoken`; the collector picks the request up within 30 s):

```bash
curl -X POST http://localhost:3302/api/v1/admin/runs -H "Authorization: Bearer devtoken" \
  -H "Content-Type: application/json" -d '{"mode":"refresh","symbols":["NVDA"]}'
```

Stop with `make down` (data stays in the `rwa-pgdata` volume); `make reset` also deletes the database and asks first.
`make psql` opens a SQL shell.

A full live run costs about 22 CMC credits (`COLLECT_ON_START=false` in `.env` skips the run on start). The keys stay in
`.env`, which Git ignores. The database credentials (`rwa`/`rwa`) are for local use only, and the ports are bound to
127.0.0.1.

Which file to use: this root `docker-compose.yml` runs everything; `rwa-backend/docker-compose.dev.yml` is only a
PostgreSQL on 127.0.0.1:5433 for working on the code with `go run` and `pnpm start` (see below).

## Quick start (development)

Prerequisites: Go 1.26, Docker (for the dev database), Node.js 22 with pnpm (`corepack enable` picks the version from
`package.json`), API keys for CoinMarketCap and Portals (only to collect new data).

```bash
# 1. database and data
cd rwa-backend
make dev-db && make migrate                 # PostgreSQL 16 on 127.0.0.1:5433
cp .env.example .env                        # put CMC_API_KEY and PORTALS_API_KEY into .env (never commit it)
go run ./cmd/rwa import-v1 --data-dir ../rwa-data-pipeline/data   # load the 1.x data: no keys needed
go run ./cmd/rwa api                        # API on http://localhost:8080 (HTTP_ADDR)

# 2. site (second terminal)
cd rwa-interface
pnpm install
pnpm start                                  # dev server, client render; /api and /logos are proxied to http://localhost:8080
```

`pnpm start` proxies `/api` and `/logos` to `VITE_API_URL`, or to `http://localhost:8080` when it is unset. Set
`VITE_API_URL` only for an API on another host; the browser then calls it directly and the API needs `CORS_ORIGINS`.

| Command (in `rwa-interface/`)       | What it does                                                                                  |
|-------------------------------------|-----------------------------------------------------------------------------------------------|
| `pnpm build && pnpm serve`          | production-like: client bundle and SSR bundle, then the SSR server on http://localhost:4173    |
| `pnpm dev:ssr`                      | build, then the SSR server on :4173 against the real API on :8080 (`rwa api`)                  |
| `pnpm mock:api` / `pnpm mock:ssr`   | MOCK API on :8080 with the frozen 1.x snapshot (`public/data/rwa.json`): no backend, old data  |
| `pnpm test && pnpm lint`            | unit tests and eslint                                                                          |
| `pnpm test:e2e`                     | Playwright; starts the mock API and the server itself (run `pnpm build` first)                 |

`pnpm serve` reads the API at `API_URL` (default `http://127.0.0.1:8080`).

## Backend in short

One binary, `rwa`. Run from `rwa-backend/` with `go run ./cmd/rwa <command>` or `bin/rwa <command>` (`make build`).

| Command                                  | What it does                                                                                       |
|------------------------------------------|----------------------------------------------------------------------------------------------------|
| `rwa run-once [--mode add] [--symbols NVDA,TSLA] [--no-publish]` | one collection job without the scheduler: collect, publish, logos. Exit 0 succeeded, 2 partial or usage error, 1 failed |
| `rwa publish --run <id>`                 | publish an existing collection run (the way to finish `run-once --no-publish`)                     |
| `rwa build [--run <id>]`                 | recompute the export from stored snapshots, no network, no history rows                            |
| `rwa import-v1 --data-dir <path> [--force]` | load 1.x pipeline data into an empty database; skips when already imported (exit 0)             |
| `rwa collector`                          | long-running scheduler (`COLLECT_CRON` or `COLLECT_INTERVAL`) and poller of manual run requests    |
| `rwa migrate up \| status \| down-to <v> --yes` | database migrations                                                                         |
| `rwa probe --provider cmc\|portals --symbols NVDA` | dev only: live check of the API keys and source clients                                  |

Configuration, endpoints and details: [rwa-backend/README.md](rwa-backend/README.md).

## Site in short

| Path             | Page                                                                              |
|------------------|-----------------------------------------------------------------------------------|
| `/assets`        | underlying assets of the basket; `/assets/:ticker` compares its tokens            |
| `/tokenized`     | all tokenized assets in one sortable, filterable table                            |
| `/tokens/:cmcId` | one tokenized asset: deployments, positions, breakdowns, history                  |
| `/issuers`       | issuer rating; `/issuers/:issuerId` lists the issuer's tokens                     |
| `/opportunities` | every token–position link with filters                                            |
| `/summary`       | charts: issuer shares, yield, breakdown per underlying; trends of the basket      |
| `/updates`       | data collection runs; `/updates/:id` shows one run (warnings, per-asset results)  |

Every page also exists under `/zh/...`. Server rendering, i18n, SEO and accessibility rules:
[rwa-interface/README.md](rwa-interface/README.md) and [docs/i18n-ssr-seo-plan.md](docs/i18n-ssr-seo-plan.md).

## Deployment

Two Docker images, both built per folder:

```bash
cd rwa-backend && ./build.sh <version>      # multi-arch image cp0x/pi-rwa-backend:<version>, pushed to the registry
cd rwa-interface && ./build.sh <version>    # multi-arch image cp0x/pi-rwa-interface:<version>, pushed to the registry
```

The backend image runs `rwa api` and `rwa collector` (two containers). The stack (compose files, reverse proxy,
secrets) lives in the private pi-infra repository, `stacks/rwa`. nginx there routes `/api/` and `/logos/` to the API and
everything else to the site. A data refresh needs no rebuild or redeploy; images change only with code.

## 1.x (legacy)

`rwa-data-pipeline/` is the Python pipeline of 1.x. The site no longer reads its output. It stays as the parity
reference (`make parity` in `rwa-backend/` compares the Go compute with `rwa-interface/public/data/rwa.json`) and as the
source for `rwa import-v1`. It will be removed in 2.1. Its own README: [rwa-data-pipeline/README.md](rwa-data-pipeline/README.md).

## Links

- Website: [rwa.cp0x.com](https://rwa.cp0x.com/)
- Twitter: [@cp0xdotcom](https://x.com/cp0xdotcom)
- Telegram: [@cp0xdotcom](https://t.me/cp0xdotcom)

## Contributing and license

Development setup, checks to run before a pull request and project conventions: [CONTRIBUTING.md](CONTRIBUTING.md).
Reporting a vulnerability or a leaked key: [SECURITY.md](SECURITY.md).

All projects are released under the GNU General Public License v3.0, see [LICENSE](LICENSE).
