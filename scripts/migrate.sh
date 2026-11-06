#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

DB_NAME="${PARTY2_DB_NAME:-party2}"

echo "==> Ensuring services are healthy..."
docker compose up -d --wait mariadb valkey

# Ensure schema_migrations table exists
docker compose exec -T mariadb mariadb -h 127.0.0.1 -u party2 -pparty2 "$DB_NAME" -e \
    "CREATE TABLE IF NOT EXISTS schema_migrations (version VARCHAR(255) PRIMARY KEY, applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP);"

echo "==> Applying database migrations..."
for file in $(ls migrations/*.sql | sort); do
    version=$(basename "$file" .sql)
    raw_applied=$(docker compose exec -T mariadb mariadb -h 127.0.0.1 -u party2 -pparty2 -N -e \
        "SELECT COUNT(1) FROM schema_migrations WHERE version = '$version';" "$DB_NAME")
    applied=$(printf '%s' "$raw_applied" | tr -d '[:space:]')
    case "$applied" in
        0)
            echo "  -> Applying $file..."
            docker compose exec -T mariadb mariadb -h 127.0.0.1 -u party2 -pparty2 "$DB_NAME" < "$file"
            docker compose exec -T mariadb mariadb -h 127.0.0.1 -u party2 -pparty2 "$DB_NAME" -e \
                "INSERT IGNORE INTO schema_migrations (version) VALUES ('$version');"
            ;;
        1)
            ;;
        *)
            echo "ERROR: Unexpected migration history count for '$version': '${raw_applied}'" >&2
            exit 1
            ;;
    esac
done
echo "==> All migrations are up to date."
