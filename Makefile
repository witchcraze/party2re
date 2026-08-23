.PHONY: all check verify check-clean fmt vet test smoke db-migrate db-reset up down setup-hooks

all: check

fmt:
	@echo "Formatting Go files..."
	@gofmt -w $$(find . -name "*.go" -not -path "./vendor/*")

vet:
	@echo "Running static analysis..."
	@go vet ./...

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
	@echo "Running tests in Docker..."
	@docker compose run --rm app go test -count=1 ./...

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
