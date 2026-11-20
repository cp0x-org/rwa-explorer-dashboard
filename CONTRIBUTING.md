# Contributing

Thank you for your interest in RWA Explorer. This file covers the projects in the repository: the backend
(`rwa-backend/`, Go), the site (`rwa-interface/`, TypeScript/React) and the legacy 1.x pipeline (`rwa-data-pipeline/`, Python).

## Prerequisites

- Go 1.26 (`go.mod` pins the toolchain)
- Docker, for the development database (`make dev-db`)
- Node.js 22 and pnpm: run `corepack enable` once, the pnpm version comes from `package.json`
- API keys for CoinMarketCap and Portals, only to collect new data; the 1.x data in `rwa-data-pipeline/data` can be
  imported without keys (`rwa import-v1`)

## Setup

```bash
cd rwa-backend && make dev-db && make migrate && cp .env.example .env   # keys into .env, never commit it
cd ../rwa-interface && pnpm install
```

The full local run is in the root [README.md](README.md#quick-start-development).

## Workflow

1. Create a branch from `main`.
2. Make the change together with its tests and documentation (the READMEs describe behaviour, keep them true).
3. Run the checks of every project you touched (below).
4. Open a pull request with a short description of what changed and why. Keep data refreshes
   (data imports, `config/assets.json`) in separate commits from code changes, so the diff of the code stays readable.

Commit messages: one line in the imperative, e.g. `Add ProtocolsCell to the tokenized assets table`.

## Backend (`rwa-backend/`)

```bash
make build         # bin/rwa
make test          # unit tests; integration tests are skipped without TEST_DATABASE_URL
make lint          # golangci-lint (config v2 in .golangci.yml); go vet only if it is not installed
make dev-db        # PostgreSQL 16 on 127.0.0.1:5433
export TEST_DATABASE_URL='postgres://rwa:rwa@127.0.0.1:5433/rwa?sslmode=disable'
make test-integration   # db and store tests against the dev database
make parity        # Go compute vs the 1.x export: must report 0 differences
```

Conventions:

- **Migrations are append-only.** SQL files in `internal/db/migrations/` (`NNNN_<name>.sql`). One new file per schema
  change; never edit a file that has been applied anywhere.
- **Compute changes keep parity.** A change to ratings or the export must keep `make parity` at 0 differences. Until 2.1
  removes the 1.x pipeline, a deliberate difference needs an explanation in the pull request.
- **Secrets only through the environment** (`.env`, git-ignored). Keys must never reach logs, snapshots, exports or
  test fixtures; the config masks them and a test enforces it.

## Site (`rwa-interface/`)

```bash
pnpm lint          # eslint
pnpm test          # vitest: dictionary parity, locale paths, page metadata, JSON-LD
pnpm build         # tsc + vite build + SSR bundle (pages are rendered per request by server/)
pnpm test:e2e      # playwright: starts the mock API and the server itself (run pnpm build first): rendering, SEO tags, i18n, hydration, interactions, axe
pnpm serve         # the SSR server like production, http://localhost:4173 (API_URL, default http://127.0.0.1:8080: run the backend or pnpm mock:api)
pnpm mock:api      # mock API on :8080 serving public/data/rwa.json, to work without the backend
```

`pnpm test:e2e` needs Chromium for Playwright once: `npx playwright install chromium`.

Conventions:

- **Languages.** `src/i18n/messages/en.ts` is the source of truth; `zh.ts` must define exactly the same keys
  (`tsc` and `pnpm test` fail otherwise). Components use `useT()` and `useFormat()`; links use `LocaleLink` /
  `useLocaleNavigate` with locale-free paths. Tickers, symbols, addresses and protocol/network/issuer names are data
  and never go through the dictionaries.
- **Server render.** Output must not differ between the server render and the first browser render: use
  `hooks/useHydrated` and responsive `sx` values instead of `useMediaQuery`. Rows that open a page get a real link
  (`RowLink`), not only `onClick`.
- **Accessibility.** One `h1` per page, landmarks (`header` / `nav` / `main` / `footer`), captions and `aria-sort` on
  tables, every control with an accessible name. The e2e axe suite enforces WCAG 2.2 AA.
- **Dependencies.** Avoid adding dependencies: supply-chain risk, and the build must stay deterministic
  (`pnpm install --frozen-lockfile` in the Dockerfile).
- **Data.** The site reads only our API (`/api/v1/...`, `/logos/...`); never call external APIs from the browser. `public/data/rwa.json` is the mock API's fixture, not a data source.

## Legacy pipeline (1.x)

`rwa-data-pipeline/` is kept only for `rwa import-v1` and `make parity` and will be removed in 2.1. Do not add features
to it; its tests (`uv run pytest`) must keep passing while it exists.

## Documentation

- `README.md` (root): overview and quick start
- `rwa-backend/README.md`, `rwa-interface/README.md`: each project in detail
- `rwa-data-pipeline/README.md`: the legacy 1.x pipeline
- `docs/workflow.md`: the 2.0 flow from collection to the published site
- `docs/i18n-ssr-seo-plan.md`: i18n, SEO and accessibility design notes and audit (history; the prerender part was replaced by the SSR server)

When behaviour changes, update the matching file in the same pull request.

## Reporting issues

Open a GitHub issue with steps to reproduce; for site bugs include the URL and language, for backend bugs the command,
the ticker and the run id (`/updates/<id>` or `GET /api/v1/runs/<id>`). Security issues: see [SECURITY.md](SECURITY.md).
