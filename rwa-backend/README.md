# rwa-backend

Go service for RWA Explorer 2.0: a collector (CoinMarketCap + Portals) and an HTTP API over PostgreSQL.
One binary, `rwa`, with subcommands `api`, `collector`, `migrate`, `run-once`, `build`, `publish`, `deals`,
`probe` and `version`. All subcommands are implemented. Part of the root [README](../README.md); the 2.0 flow is in [docs/workflow.md](../docs/workflow.md).

## Build and test

Requires Go 1.26 (`go.mod` pins `toolchain go1.26.4`).

```bash
cd rwa-backend
make build        # bin/rwa, version from git describe
make test
make lint         # golangci-lint (config v2, .golangci.yml) if installed, otherwise go vet
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
| `SETTINGS_PATH` | `/config/settings.json` | collector | rating settings; its directory is also where the curated config files below are read from |
| `COLLECT_CRON` | `0 6 * * *` | collector | cron schedule, always UTC; a `CRON_TZ=`/`TZ=` prefix is rejected, as is a schedule that never fires |
| `COLLECT_INTERVAL` | empty | collector | alternative to cron, e.g. `12h`; setting both is an error |
| `COLLECT_ON_START` | `true` | collector | run on start if the last successful run is older than the interval (cron: older than 24 h) |
| `MIGRATE_ON_START` | `true` | collector | apply pending migrations when the collector starts; when `false` it refuses to start with pending ones |
| `SNAPSHOT_RETENTION_DAYS` | `90` | collector | snapshot retention |
| `RUN_EVENTS_LEVEL` | `info` | collector, `run-once`, `build`, `publish` | minimum level stored in the run journal (`run_events`): `debug` also stores every provider request |
| `RUN_EVENTS_RETENTION_DAYS` | `30` | collector | run journal retention (the events of the current build's run are kept) |
| `RUN_TIMEOUT` | `2h` | collector | deadline of one job (collect + publish + logos) and of `run-once`; a run that exceeds it ends `failed` with `error='timeout'` |
| `RAW_CAPTURE_DIR` | empty | collector | write raw API responses here (debug) |
| `HTTP_ADDR` | `:8080` | api | listen address |
| `CORS_ORIGINS` | empty | api | comma-separated allowed origins (dev) |
| `ADMIN_TOKEN` | empty | api | enables the admin endpoints when set |
| `ENV_FILE` | `.env` | all | path of the dotenv file to load |
| `LOG_LEVEL` | `info` | all | `debug`, `info`, `warn` or `error` |
| `LOG_FORMAT` | `json` | all | `json` or `text` |

### Config files

All files live in `config/` (Docker: `/config/`); the curated ones are read from the directory of `SETTINGS_PATH`. They are the
only copy: edit them here.

| File | Required | Purpose |
|---|---|---|
| `assets.json` | yes | underlyings; `symbol_aliases` lets a token whose symbol lacks the ticker (SPCX: `SPACEX`) count as a version |
| `settings.json` | yes | rating settings, rate limits, `chain_mapping` (Mantle and Monad are known networks Portals does not index) |
| `issuers.json` | no | issuer website used when CMC has none (`schema_version` 1, `issuers.<id>.website`, clean HTTPS) |
| `links.json` | no | outbound links: position routes per platform and network (each with a `verified` record), issuer apps, swap links, cp0x referrals (`deal` slugs, `deals_max_age_days`, default 14) |
| `rights_profiles.json` | no | reviewed holder rights per token id (`review_valid_days`, default 180); a profile that fails validation is excluded with a warning, the rest is published |

A missing optional file turns its feature off (no links, no profiles, no website overrides); an invalid file is a config
error and the command exits `2`. Warnings of links and rights (an unusable URL, an excluded or stale profile, an
unavailable referral) are appended to the run and to the run journal; they never fail a publish.

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
| `GET /api/v1/runs`, `/runs/{id}` | run history with per-build changes; the detail has `events_count` and `errors_count` |
| `GET /api/v1/runs/{id}/events` | the run journal, see [Run journal](#run-journal) |
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

## Run journal

Every collection run (and every rebuild) keeps a journal in the `run_events` table: the five global collection steps, one
line per underlying (saved or failed, with its duration), warnings, the publish (start, end, summary, warnings), the logos
report, retention and a `run finished` event with the status and duration. `run finished` is not the last event: the
run is finished before publish, logos, retention and `job ended` write into it, so the job that owns the run ends the
journal with a terminal event `journal closed` (`step=run`, written by the scheduler, `rwa run-once`, `rwa publish --run`
and the rebuild itself). Provider requests (provider, path,
status, `duration_ms`) are stored only with `RUN_EVENTS_LEVEL=debug`; so are the names of the downloaded, failed and
source-less logos. `LOG_LEVEL` and `RUN_EVENTS_LEVEL` are independent: the process log (stdout) is unchanged, the journal
is an additional copy of the records that belong to a run.

How it works: `internal/runlog` is a `slog.Handler` that forwards every record to the stdout handler and, when the record
carries a run id (the attribute `run_id`, or a context set with `runlog.WithRun`), buffers it as an event. The attribute
`step` names the stage: `collect.cmc`, `collect.portals`, `collect.underlying`, `publish`, `logos`, `retention`,
`scheduler` or `run` (default). Other attributes become the JSONB `attrs` (durations as `<name>_ms`); text values whose
name contains `key`, `token`, `password` or `authorization` are stored as `[redacted]`. Events are written in batches
(200 events or every 2 s, and when a run, publish or job ends) through the pool, never inside the run's own transaction. A
failed write is logged to stdout once per flush, the events are kept for the next flush (at most 30 attempts, about a
minute, then the batch is dropped with one stdout error and one warning event per run), and the run is never affected.
A row the database rejects for its content (SQLSTATE class 22/23) is dropped on its own after the rest of the batch is
stored row by row; NUL characters are removed from messages and attributes, NaN/Inf numbers are stored as text. A run
buffers at most 10 000 unwritten events (the oldest are dropped, with one warning event). Scheduler messages before
the run row exists (`job started`, a skipped run) have no run id and go to stdout only.

Read it with `GET /api/v1/runs/{id}/events?level=info&after=<id>&limit=500` (`level` is the minimum: `debug` < `info` <
`warn` < `error`; `limit` 1..2000). The response is `{"run_id", "run_status", "journal_open", "items": [{"id", "ts", "level", "step",
"message", "attrs"}], "next_after"}`; pass `next_after` as `after` for the next page, or poll with the last seen `id` while
`journal_open` is true. `journal_open` is true while the run is `running` or its terminal `journal closed` event does not
exist yet (a finished run with no event for 15 minutes counts as closed, for runs of older versions and crashed
writers); clients must poll while `journal_open`, not while `run_status` is `running`, or they miss publish, logos and
retention. Such a response is `no-store`, a closed journal's is cached for 60 s. `GET /api/v1/runs/{id}` carries the same
`journal_open` and is `no-store` while it is true (`max-age=30` otherwise). The scheduler deletes the events of runs that
started before the `RUN_EVENTS_RETENTION_DAYS` cutoff, whole runs only, together with the snapshot retention; the events
of the 3 newest runs and of the run behind the current build are always kept. `rwa run-once`, `rwa build` and
`rwa publish --run` journal into their run too; their events are written by the 2 s flusher and at the latest when the
command ends.

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

## Migrations

Schema changes are SQL files in `internal/db/migrations/` (`NNNN_<name>.sql`, goose format with `-- +goose Up` and
`-- +goose Down`), embedded into the binary. The migration log is append-only: never edit an existing file, add a
new one. Only `DATABASE_URL` is required.

```bash
rwa migrate up                # apply pending migrations (make migrate)
rwa migrate status            # list migrations and whether they are applied
rwa migrate down-to <version> --yes  # roll back until <version> is current (0 = everything); --yes is required
```

### Tables

The prefix says what a table is for (schema: `internal/db/migrations/0001_init.sql`).

| Group | Tables | What they hold |
|---|---|---|
| `run*`: the collection process | `runs`, `run_underlyings`, `run_requests`, `run_events` | one row per collection run; its per-underlying results; the queue of manual run requests; the run journal |
| `raw_*`: source responses as fetched | `raw_snapshots`, `raw_cp0x_deals` | the CMC + Portals document per underlying and run (input of every build); the cp0x deals cache |
| `catalog_*`: every entity ever seen | `catalog_underlyings`, `catalog_issuers`, `catalog_tokens`, `catalog_deployments`, `catalog_positions`, `catalog_token_positions`, `catalog_platforms`, `catalog_networks`, `catalog_logos` | latest known facts with `active` and `first/last_seen_run_id`; history rows reference them; logos are served from `catalog_logos` |
| `history_*`: the charts | `history_summary`, `history_underlyings`, `history_tokens`, `history_issuers`, `history_positions` | one row per entity and run (`observed_at` + numeric metrics) |
| `build_*`: what the site shows | `build_exports`, `build_changes` | the export document of every build (`is_current` is the one the API serves); what changed against the previous build |

Configuration is not in the database: it lives in `config/*.json`. Runs and builds keep a copy of the settings they
used (`runs.settings`, `build_exports.settings`), and `catalog_underlyings.enabled` mirrors `assets.json`.

## Rebuild and the collector

```
rwa build [--run <id>]                               # recompute the export from stored snapshots, no network, no history
rwa deals [--no-build]                               # fetch the cp0x deals now, store them, then rebuild
rwa publish --run <id>                               # publish an existing collection run (no build yet) with history
rwa collector                                        # long-running scheduler
```

`deals` fetches `https://cp0x.com/api/v1/deals`, keeps only the deals `links.json` names and stores them as the newest
row of `raw_cp0x_deals` (`id`, `fetched_at`, `payload` = `{fetched_at, deals}`). Publish
reads the newest row; referrals appear only while it is at most `deals_max_age_days` old (and not more than an hour in
the future). A failed fetch keeps the previous row and exits `1` without a rebuild. The collector does the same fetch once
per run after the Portals step (`step=collect.deals`); there a failure is a run warning, never fatal.

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
context and the process waits up to 30 s for it, then at most 5 s for the pool to close, and at most 5 s to flush the run journal. **Docker
`stop_grace_period` must therefore be at least 40 s; the compose value of 45 s covers 30 s scheduler + 5 s journal + 5 s
pool.** At start, after the migrations, runs left `running` by a crashed process are marked
`failed` with `error='abandoned'` (only when the collector lock is free). `COLLECT_ON_START` counts only `refresh` and
`import` runs (the latter exist only in databases filled by the removed 1.x import) that have a published build, so an `add` run over a few symbols or a run that failed to publish does not
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
SETTINGS_PATH=./config/settings.json go run ./cmd/rwa probe --provider cmc --symbols NVDA
SETTINGS_PATH=./config/settings.json go run ./cmd/rwa probe --provider portals --symbols NVDA
```

`--provider cmc` prints the resolved `rwa_id`, tokens (symbol, issuer, `crypto_id`), requests and credits
(`--type etf` for ETFs); `--provider portals` prints network/platform counts and search totals per term.
Per-request progress goes to stderr. Source-client tests run on recorded responses in `testdata/raw/`.

## Run once

`rwa run-once` performs one scheduler job without the scheduler: a collection run (port of 1.x `sync`, one snapshot
per underlying in `raw_snapshots`), then the publish (export, `catalog_*` tables, history rows), then missing logos
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
usage/config error), `1` failed (fatal error such as a 401, interruption, another run in progress, or every selected underlying failed).
A fresh snapshot without tokens never replaces one that had tokens: the underlying fails with
`<status>: previous snapshot with N tokens kept` and the old snapshot stays. The Portals clients are strict: a 2xx
answer that is not JSON, a CMC answer without a `status` object or a `data` key, an empty `/v1/networks` list and an
incomplete exact lookup (fewer records than reported, or the page cap reached) are errors, not empty data. A logos problem after a successful publish is logged and does not change the exit code; a publish error exits `1` and is recorded on the run. The whole command is bounded by `RUN_TIMEOUT`. A run holds the
PostgreSQL advisory lock `hashtext('rwa-collector-run')`, so only one collector run exists at a time.

`internal/logos` (`FetchMissing`) downloads missing token, issuer and underlying logos into the `catalog_logos` table (CMC
logo, then Google favicon service for the website, then the site's `<link rel=icon>`); the scheduler calls it after
publishing.

## Integration tests

Tests under `internal/db`, `internal/store`, `internal/collect`, `internal/logos`, and `internal/publish` run against a real PostgreSQL 16. Each test creates its own
throwaway database (`rwa_test_<random>`), migrates it and drops it afterwards. They are skipped when
`TEST_DATABASE_URL` is unset, so `make test` works without a database.

```bash
make dev-db
export TEST_DATABASE_URL='postgres://rwa:rwa@127.0.0.1:5433/rwa?sslmode=disable'
make test                # runs everything, integration tests included
make test-integration    # only db + store; fails if TEST_DATABASE_URL is unset
```

The URL must point at a database the user may `CREATE DATABASE` in (the dev database does).

Tests that need 1.x documents read `testdata/golden/rwa.json` (a 1.x export) and `testdata/snapshots/v1/` (four 1.x
snapshots: NVDA, AAPL, TSLA, IWM). The model round-trip of `../rwa-interface/public/data/rwa.json` is skipped when that
file is absent.
