#!/usr/bin/env bash
set -euo pipefail

# Deterministically partition Go packages across CI shards so heavy suites spread
# evenly while guaranteeing complete, duplicate-free coverage of `go list ./...`.
if [ "$#" -lt 2 ] || [ "$#" -gt 3 ]; then
    echo "Usage: $0 <shard-index> <shard-count> [--stdin]" >&2
    exit 2
fi

SHARD_INDEX="$1"
SHARD_COUNT="$2"
SOURCE_MODE="${3:-go-list}"

case "$SHARD_INDEX:$SHARD_COUNT" in
    *[!0-9:]*|:*)
        echo "Invalid shard arguments: index=$SHARD_INDEX count=$SHARD_COUNT" >&2
        exit 2
        ;;
esac
if [ "$SHARD_COUNT" -lt 1 ] || [ "$SHARD_INDEX" -lt 0 ] || [ "$SHARD_INDEX" -ge "$SHARD_COUNT" ]; then
    echo "Shard index out of range: index=$SHARD_INDEX count=$SHARD_COUNT" >&2
    exit 2
fi

if [ "$SOURCE_MODE" = "--stdin" ]; then
    RAW_PKGS=$(cat)
elif [ "$SOURCE_MODE" = "go-list" ]; then
    RAW_PKGS=$(go list ./...)
else
    echo "Unknown source mode: $SOURCE_MODE" >&2
    exit 2
fi

PKGS=$(printf '%s\n' "$RAW_PKGS" | awk 'NF > 0' | LC_ALL=C sort -u)
if [ -z "$PKGS" ]; then
    echo "No Go packages resolved for sharding" >&2
    exit 1
fi

# Ordered by post-#2144 GOMAXPROCS=4 runtimes; remaining/new packages fall through to round-robin.
HEAVY_SUFFIXES="internal/database cmd/party2 internal/casino internal/guild internal/api/http internal/adventure internal/home scripts internal/challenge internal/architecture internal/player internal/pvp internal/gvg internal/scheduling internal/placelog"

printf '%s\n' "$PKGS" | awk -v shard="$SHARD_INDEX" -v count="$SHARD_COUNT" -v heavy="$HEAVY_SUFFIXES" '
BEGIN {
    n = split(heavy, h, " ")
}
{
    all[++total] = $0
    for (i = 1; i <= n; i++) {
        s = h[i]
        if ($0 == s || (length($0) > length(s) && substr($0, length($0) - length(s)) == "/" s)) {
            if (!($0 in is_heavy)) {
                assigned[i] = $0
                is_heavy[$0] = 1
            }
            break
        }
    }
}
END {
    pos = 0
    for (i = 1; i <= n; i++) {
        if (i in assigned) {
            cycle = pos % (2 * count)
            s = (cycle < count) ? cycle : (2 * count - 1 - cycle)
            if (s == shard) {
                print assigned[i]
            }
            pos++
        }
    }
    for (j = 1; j <= total; j++) {
        if (!(all[j] in is_heavy)) {
            s = pos % count
            if (s == shard) {
                print all[j]
            }
            pos++
        }
    }
}
'
