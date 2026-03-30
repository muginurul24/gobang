#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
mkdir -p "${ROOT_DIR}/.tmp"
TMP_DIR="$(mktemp -d "${ROOT_DIR}/.tmp/onixggr-failure.XXXXXX")"
chmod 0777 "${TMP_DIR}"

API_PID=""
WORKER_PID=""
MOCK_PID=""
KEEP_STACK="${KEEP_STACK:-0}"
ALERTS_READY=()

STORE_ID="cccccccc-cccc-cccc-cccc-cccccccccccc"
STORE_TOKEN="store_live_demo"
GAME_MEMBER="member-demo"
QRIS_MEMBER="member-alpha"
ORIGINAL_CALLBACK_URL="https://merchant.example.com/callback"

cleanup() {
  set +e
  if [[ -n "${WORKER_PID}" ]]; then
    kill "${WORKER_PID}" >/dev/null 2>&1 || true
    wait "${WORKER_PID}" >/dev/null 2>&1 || true
  fi
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

free_listen_port() {
  local port="$1"
  local pids

  if command -v lsof >/dev/null 2>&1; then
    pids="$(lsof -tiTCP:"${port}" -sTCP:LISTEN 2>/dev/null || true)"
  elif command -v fuser >/dev/null 2>&1; then
    pids="$(fuser "${port}"/tcp 2>/dev/null || true)"
  else
    pids=""
  fi

  if [[ -n "${pids}" ]]; then
    kill ${pids} >/dev/null 2>&1 || true
    sleep 1
  fi
}

sql() {
  podman exec onixggr-postgres psql -U postgres -d onixggr -Atqc "$1"
}

wait_for_url() {
  local url="$1"
  local name="$2"
  local attempts="${3:-60}"

  for _ in $(seq 1 "${attempts}"); do
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

wait_for_sql() {
  local query="$1"
  local expected="$2"
  local label="$3"
  local attempts="${4:-40}"

  for _ in $(seq 1 "${attempts}"); do
    local value
    value="$(sql "${query}" | tr -d '[:space:]')"
    if [[ "${value}" == "${expected}" ]]; then
      return 0
    fi
    sleep 1
  done

  echo "${label} did not reach ${expected}" >&2
  return 1
}

wait_for_metric_line() {
  local pattern="$1"
  local attempts="${2:-30}"

  for _ in $(seq 1 "${attempts}"); do
    if curl -fsS http://127.0.0.1:19090/metrics | rg -n "${pattern}" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done

  echo "metric pattern not found: ${pattern}" >&2
  return 1
}

http_status() {
  local method="$1"
  local url="$2"
  local body="${3:-}"

  if [[ -n "${body}" ]]; then
    curl -sS -o /dev/null -w '%{http_code}' -X "${method}" "${url}" -H 'content-type: application/json' -d "${body}"
  else
    curl -sS -o /dev/null -w '%{http_code}' -X "${method}" "${url}"
  fi
}

json_field() {
  local file="$1"
  local query="$2"
  jq -r "${query}" "${file}"
}

mark_alert_ready() {
  local alert="$1"

  for existing in "${ALERTS_READY[@]}"; do
    if [[ "${existing}" == "${alert}" ]]; then
      return 0
    fi
  done

  ALERTS_READY+=("${alert}")
}

set_mock_delay() {
  local provider="$1"
  local milliseconds="$2"
  curl -fsS -X POST "http://127.0.0.1:18081/_admin/${provider}/delay" \
    -H 'content-type: application/json' \
    -d "{\"milliseconds\":${milliseconds}}" >/dev/null
}

set_qris_payment_status() {
  local trx_id="$1"
  local status="$2"
  curl -fsS -X POST "http://127.0.0.1:18081/_admin/qris/payments/${trx_id}" \
    -H 'content-type: application/json' \
    -d "{\"status\":\"${status}\"}" >/dev/null
}

start_api() {
  APP_URL=http://127.0.0.1:4173 \
  HTTP_ADDRESS=:18080 \
  DATABASE_URL=postgresql://postgres:postgres@127.0.0.1:15432/onixggr?sslmode=disable \
  DATABASE_MAX_OPEN_CONNS=5 \
  DATABASE_MAX_IDLE_CONNS=5 \
  REDIS_URL=redis://127.0.0.1:16379 \
  NEXUSGGR_BASE_URL=http://127.0.0.1:18081 \
  NEXUSGGR_AGENT_CODE=demo-agent \
  NEXUSGGR_AGENT_TOKEN=demo-token \
  NEXUSGGR_TIMEOUT=200ms \
  QRIS_BASE_URL=http://127.0.0.1:18081 \
  QRIS_CLIENT=demo-client \
  QRIS_CLIENT_KEY=demo-key \
  QRIS_GLOBAL_UUID=demo-uuid \
  QRIS_WEBHOOK_SHARED_SECRET= \
  METRICS_ENABLED=true \
  PROMETHEUS_PORT=19090 \
  GOCACHE=/tmp/onixggr-go-build \
  go run ./apps/api >"${TMP_DIR}/api.log" 2>&1 &
  API_PID=$!

  wait_for_url "http://127.0.0.1:18080/health/live" "api live"
  wait_for_url "http://127.0.0.1:18080/health/ready" "api ready"
  wait_for_url "http://127.0.0.1:19090/metrics" "metrics"
}

start_worker() {
  if [[ -n "${WORKER_PID}" ]] && kill -0 "${WORKER_PID}" >/dev/null 2>&1; then
    return 0
  fi

  DATABASE_URL=postgresql://postgres:postgres@127.0.0.1:15432/onixggr?sslmode=disable \
  DATABASE_MAX_OPEN_CONNS=5 \
  DATABASE_MAX_IDLE_CONNS=5 \
  REDIS_URL=redis://127.0.0.1:16379 \
  NEXUSGGR_BASE_URL=http://127.0.0.1:18081 \
  NEXUSGGR_AGENT_CODE=demo-agent \
  NEXUSGGR_AGENT_TOKEN=demo-token \
  NEXUSGGR_TIMEOUT=200ms \
  QRIS_BASE_URL=http://127.0.0.1:18081 \
  QRIS_CLIENT=demo-client \
  QRIS_CLIENT_KEY=demo-key \
  QRIS_GLOBAL_UUID=demo-uuid \
  GAME_RECONCILE_INTERVAL=1s \
  QRIS_RECONCILE_INTERVAL=1s \
  WITHDRAW_RECONCILE_INTERVAL=1s \
  CALLBACK_RETRY_INTERVAL=1s \
  CALLBACK_DELIVERY_TIMEOUT=200ms \
  GOCACHE=/tmp/onixggr-go-build \
  ./appctl worker run >"${TMP_DIR}/worker.log" 2>&1 &
  WORKER_PID=$!
}

stop_worker() {
  if [[ -n "${WORKER_PID}" ]]; then
    kill "${WORKER_PID}" >/dev/null 2>&1 || true
    wait "${WORKER_PID}" >/dev/null 2>&1 || true
    WORKER_PID=""
  fi
}

bootstrap_stack() {
  cd "${ROOT_DIR}"
  free_listen_port 18080
  free_listen_port 18081
  free_listen_port 19090

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

  wait_for_url "http://127.0.0.1:18081/healthz" "mock upstream"
  start_api
}

redis_down_drill() {
  local status
  podman stop onixggr-redis >/dev/null
  sleep 3
  status="$(http_status GET http://127.0.0.1:18080/health/ready)"
  wait_for_metric_line '^onixggr_dependency_up\{dependency="redis"\} 0(\.0+)?$' 20
  mark_alert_ready "OnixggrRedisDown"

  podman start onixggr-redis >/dev/null
  wait_for_compose_dependencies
  wait_for_url "http://127.0.0.1:18080/health/ready" "api ready after redis restore"
  wait_for_metric_line '^onixggr_dependency_up\{dependency="redis"\} 1(\.0+)?$' 20

  printf 'redis_down ready_status=%s metric=redis_down recovered=yes\n' "${status}"
}

db_slow_drill() {
  local status
  podman pause onixggr-postgres >/dev/null
  sleep 3
  status="$(http_status GET http://127.0.0.1:18080/health/ready)"
  wait_for_metric_line '^onixggr_dependency_up\{dependency="postgres"\} 0(\.0+)?$' 20
  mark_alert_ready "OnixggrDatabaseDown"

  podman unpause onixggr-postgres >/dev/null
  wait_for_compose_dependencies
  wait_for_url "http://127.0.0.1:18080/health/ready" "api ready after postgres restore"
  wait_for_metric_line '^onixggr_dependency_up\{dependency="postgres"\} 1(\.0+)?$' 20

  printf 'db_slow_sim ready_status=%s metric=postgres_down recovered=yes\n' "${status}"
}

nexus_timeout_worker_down_drill() {
  local prefix="drill-timeout"
  set_mock_delay nexus 500

  for i in 1 2 3 4 5; do
    local body
    body="$(jq -nc --arg username "${GAME_MEMBER}" --arg trx_id "${prefix}-${i}" '{username:$username, amount:5000, trx_id:$trx_id}')"
    curl -fsS -o "${TMP_DIR}/nexus-timeout-${i}.json" -X POST \
      "http://127.0.0.1:18080/v1/store-api/game/deposits" \
      -H "Authorization: Bearer ${STORE_TOKEN}" \
      -H 'content-type: application/json' \
      -d "${body}" >/dev/null || true
  done

  local pending_before ledger_before timeout_count
  pending_before="$(sql "SELECT COUNT(*) FROM game_transactions WHERE trx_id LIKE '${prefix}-%' AND reconcile_status = 'pending_reconcile';" | tr -d '[:space:]')"
  ledger_before="$(sql "SELECT COUNT(*) FROM ledger_entries WHERE reference_type = 'game_transaction' AND reference_id IN (SELECT id FROM game_transactions WHERE trx_id LIKE '${prefix}-%');" | tr -d '[:space:]')"
  wait_for_metric_line '^onixggr_upstream_request_duration_seconds_count\{operation="user_deposit",provider="nexusggr",result="timeout"\} ([5-9]|[1-9][0-9]+)(\.0+)?$' 10
  mark_alert_ready "OnixggrNexusGGRErrorSpike"
  timeout_count="$(curl -fsS http://127.0.0.1:19090/metrics | awk '/onixggr_upstream_request_duration_seconds_count\{operation="user_deposit",provider="nexusggr",result="timeout"\}/ {print $2; exit}')"

  sleep 3
  local still_pending
  still_pending="$(sql "SELECT COUNT(*) FROM game_transactions WHERE trx_id LIKE '${prefix}-%' AND reconcile_status = 'pending_reconcile';" | tr -d '[:space:]')"

  set_mock_delay nexus 0
  start_worker
  wait_for_sql "SELECT COUNT(*) FROM game_transactions WHERE trx_id LIKE '${prefix}-%' AND status = 'success' AND reconcile_status = 'resolved';" "5" "game reconcile success"
  local ledger_after
  ledger_after="$(sql "SELECT COUNT(*) FROM ledger_entries WHERE reference_type = 'game_transaction' AND reference_id IN (SELECT id FROM game_transactions WHERE trx_id LIKE '${prefix}-%');" | tr -d '[:space:]')"
  sleep 3
  local ledger_after_second_check
  ledger_after_second_check="$(sql "SELECT COUNT(*) FROM ledger_entries WHERE reference_type = 'game_transaction' AND reference_id IN (SELECT id FROM game_transactions WHERE trx_id LIKE '${prefix}-%');" | tr -d '[:space:]')"
  stop_worker

  printf 'nexus_timeout pending_before=%s still_pending_while_worker_off=%s ledger_before=%s ledger_after=%s ledger_after_second_check=%s timeout_metric=%s\n' \
    "${pending_before}" "${still_pending}" "${ledger_before}" "${ledger_after}" "${ledger_after_second_check}" "${timeout_count}"
}

qris_webhook_missing_drill() {
  local custom_ref provider_trx_id qris_id ledger_before ledger_after

  curl -fsS -o "${TMP_DIR}/qris-missing.json" -X POST \
    "http://127.0.0.1:18080/v1/store-api/qris/member-payments" \
    -H "Authorization: Bearer ${STORE_TOKEN}" \
    -H 'content-type: application/json' \
    -d "$(jq -nc --arg username "${QRIS_MEMBER}" '{username:$username, amount:25000}')" >/dev/null

  custom_ref="$(json_field "${TMP_DIR}/qris-missing.json" '.data.custom_ref')"
  provider_trx_id="$(json_field "${TMP_DIR}/qris-missing.json" '.data.provider_trx_id')"
  qris_id="$(json_field "${TMP_DIR}/qris-missing.json" '.data.id')"
  ledger_before="$(sql "SELECT COUNT(*) FROM ledger_entries WHERE reference_type = 'qris_transaction' AND reference_id = '${qris_id}';" | tr -d '[:space:]')"

  set_qris_payment_status "${provider_trx_id}" success
  start_worker
  wait_for_sql "SELECT COUNT(*) FROM qris_transactions WHERE custom_ref = '${custom_ref}' AND status = 'success';" "1" "qris reconcile success"
  ledger_after="$(sql "SELECT COUNT(*) FROM ledger_entries WHERE reference_type = 'qris_transaction' AND reference_id = '${qris_id}';" | tr -d '[:space:]')"
  stop_worker

  printf 'qris_webhook_missing custom_ref=%s ledger_before=%s ledger_after=%s reconciled=yes\n' "${custom_ref}" "${ledger_before}" "${ledger_after}"
}

callback_owner_failing_drill() {
  sql "UPDATE stores SET callback_url = 'http://127.0.0.1:19999/fail', updated_at = now() WHERE id = '${STORE_ID}';"

  for i in 1 2 3; do
    curl -fsS -o "${TMP_DIR}/callback-${i}.json" -X POST \
      "http://127.0.0.1:18080/v1/store-api/qris/member-payments" \
      -H "Authorization: Bearer ${STORE_TOKEN}" \
      -H 'content-type: application/json' \
      -d "$(jq -nc --arg username "${QRIS_MEMBER}" '{username:$username, amount:12000}')" >/dev/null

    local amount provider_trx_id custom_ref
    amount="$(json_field "${TMP_DIR}/callback-${i}.json" '.data.amount_gross | tonumber')"
    provider_trx_id="$(json_field "${TMP_DIR}/callback-${i}.json" '.data.provider_trx_id')"
    custom_ref="$(json_field "${TMP_DIR}/callback-${i}.json" '.data.custom_ref')"

    curl -fsS -X POST "http://127.0.0.1:18080/v1/webhooks/qris" \
      -H 'content-type: application/json' \
      -d "$(jq -nc --argjson amount "${amount}" --arg trx_id "${provider_trx_id}" --arg custom_ref "${custom_ref}" --arg username "${QRIS_MEMBER}" '{amount:$amount, terminal_id:$username, trx_id:$trx_id, rrn:"drill-rrn", custom_ref:$custom_ref, vendor:"mock-qris", status:"success", create_at:"2026-03-31T06:00:00Z", finish_at:"2026-03-31T06:00:01Z"}')" >/dev/null
  done

  start_worker
  for _ in {1..20}; do
    sql "UPDATE outbound_callback_attempts SET next_retry_at = now() - interval '1 second' WHERE next_retry_at IS NOT NULL;"
    sleep 1
    local failed_count
    failed_count="$(sql "SELECT COUNT(*) FROM outbound_callbacks WHERE status = 'failed';" | tr -d '[:space:]')"
    if [[ "${failed_count}" -ge 3 ]]; then
      break
    fi
  done

  sleep 16
  wait_for_metric_line '^onixggr_recent_failures\{signal="callback"\} ([3-9]|[1-9][0-9]+)(\.0+)?$' 5
  mark_alert_ready "OnixggrCallbackFailureSpike"

  local failed notifications callback_metric
  failed="$(sql "SELECT COUNT(*) FROM outbound_callbacks WHERE status = 'failed';" | tr -d '[:space:]')"
  notifications="$(sql "SELECT COUNT(*) FROM notifications WHERE event_type = 'callback.delivery_failed' AND scope_type = 'store' AND scope_id = '${STORE_ID}';" | tr -d '[:space:]')"
  callback_metric="$(curl -fsS http://127.0.0.1:19090/metrics | awk '/onixggr_recent_failures\{signal="callback"\}/ {print $2; exit}')"
  stop_worker

  sql "UPDATE stores SET callback_url = '${ORIGINAL_CALLBACK_URL}', updated_at = now() WHERE id = '${STORE_ID}';"

  printf 'callback_owner_failing failed_callbacks=%s notifications=%s callback_metric=%s\n' "${failed}" "${notifications}" "${callback_metric}"
}

main() {
  bootstrap_stack

  redis_down_drill > >(tee "${TMP_DIR}/redis-down.txt")
  db_slow_drill > >(tee "${TMP_DIR}/db-slow.txt")
  nexus_timeout_worker_down_drill > >(tee "${TMP_DIR}/nexus-timeout-worker-down.txt")
  qris_webhook_missing_drill > >(tee "${TMP_DIR}/qris-webhook-missing.txt")
  callback_owner_failing_drill > >(tee "${TMP_DIR}/callback-owner-failing.txt")

  printf 'alerts_ready=%s\n' "$(IFS=,; echo "${ALERTS_READY[*]}")" | tee "${TMP_DIR}/alerts-ready.txt"

  echo "raw artifacts: ${TMP_DIR}"
}

main "$@"
