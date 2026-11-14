#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

IMAGE="${1:-party2re:smoke}"
if ! docker image inspect "$IMAGE" >/dev/null 2>&1; then
    echo "ERROR: Docker image '$IMAGE' not found. Build it first (e.g. docker build -f Dockerfile -t $IMAGE .)." >&2
    exit 1
fi

export COMPOSE_FILE="${PARTY2_SMOKE_COMPOSE_FILE:-${ROOT_DIR}/compose.test.yaml}"

echo "==> [docker_smoke] Starting MariaDB and Valkey..."
docker compose up -d --wait mariadb valkey
"${ROOT_DIR}/scripts/migrate.sh"

mariadb_cid=$(docker compose ps -q mariadb)
if [ -z "$mariadb_cid" ]; then
    echo "ERROR: could not locate mariadb container in compose stack" >&2
    exit 1
fi

network=$(docker inspect -f '{{range $k, $v := .NetworkSettings.Networks}}{{println $k}}{{end}}' "$mariadb_cid" | head -n 1)
if [ -z "$network" ]; then
    echo "ERROR: could not determine Docker network for mariadb" >&2
    exit 1
fi

container_name="party2-smoke-$$"
cleanup() {
    local status=$?
    if docker container inspect "$container_name" >/dev/null 2>&1; then
        if [ "$status" -ne 0 ]; then
            echo "==> [docker_smoke] Container logs ($container_name):" >&2
            docker logs "$container_name" >&2 || true
        fi
        docker rm -f "$container_name" >/dev/null 2>&1 || true
    fi
    exit "$status"
}
trap cleanup EXIT

docker rm -f "$container_name" >/dev/null 2>&1 || true

echo "==> [docker_smoke] Starting container from image '$IMAGE'..."
docker run -d \
    --name "$container_name" \
    --network "$network" \
    -p 127.0.0.1::8080 \
    -e PARTY2_DB_DSN="party2:party2@tcp(mariadb:3306)/party2?parseTime=true" \
    -e PARTY2_VALKEY_ADDR="valkey:6379" \
    -e PARTY2_MAX_PLAYERS="0" \
    "$IMAGE" >/dev/null

health_timeout="${PARTY2_SMOKE_HEALTH_TIMEOUT_SECONDS:-30}"
deadline=$((SECONDS + health_timeout))
base_url=""

echo "==> [docker_smoke] Waiting for /health..."
while true; do
    running=$(docker inspect -f '{{.State.Running}}' "$container_name" 2>/dev/null || echo "false")
    if [ "$running" != "true" ]; then
        echo "ERROR: container $container_name exited before /health became ready" >&2
        exit 1
    fi
    if [ -z "$base_url" ]; then
        host_addr=$(docker port "$container_name" 8080/tcp 2>/dev/null | head -n 1 || true)
        if [ -n "$host_addr" ]; then
            base_url="http://${host_addr}"
        fi
    fi
    remaining=$((deadline - SECONDS))
    if [ -n "$base_url" ] && [ "$remaining" -gt 0 ]; then
        probe_timeout=$((remaining < 2 ? remaining : 2))
        if curl --connect-timeout "$probe_timeout" --max-time "$probe_timeout" -fsS "${base_url}/health" >/dev/null 2>&1; then
            break
        fi
    fi
    if [ "$SECONDS" -ge "$deadline" ]; then
        echo "ERROR: timed out after ${health_timeout}s waiting for /health" >&2
        exit 1
    fi
    sleep 0.5
done

LAST_STATUS=""
LAST_BODY=""

request() {
    local method="$1" path="$2" want_status="$3" body="${4:-}" token="${5:-}"
    local tmp status
    local request_timeout="${PARTY2_SMOKE_REQUEST_TIMEOUT_SECONDS:-15}"
    tmp=$(mktemp)
    local -a args=(--connect-timeout "$request_timeout" --max-time "$request_timeout" -sS -o "$tmp" -w '%{http_code}' -X "$method" -H "Content-Type: application/json")
    if [ -n "$token" ]; then
        args+=(-H "Authorization: Bearer $token")
    fi
    if [ -n "$body" ]; then
        args+=(-d "$body")
    fi
    if ! status=$(curl "${args[@]}" "${base_url}${path}"); then
        echo "ERROR: ${method} ${path} failed to connect" >&2
        rm -f "$tmp"
        return 1
    fi
    LAST_STATUS="$status"
    LAST_BODY=$(cat "$tmp")
    rm -f "$tmp"
    if [ "$LAST_STATUS" != "$want_status" ]; then
        echo "ERROR: ${method} ${path}: expected HTTP ${want_status}, got ${LAST_STATUS}: ${LAST_BODY}" >&2
        return 1
    fi
}

json_field() {
    python3 -c '
import json, sys
data = json.load(sys.stdin)
cur = data
for key in sys.argv[1].split("."):
    if not isinstance(cur, dict) or key not in cur:
        sys.exit(1)
    cur = cur[key]
if cur is None or cur == "":
    sys.exit(1)
print(cur)
' "$1"
}

echo "==> [docker_smoke] Exercising HTTP sequence against ${base_url}..."

request GET /health 200
health_status=$(printf '%s' "$LAST_BODY" | json_field status)
if [ "$health_status" != "ok" ]; then
    echo "ERROR: unexpected /health status field: $health_status" >&2
    exit 1
fi

smoke_name="s$(python3 -c 'import secrets; print(secrets.token_hex(4)[:7])')"
smoke_pass="smoke-pass-1"
creds=$(printf '{"username":"%s","password":"%s"}' "$smoke_name" "$smoke_pass")

request POST /players 201 "$creds"
request POST /sessions 201 "$creds"
token=$(printf '%s' "$LAST_BODY" | json_field id)

char_req=$(printf '{"name":"%s","job_id":"job-01","gender":"m"}' "$smoke_name")
request POST /characters 201 "$char_req" "$token"
char_id=$(printf '%s' "$LAST_BODY" | json_field id)

request GET "/api/v1/characters/${char_id}/context" 200 "" "$token"
got_char_id=$(printf '%s' "$LAST_BODY" | json_field character.id)
if [ "$got_char_id" != "$char_id" ]; then
    echo "ERROR: context returned character.id=${got_char_id}, want ${char_id}" >&2
    exit 1
fi
printf '%s' "$LAST_BODY" | json_field scene.location_id >/dev/null

request DELETE /players/me 200 "$(printf '{"password":"%s"}' "$smoke_pass")" "$token"

echo "==> [docker_smoke] Production image smoke test passed."
