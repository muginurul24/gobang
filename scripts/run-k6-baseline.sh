#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
mkdir -p "${ROOT_DIR}/.tmp"
TMP_DIR="$(mktemp -d "${ROOT_DIR}/.tmp/onixggr-k6.XXXXXX")"
chmod 0777 "${TMP_DIR}"
API_PID=""
MOCK_PID=""
KEEP_STACK="${KEEP_STACK:-0}"

cleanup() {
  set +e
  if [[ -n "${API_PID}" ]]; then
    kill "${API_PID}" >/dev/null 2>&1 || true
    wait "${API_PID}" >/dev/null 2>&1 || true
  fi
  if [[ -n "${MOCK_PID}" ]]; then
    kill "${MOCK_PID}" >/dev/null 2>&1 || true
    wait "${MOCK_PID}" >/dev/null 2>&1 || true
  fi
  if [[ "${KEEP_STACK}" != "1" ]]; then
    podman compose -f docker-compose.yml down >/dev/null 2>&1 || true
    rm -rf "${TMP_DIR}"
  fi
}
trap cleanup EXIT

run_k6() {
  local script_path="$1"
  local summary_path="$2"
  local container_summary="/workspace${summary_path#"${ROOT_DIR}"}"

  if command -v k6 >/dev/null 2>&1; then
    K6_BASE_URL="${K6_BASE_URL}" \
      K6_STORE_ID="${K6_STORE_ID}" \
      K6_STORE_TOKEN="${K6_STORE_TOKEN}" \
      K6_OWNER_LOGIN="${K6_OWNER_LOGIN}" \
      K6_OWNER_PASSWORD="${K6_OWNER_PASSWORD}" \
      K6_MEMBER_USERNAME="${K6_MEMBER_USERNAME}" \
      K6_ALT_MEMBER_USERNAME="${K6_ALT_MEMBER_USERNAME}" \
      K6_DURATION="${K6_DURATION}" \
      K6_WEBHOOK_DURATION="${K6_WEBHOOK_DURATION}" \
      K6_WS_HOLD="${K6_WS_HOLD}" \
      k6 run --quiet --summary-trend-stats avg,med,p\(95\),p\(99\),max --summary-export "${summary_path}" "${script_path}"
    return
  fi

  podman run --rm --network host \
    -v "${ROOT_DIR}:/workspace:Z" \
    -w /workspace \
    -e K6_BASE_URL="${K6_BASE_URL}" \
    -e K6_STORE_ID="${K6_STORE_ID}" \
    -e K6_STORE_TOKEN="${K6_STORE_TOKEN}" \
    -e K6_OWNER_LOGIN="${K6_OWNER_LOGIN}" \
    -e K6_OWNER_PASSWORD="${K6_OWNER_PASSWORD}" \
    -e K6_MEMBER_USERNAME="${K6_MEMBER_USERNAME}" \
    -e K6_ALT_MEMBER_USERNAME="${K6_ALT_MEMBER_USERNAME}" \
    -e K6_DURATION="${K6_DURATION}" \
    -e K6_WEBHOOK_DURATION="${K6_WEBHOOK_DURATION}" \
    -e K6_WS_HOLD="${K6_WS_HOLD}" \
    docker.io/grafana/k6:0.49.0 \
    run --quiet --summary-trend-stats avg,med,p\(95\),p\(99\),max --summary-export "${container_summary}" "${script_path}"
}

print_http_summary() {
  local label="$1"
  local summary_path="$2"
  jq -r --arg label "${label}" '
    [
      $label,
      "p95=" + (.metrics.http_req_duration["p(95)"] | tostring) + "ms",
      "p99=" + (.metrics.http_req_duration["p(99)"] | tostring) + "ms"
    ] | join(" ")
  ' "${summary_path}"
}

print_ws_summary() {
  local label="$1"
  local summary_path="$2"
  jq -r --arg label "${label}" '
    [
      $label,
      "connect_p95=" + (.metrics.onixggr_ws_connect_duration["p(95)"] | tostring) + "ms",
      "connect_p99=" + (.metrics.onixggr_ws_connect_duration["p(99)"] | tostring) + "ms",
      "session_p95=" + (.metrics.onixggr_ws_session_duration["p(95)"] | tostring) + "ms",
      "session_p99=" + (.metrics.onixggr_ws_session_duration["p(99)"] | tostring) + "ms"
    ] | join(" ")
  ' "${summary_path}"
}

wait_for_url() {
  local url="$1"
  local name="$2"

  for _ in {1..60}; do
    if curl -fsS "${url}" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done

  echo "${name} failed to become ready" >&2
  return 1
}

