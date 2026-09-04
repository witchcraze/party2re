.PHONY: all check verify check-clean fmt vet openapi-sync openapi-check openapi-scaffold test test-integration test-docker test-stress smoke db-migrate db-reset up down setup-hooks arch-lint lock-lint

all: check

fmt:
	@echo "Formatting Go files..."
	@gofmt -w $$(find . -name "*.go" -not -path "./vendor/*")
	@echo "Synchronizing and formatting OpenAPI specification..."
	@go run ./scripts/sync_openapi.go

vet:
	@echo "Running static analysis..."
	@go vet ./...

openapi-sync:
	@echo "Synchronizing and formatting OpenAPI specification..."
	@go run ./scripts/sync_openapi.go

openapi-check:
	@echo "Validating OpenAPI specification synchronization and route coverage..."
	@go run ./scripts/sync_openapi.go --check
	@go test -count=1 ./internal/api/http -run "OpenAPI"

openapi-scaffold:
	@echo "Scaffolding missing routes and synchronizing OpenAPI specification..."
	@go run ./scripts/sync_openapi.go --scaffold

up:
	@echo "Starting database and cache services..."
	@docker compose up -d mariadb valkey

down:
	@echo "Stopping services..."
	@docker compose down

db-migrate:
	@./scripts/migrate.sh

db-reset:
	@./scripts/reset_db.sh

test:
	@echo "Running unit tests (skips integration tests without DB)..."
	@go test -count=1 ./...

test-integration:
	@echo "Running integration tests against local container DB..."
	@PARTY2_DB_DSN="party2:party2@tcp(127.0.0.1:3306)/party2?parseTime=true" PARTY2_VALKEY_ADDR="127.0.0.1:6379" go test -count=1 ./...

test-docker:
	@echo "Running tests in Docker..."
	@docker compose run --rm app go test -count=1 ./...

test-stress:
	@echo "Running high-concurrency stress test..."
	@./scripts/stress_test.sh

smoke:
	@echo "Building smoke production image..."
	@docker build -f Dockerfile -t party2re:smoke .

check verify:
	@./scripts/verify.sh

check-clean:
	@./scripts/reset_db.sh
	@./scripts/verify.sh

setup-hooks:
	@echo "Configuring Git hooks path..."
	@git config core.hooksPath .githooks
	@chmod +x .githooks/*
	@echo "Git hooks configured successfully (.githooks)."

arch-lint:
	@echo "Validating .arch module guidance symbols against Go AST..."
	@go test -v ./internal/architecture

lock-lint:
	@echo "Validating deterministic database row-lock acquisition hierarchy via Go AST..."
	@go test -v -count=1 ./internal/database -run "LockHierarchy"

