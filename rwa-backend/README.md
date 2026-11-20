# rwa-backend

Go service for RWA Explorer 2.0: a collector (CoinMarketCap + Portals) and an HTTP API over PostgreSQL.
One binary, `rwa`, with subcommands `api`, `collector`, `migrate`, `import-v1`, `run-once`, `build`, `publish`, `probe`
and `version`. All subcommands are implemented. Part of the root [README](../README.md); the 2.0 flow is in [docs/workflow.md](../docs/workflow.md).

## Build and test

Requires Go 1.26 (`go.mod` pins `toolchain go1.26.4`).

```bash
cd rwa-backend
make build        # bin/rwa, version from git describe
make test
make lint         # golangci-lint (config v2, .golangci.yml) if installed, otherwise go vet
make parity       # Go compute vs the 1.x export (rwa-interface/public/data/rwa.json), needs ../rwa-data-pipeline/data
./bin/rwa --help
```

## Configuration

Read from the environment; `.env` (or the file named by `ENV_FILE`) is loaded without overriding set variables.
Copy `.env.example` to `.env`. Keys are never logged: the config masks secrets in `String()`/slog and excludes them from JSON, enforced by a test.

| Variable | Default | Role | Purpose |
|---|---|---|---|
| `DATABASE_URL` | required | api, collector | PostgreSQL connection URL |
| `CMC_API_KEY` | required for collector | collector | CoinMarketCap key |
| `PORTALS_API_KEY` | required for collector | collector | Portals key (a leading `Bearer ` is stripped) |
| `ASSETS_PATH` | `/config/assets.json` | collector | underlyings list |
| `SETTINGS_PATH` | `/config/settings.json` | collector | rating settings |
| `COLLECT_CRON` | `0 6 * * *` | collector | cron schedule, always UTC; a `CRON_TZ=`/`TZ=` prefix is rejected, as is a schedule that never fires |
| `COLLECT_INTERVAL` | empty | collector | alternative to cron, e.g. `12h`; setting both is an error |
| `COLLECT_ON_START` | `true` | collector | run on start if the last successful run is older than the interval (cron: older than 24 h) |
| `MIGRATE_ON_START` | `true` | collector | apply pending migrations when the collector starts; when `false` it refuses to start with pending ones |
| `SNAPSHOT_RETENTION_DAYS` | `90` | collector | snapshot retention |
| `RUN_TIMEOUT` | `2h` | collector | deadline of one job (collect + publish + logos) and of `run-once`; a run that exceeds it ends `failed` with `error='timeout'` |
| `RAW_CAPTURE_DIR` | empty | collector | write raw API responses here (debug) |
| `HTTP_ADDR` | `:8080` | api | listen address |
| `CORS_ORIGINS` | empty | api | comma-separated allowed origins (dev) |
| `ADMIN_TOKEN` | empty | api | enables the admin endpoints when set |
| `ENV_FILE` | `.env` | all | path of the dotenv file to load |
| `LOG_LEVEL` | `info` | all | `debug`, `info`, `warn` or `error` |
| `LOG_FORMAT` | `json` | all | `json` or `text` |

## API

`rwa api` serves the read-only HTTP API (contract: `internal/api/openapi.yaml`, also served at `GET /api/v1/openapi.yaml`).
It needs `DATABASE_URL` and only checks that migrations are applied (it never migrates; run `rwa migrate up` first).
Environment: `HTTP_ADDR` (default `:8080`), `CORS_ORIGINS` (exact origins, for `pnpm start` against a remote API),
`ADMIN_TOKEN` (enables the admin endpoints), `LOG_LEVEL`, `LOG_FORMAT`. `COLLECT_CRON`/`COLLECT_INTERVAL` are read only
to report `next_run_at` in `/api/v1/status`.

```bash
DATABASE_URL=postgres://rwa:rwa@127.0.0.1:5433/rwa?sslmode=disable HTTP_ADDR=127.0.0.1:8080 rwa api
curl localhost:8080/api/v1/status
```