wait_for_compose_dependencies() {
  for _ in {1..60}; do
    if podman exec onixggr-postgres pg_isready -U postgres -d onixggr >/dev/null 2>&1 \
      && podman exec onixggr-redis redis-cli ping >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done

  echo "postgres/redis failed to become ready" >&2
  return 1
}

cd "${ROOT_DIR}"

export K6_BASE_URL="${K6_BASE_URL:-http://127.0.0.1:18080}"
export K6_STORE_ID="${K6_STORE_ID:-cccccccc-cccc-cccc-cccc-cccccccccccc}"
export K6_STORE_TOKEN="${K6_STORE_TOKEN:-store_live_demo}"
export K6_OWNER_LOGIN="${K6_OWNER_LOGIN:-owner@example.com}"
export K6_OWNER_PASSWORD="${K6_OWNER_PASSWORD:-OwnerDemo123!}"
export K6_MEMBER_USERNAME="${K6_MEMBER_USERNAME:-member-demo}"
export K6_ALT_MEMBER_USERNAME="${K6_ALT_MEMBER_USERNAME:-member-alpha}"
export K6_DURATION="${K6_DURATION:-20s}"
export K6_WEBHOOK_DURATION="${K6_WEBHOOK_DURATION:-15s}"
export K6_WS_HOLD="${K6_WS_HOLD:-10s}"

podman compose -f docker-compose.yml down >/dev/null 2>&1 || true
podman compose -f docker-compose.yml up -d postgres redis >/dev/null
wait_for_compose_dependencies
./appctl migrate fresh --seed >/dev/null

MOCK_UPSTREAM_ADDRESS=:18081 \
MOCK_NEXUSGGR_AGENT_CODE=demo-agent \
MOCK_NEXUSGGR_AGENT_TOKEN=demo-token \
MOCK_QRIS_CLIENT=demo-client \
MOCK_QRIS_CLIENT_KEY=demo-key \
MOCK_QRIS_GLOBAL_UUID=demo-uuid \
GOCACHE=/tmp/onixggr-go-build \
go run ./scripts/mock-upstreams >"${TMP_DIR}/mock-upstreams.log" 2>&1 &
MOCK_PID=$!

APP_URL=http://127.0.0.1:4173 \
HTTP_ADDRESS=:18080 \
DATABASE_URL=postgresql://postgres:postgres@127.0.0.1:15432/onixggr?sslmode=disable \
REDIS_URL=redis://127.0.0.1:16379 \
NEXUSGGR_BASE_URL=http://127.0.0.1:18081 \
NEXUSGGR_AGENT_CODE=demo-agent \
NEXUSGGR_AGENT_TOKEN=demo-token \
QRIS_BASE_URL=http://127.0.0.1:18081 \
QRIS_CLIENT=demo-client \
QRIS_CLIENT_KEY=demo-key \
QRIS_GLOBAL_UUID=demo-uuid \
QRIS_WEBHOOK_SHARED_SECRET= \
METRICS_ENABLED=false \
GOCACHE=/tmp/onixggr-go-build \
go run ./apps/api >"${TMP_DIR}/api.log" 2>&1 &
API_PID=$!

wait_for_url "http://127.0.0.1:18081/healthz" "mock upstream"
wait_for_url "http://127.0.0.1:18080/health/ready" "api"

run_k6 "./deploy/performance/k6/login.js" "${TMP_DIR}/login.json"
run_k6 "./deploy/performance/k6/game-deposit.js" "${TMP_DIR}/game-deposit.json"
run_k6 "./deploy/performance/k6/game-withdraw.js" "${TMP_DIR}/game-withdraw.json"
run_k6 "./deploy/performance/k6/game-balance.js" "${TMP_DIR}/game-balance.json"
run_k6 "./deploy/performance/k6/qris-generate.js" "${TMP_DIR}/qris-generate.json"
run_k6 "./deploy/performance/k6/webhook-burst.js" "${TMP_DIR}/webhook-burst.json"
run_k6 "./deploy/performance/k6/websocket-concurrency.js" "${TMP_DIR}/websocket-concurrency.json"

print_http_summary "login" "${TMP_DIR}/login.json"
print_http_summary "game-deposit" "${TMP_DIR}/game-deposit.json"
print_http_summary "game-withdraw" "${TMP_DIR}/game-withdraw.json"
print_http_summary "game-balance" "${TMP_DIR}/game-balance.json"
print_http_summary "qris-generate" "${TMP_DIR}/qris-generate.json"
print_http_summary "webhook-burst" "${TMP_DIR}/webhook-burst.json"
print_ws_summary "websocket-concurrency" "${TMP_DIR}/websocket-concurrency.json"

echo "raw summaries: ${TMP_DIR}"
