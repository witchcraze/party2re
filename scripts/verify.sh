#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

echo "==> [1/6] Checking and applying code formatting (gofmt)..."
GO_FILES=$(find . -name "*.go" -not -path "./vendor/*")
if [ -n "$GO_FILES" ]; then
    UNFORMATTED=$(gofmt -l $GO_FILES)
    if [ -n "$UNFORMATTED" ]; then
        echo "Auto-formatting files:"
        echo "$UNFORMATTED"
        gofmt -w $GO_FILES
    else
        echo "All Go files are properly formatted."
    fi
fi

echo "==> [2/6] Running static analysis (go vet)..."
if command -v go >/dev/null 2>&1; then
    go vet ./...
else
    docker compose run --rm app go vet ./...
fi

echo "==> [3/6] Validating OpenAPI 3.1 specification and route coverage..."
if command -v go >/dev/null 2>&1; then
    go test -count=1 ./internal/api/http -run "OpenAPI"
else
    docker compose run --rm app go test -count=1 ./internal/api/http -run "OpenAPI"
fi

echo "==> [4/6] Ensuring database migrations are applied..."
"${ROOT_DIR}/scripts/migrate.sh"

echo "==> [5/6] Running full test suite..."
if command -v go >/dev/null 2>&1; then
    PARTY2_DB_DSN="party2:party2@tcp(127.0.0.1:3306)/party2?parseTime=true" \
    PARTY2_VALKEY_ADDR="127.0.0.1:6379" \
    go test ./...
else
    docker compose run --rm app go test ./...
fi

echo "==> [6/6] Running smoke image build..."
docker build -f Dockerfile -t party2re:smoke .

echo "==> ALL VERIFICATION CHECKS PASSED!"
