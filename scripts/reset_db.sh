#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

echo "==> Resetting database (DROP and CREATE party2 database)..."
docker compose up -d mariadb valkey >/dev/null 2>&1

for i in {1..30}; do
    if docker compose exec -T mariadb mariadb -u root -proot -e "SELECT 1" >/dev/null 2>&1; then
        break
    fi
    sleep 0.5
done

docker compose exec -T mariadb mariadb -u root -proot -e \
    "DROP DATABASE IF EXISTS party2; CREATE DATABASE party2; GRANT ALL PRIVILEGES ON party2.* TO 'party2'@'%'; FLUSH PRIVILEGES;"

"${ROOT_DIR}/scripts/migrate.sh"
echo "==> Database reset and migrations completed successfully."
