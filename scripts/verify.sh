#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

STAMP_FILE="${ROOT_DIR}/.cache/verify_tree_stamp"
FORCE_VERIFY="${FORCE_VERIFY:-0}"

get_tree_hash() {
    if git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
        local tmp_idx
        tmp_idx=$(mktemp -u)
        GIT_INDEX_FILE="$tmp_idx" git add -A
        GIT_INDEX_FILE="$tmp_idx" git write-tree
        rm -f "$tmp_idx"
    else
        echo "none"
    fi
}

CURRENT_TREE=$(get_tree_hash)
if [ "$FORCE_VERIFY" = "0" ] && [ -f "$STAMP_FILE" ] && [ "$CURRENT_TREE" != "none" ]; then
    CACHED_TREE=$(cat "$STAMP_FILE" 2>/dev/null || echo "")
    if [ "$CURRENT_TREE" = "$CACHED_TREE" ]; then
        echo "==> [verify.sh] Tree ${CURRENT_TREE:0:8} already verified cleanly. Skipping duplicate verification (set FORCE_VERIFY=1 to force)."
        echo "==> ALL VERIFICATION CHECKS PASSED!"
        exit 0
    fi
fi

echo "==> [1/7] Checking and applying code formatting (gofmt & openapi-sync)..."
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
if command -v go >/dev/null 2>&1; then
    go run ./scripts/sync_openapi
fi

CHECKED_TREE=$(get_tree_hash)

echo "==> [2/7] Running static analysis (go vet)..."
if command -v go >/dev/null 2>&1; then
    go vet ./...
else
    docker compose run --rm app go vet ./...
fi

echo "==> [3/7] Validating OpenAPI 3.1 specification and route coverage..."
if command -v go >/dev/null 2>&1; then
    go run ./scripts/sync_openapi --check
    go test -count=1 ./internal/api/http -run "OpenAPI"
else
    docker compose run --rm app go run ./scripts/sync_openapi --check
    docker compose run --rm app go test -count=1 ./internal/api/http -run "OpenAPI"
fi

echo "==> [4/7] Validating .arch Guidance Layer symbols and database row-lock hierarchy via Go AST..."
if command -v go >/dev/null 2>&1; then
    go test -count=1 ./internal/architecture
else
    docker compose run --rm app go test -count=1 ./internal/architecture
fi



echo "==> [5/7] Ensuring database migrations are applied..."
. "${ROOT_DIR}/scripts/test_services.sh"
"${ROOT_DIR}/scripts/migrate.sh"

resolve_timeout_arg() {
    local timeout="${TEST_TIMEOUT:-}"
    if [ -n "$timeout" ]; then
        echo "-timeout=$timeout"
        return
    fi
    if [ -n "${PARTY2_TEST_TIMEOUT:-}" ]; then
        echo "-timeout=$PARTY2_TEST_TIMEOUT"
        return
    fi
    if [[ "${GOFLAGS:-}" =~ -timeout(=|[[:space:]]+) ]]; then
        echo ""
        return
    fi
    echo "-timeout=30m"
}
TIMEOUT_ARG=$(resolve_timeout_arg)

echo "==> [6/7] Running full test suite..."
if command -v go >/dev/null 2>&1; then
    # Go's test cache cannot see the database, so a schema change must clear it.
    SCHEMA_STAMP_FILE="${ROOT_DIR}/.cache/test_schema_stamp"
    SCHEMA_HASH=$(cat migrations/*.sql compose.test.yaml 2>/dev/null | sha256sum) || true
    if [ "$FORCE_VERIFY" != "0" ] || [ "$SCHEMA_HASH" != "$(cat "$SCHEMA_STAMP_FILE" 2>/dev/null)" ]; then
        go clean -testcache
        mkdir -p "${ROOT_DIR}/.cache"
        echo "$SCHEMA_HASH" > "$SCHEMA_STAMP_FILE"
    fi
    # Step 4 already verified ./internal/architecture on this unchanged tree, and .git/.cache walks prevent reliable Go testcache hits there.
    SUITE_PKGS=$(go list ./... | awk '!/[/]internal[/]architecture$/ { printf "%s%s", (sep ? " " : ""), $0; sep=1 }')
    if [ -z "$SUITE_PKGS" ]; then
        echo "==> [verify.sh] ERROR: No test packages resolved for Step 6." >&2
        exit 1
    fi
    PARTY2_DB_DSN="party2:party2@tcp(127.0.0.1:${TEST_DB_PORT})/party2?parseTime=true" \
    PARTY2_TEST_DB_ADMIN_DSN="root:root@tcp(127.0.0.1:${TEST_DB_PORT})/?parseTime=true" \
    PARTY2_VALKEY_ADDR="127.0.0.1:${TEST_VALKEY_PORT}" \
    go test ${TIMEOUT_ARG} ${SUITE_PKGS}
else
    docker compose run --rm app sh -c 'suite_pkgs=$(go list ./... | awk '\''!/[/]internal[/]architecture$/ { printf "%s%s", (sep ? " " : ""), $0; sep=1 }'\'') && [ -n "$suite_pkgs" ] && go test '"${TIMEOUT_ARG}"' -count=1 $suite_pkgs'
fi

echo "==> [7/7] Running smoke image build..."
docker build -f Dockerfile -t party2re:smoke .

FINAL_TREE=$(get_tree_hash)
if [ "$CHECKED_TREE" != "$FINAL_TREE" ]; then
    echo "==> [verify.sh] ERROR: Working tree changed during verification (expected ${CHECKED_TREE:0:8}, got ${FINAL_TREE:0:8})." >&2
    echo "==> [verify.sh] Verification aborted without saving stamp." >&2
    exit 1
fi

if [ "$FINAL_TREE" != "none" ]; then
    mkdir -p "${ROOT_DIR}/.cache"
    echo "$FINAL_TREE" > "$STAMP_FILE"
fi

echo "==> ALL VERIFICATION CHECKS PASSED!"
