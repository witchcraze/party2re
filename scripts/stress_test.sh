#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

if [ "$#" -eq 0 ]; then
    set -- ./...
fi
JSON_OUTPUT=false
TEST_ARGS=()
FORWARD_ONLY=false
for arg in "$@"; do
    if [ "$FORWARD_ONLY" = true ]; then
        TEST_ARGS+=("$arg")
        continue
    fi
    case "$arg" in
        -json|--json) JSON_OUTPUT=true ;;
        -json=*|--json=*) JSON_OUTPUT="${arg#*=}" ;;
        -args|--args|--) FORWARD_ONLY=true; TEST_ARGS+=("$arg") ;;
        *) TEST_ARGS+=("$arg") ;;
    esac
done

LOG_FD=1
case "$JSON_OUTPUT" in
    1|t|T|true|TRUE|True) LOG_FD=2 ;;
esac
echo "==> Ensuring services and database migrations are ready..." >&"$LOG_FD"
docker compose up -d --wait mariadb valkey >&"$LOG_FD"
"${ROOT_DIR}/scripts/migrate.sh" >&"$LOG_FD"

echo "==> Running tests with adaptive deep stress enabled..." >&"$LOG_FD"
set +e
PARTY2_DB_DSN="${PARTY2_DB_DSN:-party2:party2@tcp(127.0.0.1:3306)/party2?parseTime=true}" \
PARTY2_TEST_DB_ADMIN_DSN="${PARTY2_TEST_DB_ADMIN_DSN:-root:root@tcp(127.0.0.1:3306)/?parseTime=true}" \
PARTY2_VALKEY_ADDR="${PARTY2_VALKEY_ADDR:-127.0.0.1:6379}" \
PARTY2_STRESS_ENABLED="1" \
go test -json -v -count=1 "${TEST_ARGS[@]}" | go run ./scripts/stresscheck/main.go -json="$JSON_OUTPUT"
TEST_STATUS=("${PIPESTATUS[@]}")
set -e

if [ "${TEST_STATUS[0]}" -ne 0 ]; then
    exit "${TEST_STATUS[0]}"
fi
if [ "${TEST_STATUS[1]}" -ne 0 ]; then
    exit "${TEST_STATUS[1]}"
fi

echo "==> DEEP STRESS TEST RUN COMPLETED!" >&"$LOG_FD"