| Endpoint | Notes |
|---|---|
| `GET /api/v1/export` | stored build bytes verbatim; `?build=<id>` for a specific build |
| `GET /api/v1/{status,summary,underlyings[/{idOrTicker}],tokens[/{id}],issuers[/{id}],opportunities}` | narrow views over the same in-memory build; `opportunities` is cursor-paginated |
| `GET /api/v1/runs`, `/runs/{id}` | run history with per-build changes |
| `GET /api/v1/history/{summary,underlyings,tokens,issuers,opportunities}[/{id}]?metrics=` | time series; `GET /api/v1/history/metrics` lists the metrics |
| `GET /logos/{kind}/{entityId}.{ext}` | logo bytes from the database |
| `GET /healthz`, `GET /readyz` | liveness; readiness (database reachable and a build loaded, else 503) |
| `POST /api/v1/admin/runs`, `GET /api/v1/admin/runs/{request_id}` | queue a manual run / read its state |

Caching: the current build is held in memory (raw bytes, a precomputed gzip copy, indexes) and re-checked every 15 s.
`/export` answers `ETag: "<export_sha256>"`, `Cache-Control: public, max-age=60, stale-while-revalidate=600` and 304 for
`If-None-Match`; `?build=<id>` is `immutable` for a year. Narrow current-build endpoints use the weak `W/"<sha>"` and the
same `Cache-Control`. `/status` is `no-store`. Other JSON bodies over 1 KB are gzipped on request. Until the first build
exists `/readyz` and the current-build endpoints return 503.

Admin: `POST /api/v1/admin/runs` with `Authorization: Bearer $ADMIN_TOKEN` and body
`{"mode":"refresh|add|rebuild","symbols":["NVDA"]}` returns `202 {"request_id": N}`; the collector picks the request up on
its next poll and `GET /api/v1/admin/runs/N` shows `taken_at`, `finished_at`, `outcome`, `error`, `run_id`. A missing or
wrong token is 401 (constant-time comparison); without `ADMIN_TOKEN` the endpoints are 404. The API writes nothing else.

Errors are `{"error":{"code","message"}}` with `not_found`, `bad_request`, `unknown_metric` (adds `metric` and
`allowed`), `unauthorized`, `unavailable`, `internal`. Every response carries `X-Request-ID` (an incoming one is kept);
the access log records method, path (no query), status, duration and the request id.

## Docker

```bash
make docker                  # local single-arch image cp0x/pi-rwa-backend:dev, not pushed
./build.sh <version>         # release: multi-arch (linux/amd64, linux/arm64) cp0x/pi-rwa-backend:<version>, pushed
```

`build.sh` refuses a dirty working tree in this folder unless `--allow-dirty` is given. One image serves both roles: the
stack runs `rwa api` and `rwa collector` as two containers (compose files and secrets: private pi-infra repository,
`stacks/rwa`). The image carries default `config/*.json` under `/config`; the stack mounts its own copies over them. The
`HEALTHCHECK` polls `/readyz` and fits only the API container. Set `stop_grace_period` of the collector to at least 40 s.

## Development database

```bash
make dev-db        # postgres:16 on 127.0.0.1:5433, user/password/db: rwa (dev only)
make dev-db-down
```

`config/assets.json` and `config/settings.json` are copies of the files in `../rwa-data-pipeline/config/`.

## Migrations

Schema changes are SQL files in `internal/db/migrations/` (`NNNN_<name>.sql`, goose format with `-- +goose Up` and
`-- +goose Down`), embedded into the binary. The migration log is append-only: never edit an existing file, add a
new one. Only `DATABASE_URL` is required.

```bash
rwa migrate up                # apply pending migrations (make migrate)
rwa migrate status            # list migrations and whether they are applied
rwa migrate down-to <version> --yes  # roll back until <version> is current (0 = everything); --yes is required
```

