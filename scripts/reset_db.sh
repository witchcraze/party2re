#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

echo "==> Resetting database (DROP and CREATE party2 database)..."
docker compose up -d --wait mariadb valkey

docker compose exec -T mariadb mariadb -h 127.0.0.1 -u root -proot -e \
    "DROP DATABASE IF EXISTS party2; CREATE DATABASE party2; GRANT ALL PRIVILEGES ON party2.* TO 'party2'@'%'; FLUSH PRIVILEGES;"

"${ROOT_DIR}/scripts/migrate.sh"
echo "==> Database reset and migrations completed successfully."
