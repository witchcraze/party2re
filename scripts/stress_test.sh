#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

echo "==> Ensuring services and database migrations are ready..."
docker compose up -d --wait mariadb valkey
"${ROOT_DIR}/scripts/migrate.sh"

if [ "$#" -eq 0 ]; then
    set -- ./...
fi
TEST_LOG=$(mktemp)
trap 'rm -f "$TEST_LOG"' EXIT

echo "==> Running tests with adaptive deep stress enabled..."
PARTY2_DB_DSN="${PARTY2_DB_DSN:-party2:party2@tcp(127.0.0.1:3306)/party2?parseTime=true}" \
PARTY2_TEST_DB_ADMIN_DSN="${PARTY2_TEST_DB_ADMIN_DSN:-root:root@tcp(127.0.0.1:3306)/?parseTime=true}" \
PARTY2_VALKEY_ADDR="${PARTY2_VALKEY_ADDR:-127.0.0.1:6379}" \
PARTY2_STRESS_ENABLED="1" \
go test -v -count=1 "$@" | tee "$TEST_LOG"

if ! grep -q '^--- PASS: ' "$TEST_LOG"; then
    echo "ERROR: No tests completed successfully (empty selection or all skipped)." >&2
    exit 1
fi

echo "==> DEEP STRESS TEST RUN COMPLETED!"
