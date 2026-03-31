#!/usr/bin/env sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
ENV_FILE=${ENV_FILE:-"${SCRIPT_DIR}/env.staging"}
STACK_FILE="${SCRIPT_DIR}/docker-compose.yml"

if [ ! -f "${ENV_FILE}" ]; then
  echo "missing ${ENV_FILE}; copy deploy/staging/env.staging.example first" >&2
  exit 1
fi

set -a
. "${ENV_FILE}"
set +a

compose() {
  podman compose --env-file "${ENV_FILE}" -f "${STACK_FILE}" "$@"
}

wait_for_postgres() {
  for _ in $(seq 1 60); do
    if compose exec -T postgres pg_isready -U "${POSTGRES_USER}" -d "${POSTGRES_DB}" >/dev/null 2>&1; then
      return 0
    fi
    sleep 2
  done

  echo "postgres failed to become ready" >&2
  return 1
}

wait_for_redis() {
  for _ in $(seq 1 60); do
    if compose exec -T redis redis-cli ping >/dev/null 2>&1; then
      return 0
    fi
    sleep 2
  done

  echo "redis failed to become ready" >&2
  return 1
}

wait_for_proxy() {
  for _ in $(seq 1 60); do
    if curl -ksSf --resolve "${STAGING_DOMAIN}:${STAGING_HTTPS_PORT}:127.0.0.1" \
      "https://${STAGING_DOMAIN}:${STAGING_HTTPS_PORT}/health/live" >/dev/null 2>&1; then
      return 0
    fi
    sleep 2
  done

  echo "staging proxy failed to become ready" >&2
  return 1
}

EXTRA_SERVICES=
if [ "${STAGING_ENABLE_MOCK_UPSTREAMS:-0}" = "1" ]; then
  EXTRA_SERVICES="mock-upstreams"
fi

compose up -d --build postgres redis ${EXTRA_SERVICES}
wait_for_postgres
wait_for_redis

compose build manage
compose run --rm -T manage migrate up
if [ "${STAGING_LOAD_DEMO_DATA:-0}" = "1" ]; then
  compose run --rm -T manage seed demo
fi

compose up -d --build api worker scheduler web proxy ${EXTRA_SERVICES}
wait_for_proxy

printf '%s\n' \
  "staging deploy complete" \
  "proxy: https://${STAGING_DOMAIN}:${STAGING_HTTPS_PORT}" \
  "health: https://${STAGING_DOMAIN}:${STAGING_HTTPS_PORT}/health/ready"
