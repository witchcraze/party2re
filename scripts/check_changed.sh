#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

select_packages() {
    local base changed
    base=$(git merge-base origin/main HEAD) || return 1
    changed=$(git diff --name-only --no-renames "$base" && git ls-files --others --exclude-standard) || return 1
    printf '%s\n' "$changed" | go run ./scripts/changedpkgs
}

if ! packages=$(select_packages); then
    echo "==> [check-changed] Full suite: could not determine the changes against origin/main." >&2
    packages=./...
fi

echo "==> [check-changed] Selected packages:"
printf '    %s\n' ${packages:-"(none)"}

# These checks read source across packages, which import data cannot reveal.
echo "==> [check-changed] Running repository-wide checks (internal/architecture, internal/core, lock hierarchy)..."
go test -count=1 ./internal/database -run "LockHierarchy"

echo "==> [check-changed] Ensuring services are healthy and migrations are applied..."
"${ROOT_DIR}/scripts/migrate.sh"

timeout="${TEST_TIMEOUT-30m}"
PARTY2_DB_DSN="party2:party2@tcp(127.0.0.1:3306)/party2?parseTime=true" \
PARTY2_TEST_DB_ADMIN_DSN="root:root@tcp(127.0.0.1:3306)/?parseTime=true" \
PARTY2_VALKEY_ADDR="127.0.0.1:6379" \
go test ${timeout:+-timeout="$timeout"} -count=1 ./internal/architecture ./internal/core $packages

echo "==> [check-changed] Selected tests passed. This does not replace make check."