## Import, rebuild and the collector

```
rwa import-v1 --data-dir ../rwa-data-pipeline/data [--force]   # needs DATABASE_URL, ASSETS_PATH, SETTINGS_PATH; migrations applied
rwa build [--run <id>]                               # recompute the export from stored snapshots, no network, no history
rwa publish --run <id>                               # publish an existing collection run (no build yet) with history
rwa collector                                        # long-running scheduler
```

`import-v1` loads the 1.x pipeline data as one run (`mode=import`, `run_key` = the newest `data/runs/*.json` run id)
in a single transaction: the snapshot documents verbatim (their `fetched_at` starts the history at the 1.x
date), `issuers.json`, `rwa_info.json`, `portals_platforms.json`, `data/logos/**`, then a published build. It is
idempotent: when a run with that `run_key` exists it prints "import skipped" and exits 0. Without `data/runs/*.json` the
`run_key` is the `run_id` of the newest snapshot. It is meant for an empty database: when `collection_runs` already has
rows it refuses unless `--force` is given. It holds the collector advisory lock, so it fails while a collector run is in
progress. Logo files keep the case of their extension (`X.PNG` is stored as `PNG`); unknown extensions are stored as
`application/octet-stream`. The stored export matches the 1.x `rwa.json` structurally (`TestImportV1Parity`).

`build` creates a run with `mode=rebuild` and a new current build from the latest stored snapshot of every enabled
underlying. A rebuild is not a new observation, so it writes **no history rows**; `--run <id>` must name an existing
run and is only recorded in the new run's key (point-in-time rebuilds are not supported).

`publish --run <id>` publishes an existing collection run WITH history rows: the way to finish a `run-once --no-publish`
run, or to retry a run whose publish failed (`status=failed`, `error='publish: ...'`; the status is restored on
success). It refuses a run that already has a build, a rebuild run, a running run and a run that failed during
collection. History points are added only for underlyings refreshed by the run (and their tokens and positions); a
`--symbols` run adds points for those underlyings plus the basket (summary) and issuer aggregates. Every export carries its own `build_id` (the `builds.id` it is stored under); publishes are serialized by
a database lock, so `build_changes` always refers to the build that was current before.

`collector` runs jobs on `COLLECT_CRON` (UTC) or `COLLECT_INTERVAL`, once at start when the data are stale
(`COLLECT_ON_START`), and for every manual request it polls from `run_requests` every 30 s (its mode and symbols are
used). A job is: collect, then publish, then logos, then snapshot retention (`SNAPSHOT_RETENTION_DAYS`). Jobs never
overlap (a tick during a job is skipped with a warning); a publish failure leaves the previous build current and skips
logos and retention, and is recorded on the run (`status=failed`, `error='publish: ...'`). Each job has a deadline
(`RUN_TIMEOUT`, error `timeout`; a SIGTERM stop is `interrupted`). On SIGTERM/SIGINT the running job gets a cancelled
context and the process waits up to 30 s for it, then at most 5 s for the pool to close. **Docker `stop_grace_period`
must therefore be at least 40 s.** At start, after the migrations, runs left `running` by a crashed process are marked
`failed` with `error='abandoned'` (only when the collector lock is free). `COLLECT_ON_START` counts only `refresh` and
`import` runs that have a published build, so an `add` run over a few symbols or a run that failed to publish does not
make the basket look fresh.

A manual request (`run_requests`) always ends visibly: `outcome` is `done` (the run is linked in `run_id`; `error` holds
a post-run problem such as a failed publish), `nothing_to_do` (mode add), `rejected` (unknown symbols) or `released`:
the request goes back to the queue and is retried on a later poll (another collector run in progress, shutdown, a
database error before the run started). The scheduler claims a request only while none of its own jobs runs. Retries
of a request that keeps failing before a run starts are not capped. Underlyings a failed or interrupted run did not
reach get a `run_underlyings` row with status `skipped`; `selected_count` is the number of selected tickers.

