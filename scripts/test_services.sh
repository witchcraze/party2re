# Sourced by verify.sh and check_changed.sh with ROOT_DIR set.
# Selects the services for DB-backed tests and sets TEST_DB_PORT and TEST_VALKEY_PORT.

TEST_DB_PORT="${PARTY2_TEST_DB_PORT:-3307}"
TEST_VALKEY_PORT="${PARTY2_TEST_VALKEY_PORT:-6380}"
export COMPOSE_FILE="${ROOT_DIR}/compose.test.yaml"

# A test stack that cannot start (occupied ports, no tmpfs support) must not block
# verification. Without host Go, tests run in compose.yaml's app service.
if ! { command -v go >/dev/null 2>&1 && docker compose up -d --wait mariadb valkey; }; then
    echo "==> WARNING: test stack unavailable. Testing against the DEVELOPMENT services and database instead." >&2
    docker compose down >/dev/null 2>&1 || true
    unset COMPOSE_FILE
    TEST_DB_PORT=3306
    TEST_VALKEY_PORT=6379
fi
