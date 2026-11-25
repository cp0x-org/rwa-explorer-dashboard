# Workflow: from data collection to the published site (2.0)

All paths are relative to the repository root. Deployment itself (compose files, reverse proxy, secrets) lives in the
private pi-infra repository, `stacks/rwa`.

```
CMC + Portals ──► collector ──► snapshots ──► publish ──► builds + current state + history ──► logos
 (cron, 0 6 * * * UTC;           (one per        (ratings,        │
  manual: run-once / admin API)   underlying)     export)         ▼
                                                            rwa api ──► site (SSR) ──► /updates, charts
```

## 1. Scheduled run

The `rwa collector` container runs one job per schedule tick (`COLLECT_CRON`, default `0 6 * * *`, UTC, or
`COLLECT_INTERVAL`). On start it also runs once when the last successful run is stale (`COLLECT_ON_START`). A job:

1. collect every enabled underlying from CoinMarketCap and Portals into `snapshots` (a failed underlying keeps its
   previous snapshot);
2. publish: compute ratings, store the export as a new build, update the current-state tables, add history rows;
3. fetch missing logos (a rebuild of the export follows when new logos arrived);
4. delete snapshots older than `SNAPSHOT_RETENTION_DAYS`.

A publish failure leaves the previous build current and is recorded on the run. Jobs never overlap. The site picks up a
new build by itself (it polls `/api/v1/status`), so there is no rebuild or redeploy.

## 2. Manual run

On the server, in the stack directory:

```bash
docker compose exec rwa-collector rwa run-once                      # refresh every enabled underlying
docker compose exec rwa-collector rwa run-once --symbols NVDA,TSLA  # a subset
docker compose exec rwa-collector rwa run-once --mode add           # only underlyings without a snapshot
```

Or through the admin API (enabled when `ADMIN_TOKEN` is set in the server `.env`; the collector picks the request up
within 30 s):

```bash
curl -X POST https://rwa.cp0x.com/api/v1/admin/runs \
  -H "Authorization: Bearer $ADMIN_TOKEN" -H "Content-Type: application/json" \
  -d '{"mode":"refresh","symbols":["NVDA"]}'          # 202 {"request_id": N}
curl -H "Authorization: Bearer $ADMIN_TOKEN" https://rwa.cp0x.com/api/v1/admin/runs/N   # outcome, run_id, error
```

`mode` is `refresh`, `add` or `rebuild`. Take `ADMIN_TOKEN` from the server `.env`; never paste it into files or chats.

## 3. Everyday changes

| Change                                  | Do this                                                                                          |
|-----------------------------------------|--------------------------------------------------------------------------------------------------|
| Rating rules (`settings.json`) changed  | `rwa build`: recomputes the export from stored snapshots, no network, no history rows            |
| Add an underlying                       | add an entry with a valid ISIN to `rwa-backend/config/assets.json` (and the copy in the stack), restart the collector, or run `rwa run-once --mode add` |
| Refresh one ticker                      | `rwa run-once --symbols NVDA`                                                                    |
| Run stopped after the snapshots         | `rwa publish --run <id>` (after `run-once --no-publish`, or a failed publish)                    |

`enabled: false` in `assets.json` removes an underlying from the basket but keeps its snapshots.
`rwa-backend/config/*.json` are the defaults baked into the image; the stack mounts its own copies over them.

## 4. History and `/updates`

Every publish of a collection run adds history points (summary, underlyings, tokens, issuers, positions). A `rebuild`
adds none. The site shows them as charts on `/summary`, the asset, token and issuer pages. `/updates` lists the runs
(status, duration, requests, credits, changes); `/updates/:id` shows warnings and per-asset results of one run.

## 5. After a run, check

| Check                    | What to look for                                                                       |
|--------------------------|----------------------------------------------------------------------------------------|
| `GET /api/v1/status`     | `current_build` (id, `generated_at`), `last_run` (status, counts), `next_run_at`                       |
| `GET /api/v1/runs`       | the run has status `succeeded` (or `partial` with the failed underlyings listed)       |
| `/updates` on the site   | the run is the newest row, warnings and changes look plausible                         |

```bash
curl -s https://rwa.cp0x.com/api/v1/status
curl -s https://rwa.cp0x.com/api/v1/runs
```

## 6. Local development flow

```bash
cd rwa-backend && make dev-db && make migrate
go run ./cmd/rwa run-once                                         # first live run into the empty database
go run ./cmd/rwa api                                              # needs DATABASE_URL in .env
cd ../rwa-interface && pnpm install && pnpm start                 # /api proxied to localhost:8080
```

`go run ./cmd/rwa run-once` collects for real and needs `CMC_API_KEY` and `PORTALS_API_KEY` in `rwa-backend/.env`
(about 4–5 s per underlying, so a full refresh takes minutes). Without the backend: `pnpm mock:api` in `rwa-interface/` (frozen 1.x data). Step by step: [run-locally.md](run-locally.md).

To run the whole stack from source in Docker (database, api, site, collector) use the root `docker-compose.yml`:
see "Run everything with Docker (local)" in the root README.
