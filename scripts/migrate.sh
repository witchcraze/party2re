#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

echo "==> Ensuring database container is running..."
docker compose up -d mariadb valkey >/dev/null 2>&1

echo "==> Waiting for MariaDB to be healthy..."
for i in {1..60}; do
    HEALTH=$(docker inspect --format='{{.State.Health.Status}}' $(docker compose ps -q mariadb) 2>/dev/null || echo "unhealthy")
    if [ "$HEALTH" = "healthy" ]; then
        break
    fi
    sleep 1
done

if ! docker compose exec -T mariadb mariadb -h 127.0.0.1 -u party2 -pparty2 -e "SELECT 1" party2 >/dev/null 2>&1; then
    echo "ERROR: MariaDB is still not accepting connections."
    exit 1
fi

# Ensure schema_migrations table exists
docker compose exec -T mariadb mariadb -h 127.0.0.1 -u party2 -pparty2 party2 -e \
    "CREATE TABLE IF NOT EXISTS schema_migrations (version VARCHAR(255) PRIMARY KEY, applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP);" >/dev/null 2>&1 || true

echo "==> Applying database migrations..."
for file in $(ls migrations/*.sql | sort); do
    version=$(basename "$file" .sql)
    applied=$(docker compose exec -T mariadb mariadb -h 127.0.0.1 -u party2 -pparty2 -N -e \
        "SELECT COUNT(1) FROM schema_migrations WHERE version = '$version';" party2 2>/dev/null | tr -d '[:space:]' || echo "0")
    if [ "$applied" = "0" ]; then
        echo "  -> Applying $file..."
        docker compose exec -T mariadb mariadb -h 127.0.0.1 -u party2 -pparty2 party2 < "$file"
        docker compose exec -T mariadb mariadb -h 127.0.0.1 -u party2 -pparty2 party2 -e \
            "INSERT INTO schema_migrations (version) VALUES ('$version');" >/dev/null 2>&1 || true
    fi
done
echo "==> All migrations are up to date."
