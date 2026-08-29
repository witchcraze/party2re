#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

echo "==> Ensuring services and database migrations are ready..."
"${ROOT_DIR}/scripts/migrate.sh"

echo "==> Running High-Concurrency Stress and Deadlock Detection Benchmark..."
PARTY2_DB_DSN="${PARTY2_DB_DSN:-party2:party2@tcp(127.0.0.1:3306)/party2?parseTime=true}" \
PARTY2_VALKEY_ADDR="${PARTY2_VALKEY_ADDR:-127.0.0.1:6379}" \
PARTY2_STRESS_ENABLED="1" \
go test -v -count=1 -run "TestConcurrencyStress" ./internal/database/...

echo "==> ALL CONCURRENCY STRESS & DEADLOCK CHECKS COMPLETED WITH 0 DEADLOCKS!"
