VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
GO ?= go

.PHONY: docker build test test-integration lint dev-db dev-db-down migrate parity

build:
	$(GO) build -ldflags "-X main.version=$(VERSION)" -o bin/rwa ./cmd/rwa

test:
	$(GO) test ./...

# Integration tests need PostgreSQL (make dev-db). Plain `make test` skips them when TEST_DATABASE_URL is unset.
test-integration:
	@test -n "$$TEST_DATABASE_URL" || { echo "TEST_DATABASE_URL is required, e.g. postgres://rwa:rwa@127.0.0.1:5433/rwa?sslmode=disable" >&2; exit 1; }
	$(GO) test -count=1 ./internal/db/... ./internal/store/...

lint:
	@if command -v golangci-lint >/dev/null 2>&1; then golangci-lint run; \
	elif [ "$$CI" = "true" ]; then echo "golangci-lint is required in CI" >&2; exit 1; \
	else echo "golangci-lint not installed (https://golangci-lint.run/welcome/install/); running go vet only"; $(GO) vet ./...; fi

dev-db:
	docker compose -f docker-compose.dev.yml up -d

dev-db-down:
	docker compose -f docker-compose.dev.yml down

migrate:
	$(GO) run ./cmd/rwa migrate up

# Compares the Go compute with the 1.x export over the 1.x data (rwa-data-pipeline/data, or RWA_V1_DATA_DIR).
parity:
	$(GO) test -tags parity -count=1 -run TestParity ./internal/compute/ -v

# Local single-arch image, not pushed (release images: ./build.sh <version>).
docker:
	docker build --build-arg VERSION=$(VERSION) -t cp0x/pi-rwa-backend:dev .
