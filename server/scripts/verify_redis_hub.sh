#!/usr/bin/env bash
# verify_redis_hub.sh — Step 31 multi-instance MessageHub smoke check
# (docs/tasks/step31.md).
#
# Brings up db, migrate, redis, and llm-gateway, then scales `api` to two
# replicas using docker-compose.scale-test.yml (so each replica gets its own
# auto-assigned host port instead of colliding on 8080). It registers a test
# user, opens a WebSocket connection against replica 1, sends a room message
# via HTTP against replica 2, and asserts the message is delivered over
# replica 1's WebSocket within a timeout — proving RedisHub fans events out
# across API server processes rather than only within one, which InProcessHub
# cannot do.
#
# Exits 0 on success, non-zero on any failure (missing prerequisite, replica
# never becomes healthy, or the cross-instance delivery timing out).
#
# Explicitly forces AUTH_MODE=simple_jwt for this stack's own bring-up
# (independent of whatever AUTH_MODE default docker-compose.yml ships with),
# so this script keeps working with the register/login flow below regardless
# of which AuthService implementation is the compose default in a given
# wave.
#
# Prerequisites: docker, docker compose (v2, with the `--wait` flag), curl,
# jq, go (1.25+, to run the verifywshub WebSocket test client via `go run`).
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
cd "$REPO_ROOT"

for bin in docker curl jq go; do
  if ! command -v "$bin" >/dev/null 2>&1; then
    echo "verify_redis_hub: required tool '$bin' not found on PATH" >&2
    exit 1
  fi
done

export AUTH_MODE=simple_jwt

COMPOSE=(docker compose -f docker-compose.yml -f docker-compose.scale-test.yml)
BASE_COMPOSE=(docker compose)

RUN_SUFFIX="$(date +%s)"
TEST_EMAIL="verify-redis-hub-${RUN_SUFFIX}@example.com"
TEST_USERNAME="verify-redis-hub-${RUN_SUFFIX}"
TEST_PASSWORD="verify-redis-hub-password"
MESSAGE_CONTENT="redis-hub-cross-instance-check-${RUN_SUFFIX}"
WS_WAIT_LOG="$(mktemp)"

cleanup() {
  local status=$?
  echo "==> Tearing down the scale-test stack"
  "${COMPOSE[@]}" down >/dev/null 2>&1 || true
  rm -f "$WS_WAIT_LOG"
  exit "$status"
}
trap cleanup EXIT

echo "==> Starting db, migrate, redis, llm-gateway"
"${BASE_COMPOSE[@]}" up -d --wait db migrate redis llm-gateway

echo "==> Scaling api to 2 replicas via docker-compose.scale-test.yml"
"${COMPOSE[@]}" up -d --build --wait --scale api=2 api

port_for_replica() {
  # docker compose port prints "0.0.0.0:PORT"; keep only the port number.
  "${COMPOSE[@]}" port --index="$1" api 8080 | sed -E 's/.*://'
}

PORT1="$(port_for_replica 1)"
PORT2="$(port_for_replica 2)"
if [[ -z "$PORT1" || -z "$PORT2" ]]; then
  echo "verify_redis_hub: failed to resolve replica host ports" >&2
  exit 1
fi
echo "==> Replica 1 on host port ${PORT1}, replica 2 on host port ${PORT2}"

BASE1="http://localhost:${PORT1}"
BASE2="http://localhost:${PORT2}"

echo "==> Registering test user against replica 1"
REGISTER_BODY="$(curl -sS -X POST "${BASE1}/auth/register" \
  -H 'Content-Type: application/json' \
  -d "{\"email\":\"${TEST_EMAIL}\",\"username\":\"${TEST_USERNAME}\",\"password\":\"${TEST_PASSWORD}\"}")"
ACCESS_TOKEN="$(jq -r '.access_token // empty' <<<"$REGISTER_BODY")"
if [[ -z "$ACCESS_TOKEN" ]]; then
  echo "verify_redis_hub: registration failed: ${REGISTER_BODY}" >&2
  exit 1
fi

echo "==> Creating a room against replica 1"
ROOM_BODY="$(curl -sS -X POST "${BASE1}/rooms" \
  -H "Authorization: Bearer ${ACCESS_TOKEN}" \
  -H 'Content-Type: application/json' \
  -d "{\"name\":\"redis-hub-verify-${RUN_SUFFIX}\",\"description\":\"\"}")"
ROOM_ID="$(jq -r '.id // empty' <<<"$ROOM_BODY")"
if [[ -z "$ROOM_ID" ]]; then
  echo "verify_redis_hub: room creation failed: ${ROOM_BODY}" >&2
  exit 1
fi

echo "==> Issuing a WebSocket ticket against replica 1"
TICKET_BODY="$(curl -sS -X POST "${BASE1}/ws/ticket" \
  -H "Authorization: Bearer ${ACCESS_TOKEN}")"
TICKET="$(jq -r '.ticket // empty' <<<"$TICKET_BODY")"
if [[ -z "$TICKET" ]]; then
  echo "verify_redis_hub: ws ticket issuance failed: ${TICKET_BODY}" >&2
  exit 1
fi

WS_URL="ws://localhost:${PORT1}/rooms/${ROOM_ID}/ws?ticket=${TICKET}"

echo "==> Opening a WebSocket to replica 1 (${BASE1}) and waiting for the room event"
(
  cd server
  go run ./cmd/verifywshub -url "$WS_URL" -timeout 20s
) >"$WS_WAIT_LOG" 2>&1 &
WS_PID=$!

# Give the WebSocket client time to dial, upgrade, and subscribe on the hub
# before we publish — RedisHub's Subscribe issues a Redis SUBSCRIBE command
# that needs a moment to take effect, especially over the Docker network.
sleep 3

echo "==> Sending a message via HTTP against replica 2 (${BASE2})"
SEND_BODY="$(curl -sS -X POST "${BASE2}/rooms/${ROOM_ID}/messages" \
  -H "Authorization: Bearer ${ACCESS_TOKEN}" \
  -H 'Content-Type: application/json' \
  -d "{\"content\":\"${MESSAGE_CONTENT}\"}")"
if [[ "$(jq -r '.id // empty' <<<"$SEND_BODY")" == "" ]]; then
  echo "verify_redis_hub: send message via replica 2 failed: ${SEND_BODY}" >&2
  kill "$WS_PID" 2>/dev/null || true
  exit 1
fi

echo "==> Waiting for the WebSocket client (connected to replica 1) to observe the event"
if ! wait "$WS_PID"; then
  echo "verify_redis_hub: WebSocket client on replica 1 did not receive the event in time" >&2
  cat "$WS_WAIT_LOG" >&2
  exit 1
fi
cat "$WS_WAIT_LOG"

if ! grep -q "$MESSAGE_CONTENT" "$WS_WAIT_LOG"; then
  echo "verify_redis_hub: received event frame did not contain the expected message content" >&2
  exit 1
fi

echo "==> SUCCESS: message sent via replica 2 was delivered over replica 1's WebSocket"
