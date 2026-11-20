# Workflow: from data collection to the published site

All paths below are relative to the repository root.

Two projects, one data file. Nothing has to be copied by hand: the pipeline writes straight into the frontend folder.

```
rwa-data-pipeline/                                   pi-rwa-interface/
config/assets.json ──► sync (CMC + Portals) ──► data/snapshots/*.json
                                                     │
                       logos (CMC, websites) ──► data/logos/**
                                                     │
                       build (no network) ──────► public/data/rwa.json ──► pnpm mock:api  (2.0: the file is served as build 1 of the API)
                                             └──► public/logos/**     ──► pnpm build && pnpm serve  (pages rendered per request)
                       export (no network) ─────► data/exports/rwa.xlsx, data/exports/csv/*.csv   (manual analysis)
```

`sync` runs `logos` and `build` by itself, so one command refreshes everything up to the frontend JSON.
The site (2.0) no longer bakes the data into `dist/`: `server/` renders every page on request from the API's
`/api/v1/export`. Until `rwa-backend` takes over the collection, `pnpm mock:api` serves `public/data/rwa.json` as that
API, so the frontend can be run and tested with the 1.x pipeline's output. `pnpm build` is needed for code changes only.

## 1. One-time setup

```bash
cd rwa-data-pipeline && uv sync && cp .env.example .env   # put CMC_API_KEY and PORTALS_API_KEY into .env
cd ../pi-rwa-interface && pnpm install
```

## 2. Collect everything from scratch or refresh everything

```bash
cd rwa-data-pipeline
uv run rwa-pipeline sync --refresh             # re-fetches ALL enabled underlyings in config/assets.json:
                                               #   CMC: tokenized assets, contracts, issuers, RWA metadata
                                               #   Portals: contract lookup + text search per underlying (the slow part)
                                               #   then logos, then build -> ../pi-rwa-interface/public/data/rwa.json
uv run rwa-pipeline sync --refresh -v          # same, plus a line per HTTP request
uv run rwa-pipeline sync --refresh --format all   # same, plus data/exports/rwa.xlsx and csv/
```

Progress is printed per step (`[1/5] CMC ...`, `[5/5] Portals ...`) and per underlying (`[17/125] NVDA: search 'NVDA'
-> 412 positions ...`, with the estimated time left). Budget: about 4–5 s per underlying, 125 underlyings ≈ 10 min,
≈ 10 CMC credits. Each underlying is saved as soon as it is done, so an interrupted run keeps what it finished;
a failed underlying keeps its previous snapshot (exit code 2) and can be redone with `--symbols`.

## 3. Everyday changes

```bash
uv run rwa-pipeline sync                       # only underlyings that have no snapshot yet (after adding to assets.json)
uv run rwa-pipeline sync --refresh --symbols NVDA,TSLA   # refresh a few tickers
uv run rwa-pipeline build                      # recompute ratings after editing config/settings.json, no network
uv run rwa-pipeline logos                      # fetch logos that were missing (e.g. after placing a file by hand)
uv run rwa-pipeline export                     # tables for manual analysis -> data/exports/rwa.xlsx (or --format csv)
```

## 4. Publish the site

```bash
cd pi-rwa-interface
pnpm mock:api                                  # mock API on :8080 serving public/data/rwa.json (restart after a new build of the JSON)
pnpm start                                     # dev server (client render): /api is proxied to :8080 (or VITE_API_URL)
pnpm build && pnpm serve                       # production-like: tsc + vite + SSR bundle, then the server on :4173 (API_URL=http://127.0.0.1:8080)
./build.sh <version>                           # Docker image (dist/, dist-ssr/, server/; see Dockerfile), after pnpm build is green
```

Check: `data/runs/<run-id>.json` has the request counts and failures of the last sync; `public/data/rwa.json` ->
`generated_at` shows the build time; the site footer/summary page shows `newest_fetched_at`.

## Order of operations, in one line

`sync --refresh` (data) → *(automatic: logos → build → JSON in the frontend)* → `pnpm mock:api` / the real API → the site renders from it.
