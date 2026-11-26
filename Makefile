# Local stack shortcuts (see "Run everything with Docker" in README.md). Needs .env (cp .env.example .env).
# Ports and paths come from .env when set there (the same values docker compose uses).
-include .env
SHELL := /bin/bash
.SHELLFLAGS := -eo pipefail -c
WEB_PORT ?= 3301
API_PORT ?= 3302
PGDATA_DIR ?= ./pg-data
COLLECT_INTERVAL ?= 24h
COLLECT_ON_START ?= true

# local build from this checkout (docker-compose.build.yml); the server only pulls images (make deploy)
BUILD_COMPOSE = docker compose -f docker-compose.yml -f docker-compose.build.yml

.PHONY: up deploy info logs run down reset psql db-dump db-restore

up:
	$(BUILD_COMPOSE) up --build -d
	@$(MAKE) --no-print-directory info

# on the server: pull cp0x/pi-rwa-backend and cp0x/pi-rwa-interface (tag RWA_VERSION from .env, default latest), restart
deploy:
	docker compose pull
	docker compose up -d
	@$(MAKE) --no-print-directory info

# where everything is: host ports, containers, database; each service prints its own banner at the top of its log
info:
	@echo "RWA Explorer, local stack (docker compose project rwa-explorer)"
	@echo "Containers: cp0x-pi-rwa-interface, cp0x-pi-rwa-backend-{api,collector,migrate}, cp0x-pi-rwa-postgres"
	@printf "  %-10s %-34s %s\n" "web" "http://localhost:$(WEB_PORT)" "Node SSR site, container :4173 -> api:8080"
	@printf "  %-10s %-34s %s\n" "api" "http://localhost:$(API_PORT)" "Go API, container :8080; /api/v1/status, /api/v1/openapi.yaml"
	@printf "  %-10s %-34s %s\n" "postgres" "127.0.0.1:5433 (rwa/rwa, db rwa)" "container postgres:5432; files in $(PGDATA_DIR); make psql"
	@printf "  %-10s %-34s %s\n" "collector" "no port" "every $(COLLECT_INTERVAL), on start: $(COLLECT_ON_START); CMC + Portals -> postgres"
	@printf "  %-10s %-34s %s\n" "migrate" "one-shot" "applies the schema, then exits"
	@echo "Details of one service (ports, connections, tables, config): docker compose logs <service> | head -40"
	@echo "Follow the collection: make logs"

logs:
	docker compose logs -f collector api web

# one more live run now; a subset: docker compose exec collector rwa run-once --symbols NVDA
run:
	docker compose exec collector rwa run-once

down:
	docker compose down

# stops the stack and deletes the database folder ($(PGDATA_DIR)); the next `make up` migrates and collects again
reset:
	@printf 'Stop the stack and delete the database folder $(PGDATA_DIR)? [y/N] ' && read a && [ "$$a" = y ]
	docker compose down
	rm -rf "$(PGDATA_DIR)"

psql:
	docker compose exec postgres sh -c 'psql -U "$$POSTGRES_USER" "$$POSTGRES_DB"'

# dump of the stack's database (schema, data, goose version) into backups/rwa-<UTC time>.sql.gz; no owners or grants,
# so it restores under any POSTGRES_USER / POSTGRES_PASSWORD. Safe while the stack runs (pg_dump reads one snapshot).
db-dump:
	@mkdir -p backups
	@f=backups/rwa-$$(date -u +%Y%m%dT%H%M%SZ).sql.gz; \
	docker compose exec -T postgres sh -c 'pg_dump -U "$$POSTGRES_USER" -d "$$POSTGRES_DB" --no-owner --no-privileges' \
	  | gzip -9 > "$$f.part"; \
	mv "$$f.part" "$$f"; ls -lh "$$f"

# restore a dump into an EMPTY database before the first `make deploy`: migrate then finds the schema up to date and
# the collector continues from the last run. Usage: make db-restore FILE=backups/rwa-....sql.gz
db-restore:
	@test -n "$(FILE)" && test -f "$(FILE)" || { echo "usage: make db-restore FILE=backups/rwa-....sql.gz" >&2; exit 1; }
	docker compose up -d --wait postgres
	@n=$$(docker compose exec -T postgres sh -c \
	  'psql -U "$$POSTGRES_USER" -d "$$POSTGRES_DB" -Atc "select count(*) from pg_tables where schemaname = current_schema()"'); \
	if [ "$$n" != 0 ]; then echo "the database already has $$n tables: restore only into an empty one" >&2; exit 1; fi
	gunzip -c "$(FILE)" | docker compose exec -T postgres sh -c \
	  'psql -v ON_ERROR_STOP=1 -q --single-transaction -U "$$POSTGRES_USER" -d "$$POSTGRES_DB"' > /dev/null
	@echo "restored $(FILE); next: make deploy"
