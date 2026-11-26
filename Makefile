# Local stack shortcuts (see "Run everything with Docker" in README.md). Needs .env (cp .env.example .env).
# Ports and paths come from .env when set there (the same values docker compose uses).
-include .env
WEB_PORT ?= 3301
API_PORT ?= 3302
PGDATA_DIR ?= ./pg-data
COLLECT_INTERVAL ?= 24h
COLLECT_ON_START ?= true

# local build from this checkout (docker-compose.build.yml); the server only pulls images (make deploy)
BUILD_COMPOSE = docker compose -f docker-compose.yml -f docker-compose.build.yml

.PHONY: up deploy info logs run down reset psql

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
	docker compose exec postgres psql -U rwa rwa
