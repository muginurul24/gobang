#!/usr/bin/env sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
ENV_FILE=${ENV_FILE:-"${SCRIPT_DIR}/env.production"}
STACK_FILE="${SCRIPT_DIR}/docker-compose.yml"
BACKUP_SUMMARY=
PREVIOUS_STACK_RUNNING=0

if [ ! -f "${ENV_FILE}" ]; then
  echo "missing ${ENV_FILE}; copy deploy/production/env.production.example first" >&2
  exit 1
fi

set -a
. "${ENV_FILE}"
set +a

compose() {
  podman compose --env-file "${ENV_FILE}" -f "${STACK_FILE}" "$@"
}

read_var() {
  var_name="$1"
  eval "printf '%s' \"\${${var_name}:-}\""
}

fail() {
  echo "$1" >&2
  exit 1
}

require_value() {
  var_name="$1"
  value=$(read_var "${var_name}")
  if [ -z "${value}" ]; then
    fail "missing required env ${var_name}"
  fi
}

require_not_placeholder() {
  var_name="$1"
  value=$(read_var "${var_name}")
  case "${value}" in
    ''|change-me-*|demo-*|*example.com|*provider.example)
      fail "env ${var_name} must be rotated from placeholder value"
      ;;
  esac
}

require_no_fragment() {
  var_name="$1"
  fragment="$2"
  value=$(read_var "${var_name}")
  case "${value}" in
    *"${fragment}"*)
      fail "env ${var_name} must not contain ${fragment}"
      ;;
  esac
}

require_https_url() {
  var_name="$1"
  value=$(read_var "${var_name}")
  case "${value}" in
    https://*) ;;
    *)
      fail "env ${var_name} must start with https://"
      ;;
  esac
}

preflight() {
  require_value COMPOSE_PROJECT_NAME
  require_value PRODUCTION_DOMAIN
  require_value APP_URL
  require_value TLS_EMAIL
  require_value APP_ENV
  require_value METRICS_ENABLED

  if [ "${APP_ENV}" != "production" ]; then
    fail "APP_ENV must be production"
  fi

  if [ "${METRICS_ENABLED}" != "true" ]; then
    fail "METRICS_ENABLED must stay true in production"
  fi

  require_https_url APP_URL
  require_not_placeholder APP_URL
  require_not_placeholder PRODUCTION_DOMAIN
  require_not_placeholder TLS_EMAIL
  require_not_placeholder POSTGRES_PASSWORD
  require_not_placeholder REDIS_PASSWORD
  require_not_placeholder JWT_ACCESS_SECRET
  require_not_placeholder AUTH_ENCRYPTION_KEY
  require_not_placeholder CALLBACK_SIGNING_SECRET
  require_not_placeholder QRIS_CLIENT
  require_not_placeholder QRIS_CLIENT_KEY
  require_not_placeholder QRIS_GLOBAL_UUID
  require_not_placeholder QRIS_WEBHOOK_SHARED_SECRET
  require_not_placeholder QRIS_BASE_URL
  require_not_placeholder NEXUSGGR_AGENT_CODE
  require_not_placeholder NEXUSGGR_AGENT_TOKEN
  require_not_placeholder NEXUSGGR_BASE_URL

  require_no_fragment PRODUCTION_DOMAIN localhost
  require_no_fragment PRODUCTION_DOMAIN 127.0.0.1
  require_no_fragment APP_URL localhost
  require_no_fragment APP_URL 127.0.0.1
  require_no_fragment QRIS_BASE_URL mock-upstreams
  require_no_fragment QRIS_BASE_URL localhost
  require_no_fragment QRIS_BASE_URL 127.0.0.1
  require_no_fragment NEXUSGGR_BASE_URL mock-upstreams
  require_no_fragment NEXUSGGR_BASE_URL localhost
  require_no_fragment NEXUSGGR_BASE_URL 127.0.0.1
}

wait_for_postgres() {
  for _ in $(seq 1 60); do
    if compose exec -T postgres pg_isready -U "${POSTGRES_USER}" -d "${POSTGRES_DB}" >/dev/null 2>&1; then
      return 0
    fi
    sleep 2
  done

  fail "postgres failed to become ready"
}

wait_for_redis() {
  for _ in $(seq 1 60); do
    if compose exec -T redis redis-cli -a "${REDIS_PASSWORD}" ping >/dev/null 2>&1; then
      return 0
    fi
    sleep 2
  done

  fail "redis failed to become ready"
}

base_url() {
  case "${PRODUCTION_HTTPS_PORT:-443}" in
    ''|443)
      printf '%s' "${APP_URL}"
      ;;
    *)
      printf 'https://%s:%s' "${PRODUCTION_DOMAIN}" "${PRODUCTION_HTTPS_PORT}"
      ;;
  esac
}

wait_for_proxy() {
  target=$(base_url)
  for _ in $(seq 1 60); do
    if curl -ksSf --resolve "${PRODUCTION_DOMAIN}:${PRODUCTION_HTTPS_PORT:-443}:127.0.0.1" \
      "${target}/health/live" >/dev/null 2>&1; then
      return 0
    fi
    sleep 2
  done

  fail "production proxy failed to become ready"
}

pre_deploy_backup() {
  if [ "${PRODUCTION_REQUIRE_BACKUP_BEFORE_MIGRATE:-1}" != "1" ]; then
    return 0
  fi

  if [ "${PREVIOUS_STACK_RUNNING}" = "1" ]; then
    BACKUP_SUMMARY=$("${SCRIPT_DIR}/backup-db.sh")
  fi
}

preflight
if podman container exists "${COMPOSE_PROJECT_NAME}_postgres_1"; then
  PREVIOUS_STACK_RUNNING=1
fi
compose up -d --build postgres redis
wait_for_postgres
wait_for_redis
pre_deploy_backup
compose build manage
compose run --rm -T manage migrate up
compose up -d --build api worker scheduler web proxy
wait_for_proxy

printf '%s\n' \
  "production deploy complete" \
  "proxy: $(base_url)" \
  "health: $(base_url)/health/ready"

if [ -n "${BACKUP_SUMMARY}" ]; then
  printf '%s\n' "${BACKUP_SUMMARY}"
fi
