# rwa-backend

Go service for RWA Explorer 2.0: a collector (CoinMarketCap + Portals) and an HTTP API over PostgreSQL.
One binary, `rwa`, with subcommands `api`, `collector`, `migrate`, `import-v1`, `run-once`, `build`
(`migrate` works; the others are stubs that exit with code 2).

## Build and test

Requires Go 1.26 (`go.mod` pins `toolchain go1.26.4`).

```bash
cd rwa-backend
make build        # bin/rwa, version from git describe
make test
make lint         # golangci-lint if installed, otherwise go vet
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
| `COLLECT_CRON` | `0 6 * * *` | collector | cron schedule (UTC) |
| `COLLECT_INTERVAL` | empty | collector | alternative to cron, e.g. `12h`; setting both is an error |
| `COLLECT_ON_START` | `true` | collector | run on start if the last run is stale |
| `SNAPSHOT_RETENTION_DAYS` | `90` | collector | snapshot retention |
| `RAW_CAPTURE_DIR` | empty | collector | write raw API responses here (debug) |
| `HTTP_ADDR` | `:8080` | api | listen address |
| `CORS_ORIGINS` | empty | api | comma-separated allowed origins (dev) |
| `ADMIN_TOKEN` | empty | api | enables the admin endpoints when set |
| `ENV_FILE` | `.env` | all | path of the dotenv file to load |
| `LOG_LEVEL` | `info` | all | `debug`, `info`, `warn` or `error` |
| `LOG_FORMAT` | `json` | all | `json` or `text` |

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

## Integration tests

Tests under `internal/db` and `internal/store` run against a real PostgreSQL 16. Each test creates its own
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
