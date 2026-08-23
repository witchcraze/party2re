.PHONY: all check verify fmt vet test smoke setup-hooks

all: check

fmt:
	@echo "Formatting Go files..."
	@gofmt -w $$(find . -name "*.go" -not -path "./vendor/*")

vet:
	@echo "Running static analysis..."
	@go vet ./...

test:
	@echo "Running tests in Docker..."
	@docker compose run --rm app go test -count=1 ./...

smoke:
	@echo "Building smoke production image..."
	@docker build -f Dockerfile -t party2re:smoke .

check verify:
	@./scripts/verify.sh

setup-hooks:
	@echo "Configuring Git hooks path..."
	@git config core.hooksPath .githooks
	@chmod +x .githooks/*
	@echo "Git hooks configured successfully (.githooks)."
