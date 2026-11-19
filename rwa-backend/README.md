# rwa-backend

Go service for RWA Explorer 2.0: a collector (CoinMarketCap + Portals) and an HTTP API over PostgreSQL.
One binary, `rwa`, with subcommands `api`, `collector`, `migrate`, `import-v1`, `run-once`, `build`
(currently stubs; they exit with code 2).

## Build and test

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
