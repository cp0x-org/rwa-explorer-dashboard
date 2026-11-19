VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
GO ?= go

.PHONY: build test lint dev-db dev-db-down migrate parity

build:
	$(GO) build -ldflags "-X main.version=$(VERSION)" -o bin/rwa ./cmd/rwa

test:
	$(GO) test ./...

lint:
	@if command -v golangci-lint >/dev/null 2>&1; then golangci-lint run; \
	elif [ "$$CI" = "true" ]; then echo "golangci-lint is required in CI" >&2; exit 1; \
	else echo "golangci-lint not installed (https://golangci-lint.run/welcome/install/); running go vet only"; $(GO) vet ./...; fi

dev-db:
	docker compose -f docker-compose.dev.yml up -d

dev-db-down:
	docker compose -f docker-compose.dev.yml down

migrate:
	@echo "not implemented in phase 0"

parity:
	@echo "not implemented in phase 0"
