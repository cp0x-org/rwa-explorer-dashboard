# Contributing

Thank you for your interest in RWA Explorer. This file covers both projects in the repository:
the data pipeline (`rwa-data-pipeline/`, Python) and the site (`pi-rwa-interface/`, TypeScript/React).

## Prerequisites

- [uv](https://docs.astral.sh/uv/) — installs Python 3.12+ by itself
- Node.js 22 and pnpm — run `corepack enable` once, the pnpm version comes from `package.json`
- API keys for CoinMarketCap and Portals, only if you need to refresh the data
  (the repository ships snapshots and the published JSON, so the site builds without keys)

## Setup

```bash
cd rwa-data-pipeline && uv sync && cp .env.example .env     # keys into .env
cd ../pi-rwa-interface && pnpm install
```

## Workflow

1. Create a branch from `main`.
2. Make the change together with its tests and documentation (the READMEs describe behaviour, keep them true).
3. Run the checks of every project you touched (below).
4. Open a pull request with a short description of what changed and why. Keep data refreshes
   (`rwa-pipeline sync`) in separate commits from code changes, so the diff of the code stays readable.

Commit messages: one line in the imperative, e.g. `Add ProtocolsCell to the tokenized assets table`.

## Site (`pi-rwa-interface/`)

```bash
pnpm lint          # eslint
pnpm test          # vitest: dictionary parity, locale paths, page metadata, JSON-LD
pnpm build         # tsc + vite build + prerender of every page in both languages
pnpm test:e2e      # playwright against dist/ (run pnpm build first): rendering, SEO tags, i18n, interactions, axe
pnpm serve         # serve dist/ like production does, http://localhost:4173
```

`pnpm test:e2e` needs Chromium for Playwright once: `npx playwright install chromium`.

Conventions:

- **Languages.** `src/i18n/messages/en.ts` is the source of truth; `zh.ts` must define exactly the same keys
  (`tsc` and `pnpm test` fail otherwise). Components use `useT()` and `useFormat()`; links use `LocaleLink` /
  `useLocaleNavigate` with locale-free paths. Tickers, symbols, addresses and protocol/network/issuer names are data
  and never go through the dictionaries.
- **Prerender.** Output must not differ between the prerender and the first browser render: use
  `hooks/useHydrated` and responsive `sx` values instead of `useMediaQuery`. Rows that open a page get a real link
  (`RowLink`), not only `onClick`.
- **Accessibility.** One `h1` per page, landmarks (`header` / `nav` / `main` / `footer`), captions and `aria-sort` on
  tables, every control with an accessible name. The e2e axe suite enforces WCAG 2.2 AA.
- **Dependencies.** Avoid adding dependencies: supply-chain risk, and the build must stay deterministic
  (`pnpm install --frozen-lockfile` in the Dockerfile).
- **Data.** The site reads only `public/data/rwa.json` and `public/logos/`; never call external APIs from the browser.

## Pipeline (`rwa-data-pipeline/`)

```bash
uv run pytest                      # tests (fixtures in tests/fixtures/, no network)
uv run rwa-pipeline build          # recompute ratings from saved snapshots, no network
uv run rwa-pipeline sync --refresh --symbols NVDA   # re-fetch one underlying to check a collector change
```

Conventions:

- **Rules live in `build`.** Thresholds, categories and the chain mapping are in `config/settings.json`; changing them
  needs only `build`, not new requests. Collection (`sync`) stores raw facts, `build` interprets them.
- **Snapshots are the source.** `sync` replaces a snapshot only after all requests for that underlying succeeded.
  Never edit `data/snapshots/*.json` by hand; add or fix an asset in `config/assets.json` and run `sync`.
- **Secrets.** Keys are read from `.env` / environment only and must never appear in snapshots, raw responses, run
  reports or exports. `.env` and `data/raw/` are git-ignored; keep it that way.
- **Adding an underlying.** Add an entry with a valid ISIN to `config/assets.json`, run `uv run rwa-pipeline sync`,
  then rebuild the site. `enabled: false` removes an asset from the basket but keeps its snapshot.

## Documentation

- `README.md` (root) — overview and quick start
- `rwa-data-pipeline/README.md`, `pi-rwa-interface/README.md` — each project in detail
- `docs/workflow.md` — the full flow from collection to deploy
- `docs/i18n-ssr-seo-plan.md` — i18n, prerender, SEO and accessibility design notes and audit

When behaviour changes, update the matching file in the same pull request.

## Reporting issues

Open a GitHub issue with steps to reproduce; for site bugs include the URL and language, for pipeline bugs include
the command, the ticker and the relevant `data/runs/<run-id>.json`. Security issues: see [SECURITY.md](SECURITY.md).
