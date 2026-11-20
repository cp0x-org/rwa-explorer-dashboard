# Local stack shortcuts (see "Run everything with Docker" in README.md). Needs .env (cp .env.example .env).
WEB_PORT ?= 3000
API_PORT ?= 8080

.PHONY: up logs run down reset psql

up:
	docker compose up --build -d
	@echo "Site:   http://localhost:$(WEB_PORT)"
	@echo "Status: http://localhost:$(API_PORT)/api/v1/status"
	@echo "Follow the collection: make logs"

logs:
	docker compose logs -f collector api web

# one more live run now; a subset: docker compose exec collector rwa run-once --symbols NVDA
run:
	docker compose exec collector rwa run-once

down:
	docker compose down

# deletes the database volume
reset:
	@printf 'Delete the database volume (docker compose down -v)? [y/N] ' && read a && [ "$$a" = y ]
	docker compose down -v

psql:
	docker compose exec postgres psql -U rwa rwa
