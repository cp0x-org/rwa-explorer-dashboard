# Run RWA Explorer locally (with the real backend)

Two ways, both with the real Go backend and PostgreSQL. They use different ports and databases, so they can run side by side.

| Mode | When | What runs where |
|------|------|-----------------|
| **A. Local** | coding, fast reload | PostgreSQL in Docker (:5433); `rwa api` (:8080) and the site (:3000 / :4173) on your machine |
| **B. Docker** | before a deploy | everything in containers, built from this checkout: site :3301, API :3303 |

> **Mock vs. backend.** `pnpm mock:api` and `pnpm mock:ssr` serve a *frozen* 1.x snapshot (`public/data/rwa.json`,
> collected 2026-10-18) on port 8080. They never show new data. Stop them before starting `rwa api` (same port 8080).

**Which data am I looking at?** The site footer says "Data collected …". The API tells the build and its date:

```bash
curl -s localhost:8080/api/v1/status | jq '.current_build | {id, generated_at}, .last_run | {id, mode, status, finished_at}'   # A
curl -s localhost:3303/api/v1/status | jq '.current_build | {id, generated_at}, .last_run | {id, mode, status, finished_at}'   # B
```

An empty database has no build: the site has no data until the first collection run (`run-once` or the collector) is
published.

---

## A. Local

Prerequisites: Docker, Go 1.26, Node 22+ with pnpm (`corepack enable`), the CMC and Portals API keys.

**1. Backend config (once)**

```bash
cd rwa-backend
cp .env.example .env          # fill CMC_API_KEY and PORTALS_API_KEY (Portals key without "Bearer ")
```

**2. Database and API** (terminal 1)

```bash
cd rwa-backend
make dev-db                   # PostgreSQL 16 on 127.0.0.1:5433 (rwa/rwa/rwa), data kept in a Docker volume
make migrate                  # apply migrations (safe to repeat)
go run ./cmd/rwa api          # API on http://127.0.0.1:8080, reads ./.env
```

On an empty database the API answers `/readyz` with 503 and the site has no data until step 4 publishes the first build.

**3. Site** (terminal 2), one of:

```bash
cd rwa-interface && pnpm install
pnpm start                    # dev server with hot reload: http://localhost:3000 (next free port if busy); /api → :8080
pnpm dev:ssr                  # production-like server rendering: http://localhost:4173 (PORT=4174 pnpm dev:ssr if busy)
```

**4. Collect fresh data** (terminal 3) and watch it on `/updates` → the newest run → *Journal*

```bash
cd rwa-backend
go run ./cmd/rwa run-once --symbols NVDA   # ~40 s, ~11 CMC credits: one asset, then publish and logos
go run ./cmd/rwa run-once                  # all 125 assets: ~7 min, ~22 CMC credits
```

The site picks up the new build by itself within a minute (or reload). Instead of `run-once` you can run the scheduler,
`go run ./cmd/rwa collector` (with `COLLECT_ON_START=false` in `.env` it waits for the schedule).

**Stop / reset**

```bash
Ctrl-C in each terminal
cd rwa-backend && make dev-db-down                                 # stop PostgreSQL, keep the data
docker compose -f docker-compose.dev.yml down -v                   # delete the dev database completely
```

---

## B. Docker (everything in containers)

Mode A may keep running: Docker uses its own database and the ports 3301 / 3303.

```bash
cp .env.example .env          # in the REPOSITORY ROOT (not rwa-backend/.env.example): fill the two keys
make up                       # builds this checkout and starts it (first build takes a few minutes)
```

What happens, in order: `postgres` → `migrate` → `api` and `web` → `collector` starts a **live collection at once**
because the database has no successful run yet (~22 CMC credits, 10–15 minutes in Docker). The site shows the data
when that run is published; until then it has no data. After that the collector runs again every `COLLECT_INTERVAL`
(default `24h`). Set `COLLECT_ON_START=false` in `.env` to skip the run on start.

| Look at | URL / command |
|---------|---------------|
| site | http://localhost:3301 (or the `WEB_PORT` you set) |
| live run and its journal | `/updates` → the newest run |
| API status | `curl -s localhost:3303/api/v1/status` (or the `API_PORT` you set) |
| collector logs | `make logs` or `docker compose logs -f collector` |
| another run | `make run` (all assets) or `docker compose exec collector rwa run-once --symbols NVDA` |
| database shell | `make psql` |

**Stop / reset**: `make down` (the database stays in `./pg-data`, or `PGDATA_DIR` from `.env`), `make reset` (asks, then deletes that folder: the next start collects from scratch).

`make up` is `docker compose -f docker-compose.yml -f docker-compose.build.yml up --build -d`: the build part is only in
`docker-compose.build.yml`. A server runs the same `docker-compose.yml` from prebuilt images with `make deploy` (see the
README, "Deployment").

---

## Troubleshooting

| Symptom | Cause and fix |
|---------|---------------|
| site shows October data | you run the mock (`mock:api`/`mock:ssr`), or no collection ran yet: check `/api/v1/status`, then `run-once` |
| `rwa api`: `address already in use` | the mock holds 8080: stop it (`lsof -iTCP:8080 -sTCP:LISTEN`) |
| Docker site does not open, `web` logs `export refresh failed: fetch failed` | the `web` container started without its network (a host port was busy at start): `docker compose up -d --force-recreate web` |
| `rwa api`: pending migration | `make migrate` |
| `run-once`: `CMC_API_KEY ... required` | keys missing in `rwa-backend/.env` (mode A) or root `.env` (mode B) |
| `run-once`: `another collector run is in progress` | a collector (or another run-once) is collecting; wait for it, see `/updates` |
| `pnpm dev:ssr`: "Data unavailable" | `rwa api` is not running on 8080 (or set `API_URL=http://127.0.0.1:<port>`) |
