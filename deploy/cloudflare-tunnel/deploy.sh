#!/usr/bin/env sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
PRODUCTION_DIR=$(CDPATH= cd -- "${SCRIPT_DIR}/../production" && pwd)
ENV_FILE=${ENV_FILE:-"${PRODUCTION_DIR}/env.production"}
STACK_FILE="${PRODUCTION_DIR}/docker-compose.yml"
OVERRIDE_FILE="${SCRIPT_DIR}/docker-compose.override.yml"

if [ ! -f "${ENV_FILE}" ]; then
	echo "missing ${ENV_FILE}; copy deploy/production/env.production.example first" >&2
	exit 1
fi

set -a
. "${ENV_FILE}"
set +a

compose() {
	podman compose -f "${STACK_FILE}" -f "${OVERRIDE_FILE}" "$@"
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

require_loopback_port() {
	var_name="$1"
	value=$(read_var "${var_name}")
	case "${value}" in
		127.0.0.1:*)
			;;
		*)
			fail "env ${var_name} must bind to loopback, for example 127.0.0.1:18080"
			;;
	esac
}

local_http_port() {
	printf '%s' "${PRODUCTION_HTTP_PORT##*:}"
}

preflight() {
	require_value COMPOSE_PROJECT_NAME
	require_value PRODUCTION_DOMAIN
	require_value APP_URL
	require_value APP_ENV
	require_value POSTGRES_DB
	require_value POSTGRES_USER
	require_value POSTGRES_PASSWORD
	require_value REDIS_PASSWORD
	require_value PRODUCTION_HTTP_PORT
	require_value PRODUCTION_HTTPS_PORT
	require_loopback_port PRODUCTION_HTTP_PORT
	require_loopback_port PRODUCTION_HTTPS_PORT

	if [ "${APP_ENV}" != "production" ]; then
		fail "APP_ENV must be production"
	fi

	case "${APP_URL}" in
		https://*)
			;;
		*)
			fail "APP_URL must start with https://"
			;;
	esac
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

wait_for_local_proxy() {
	http_port=$(local_http_port)

	for _ in $(seq 1 60); do
		if curl -fsS "http://127.0.0.1:${http_port}/health/live" >/dev/null 2>&1; then
			return 0
		fi
		sleep 2
	done

	fail "cloudflare tunnel origin proxy failed to become ready on 127.0.0.1:${http_port}"
}

preflight
compose up -d --build postgres redis
wait_for_postgres
wait_for_redis
compose build manage
compose run --rm -T manage migrate up
compose up -d --build api worker scheduler web proxy
wait_for_local_proxy

printf '%s\n' \
	"cloudflare tunnel origin deploy complete" \
	"local_origin=http://127.0.0.1:$(local_http_port)" \
	"health=http://127.0.0.1:$(local_http_port)/health/ready" \
	"next_step=create Cloudflare Tunnel public hostname ${PRODUCTION_DOMAIN} -> http://127.0.0.1:$(local_http_port)"