## Probe (dev only)

`rwa probe` makes a few live calls to check the API keys and the source clients (`internal/source/{httpx,cmc,portals}`).
It needs `CMC_API_KEY`/`PORTALS_API_KEY` and `SETTINGS_PATH`, no database, and never prints keys.

```bash
ENV_FILE=../rwa-data-pipeline/.env SETTINGS_PATH=./config/settings.json go run ./cmd/rwa probe --provider cmc --symbols NVDA
ENV_FILE=../rwa-data-pipeline/.env SETTINGS_PATH=./config/settings.json go run ./cmd/rwa probe --provider portals --symbols NVDA
```

`--provider cmc` prints the resolved `rwa_id`, tokens (symbol, issuer, `crypto_id`), requests and credits
(`--type etf` for ETFs); `--provider portals` prints network/platform counts and search totals per term.
Per-request progress goes to stderr. Source-client tests run on recorded responses in `testdata/raw/`.

## Run once

`rwa run-once` performs one scheduler job without the scheduler: a collection run (port of 1.x `sync`, one snapshot
per underlying in `snapshots`), then the publish (export, current-state tables, history rows), then missing logos
(with a rebuild of the export when new logos arrived). `--no-publish` stops after the snapshots (publish later with `rwa publish --run <id>`). It needs the
collector configuration (`DATABASE_URL`, `CMC_API_KEY`, `PORTALS_API_KEY`, `ASSETS_PATH`, `SETTINGS_PATH`) and an
up-to-date schema (it fails if a migration is pending).

```bash
rwa run-once                          # refresh: every enabled underlying
rwa run-once --mode add               # only underlyings without any snapshot (nothing to do: no run row, exit 0)
rwa run-once --symbols NVDA,TSLA      # a subset (unknown or disabled symbols are an error)
rwa run-once --raw-dir /tmp/raw       # keep raw API responses in <dir>/<run_key>/ (default $RAW_CAPTURE_DIR)
rwa run-once --no-publish             # snapshots only; publish later with `rwa publish --run <id>`
```

Steps and per-underlying progress (elapsed, ETA) are logged to stderr with `log/slog`; per-request lines appear at
`LOG_LEVEL=debug`. The report goes to stdout. Exit code: `0` succeeded, `2` partial (some underlyings failed, or a
usage/config error), `1` failed (fatal error such as a 401, interruption, another run in progress). A logos problem after a successful publish is logged and does not change the exit code; a publish error exits `1` and is recorded on the run. The whole command is bounded by `RUN_TIMEOUT`. A run holds the
PostgreSQL advisory lock `hashtext('rwa-collector-run')`, so only one collector run exists at a time.

`internal/logos` (`FetchMissing`) downloads missing token, issuer and underlying logos into the `logos` table (CMC
logo, then Google favicon service for the website, then the site's `<link rel=icon>`); the scheduler calls it after
publishing.

## Integration tests

Tests under `internal/db`, `internal/store`, `internal/collect`, `internal/logos`, `internal/publish` and `internal/importer` run against a real PostgreSQL 16. Each test creates its own
throwaway database (`rwa_test_<random>`), migrates it and drops it afterwards. They are skipped when
`TEST_DATABASE_URL` is unset, so `make test` works without a database.

```bash
make dev-db
export TEST_DATABASE_URL='postgres://rwa:rwa@127.0.0.1:5433/rwa?sslmode=disable'
make test                # runs everything, integration tests included
make test-integration    # only db + store; fails if TEST_DATABASE_URL is unset
```

The URL must point at a database the user may `CREATE DATABASE` in (the dev database does).

Model tests that read 1.x files (`../rwa-interface/public/data/rwa.json`, `../rwa-data-pipeline/data/snapshots/`)
are skipped when those files are absent.
