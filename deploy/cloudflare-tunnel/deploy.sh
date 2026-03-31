#!/usr/bin/env sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
PRODUCTION_DIR=$(CDPATH= cd -- "${SCRIPT_DIR}/../production" && pwd)
ENV_FILE=${ENV_FILE:-"${PRODUCTION_DIR}/env.production"}
STACK_FILE="docker-compose.yml"
OVERRIDE_FILE="../cloudflare-tunnel/docker-compose.override.yml"
COMPOSE_RUNTIME_ENV="${PRODUCTION_DIR}/.env"
COMPOSE_RUNTIME_ENV_BACKUP=

if [ ! -f "${ENV_FILE}" ]; then
	echo "missing ${ENV_FILE}; copy deploy/production/env.production.example first" >&2
	exit 1
fi

set -a
. "${ENV_FILE}"
set +a

compose() {
	(
		cd "${PRODUCTION_DIR}"
		podman compose -f "${STACK_FILE}" -f "${OVERRIDE_FILE}" "$@"
	)
}

read_var() {
	var_name="$1"
	eval "printf '%s' \"\${${var_name}:-}\""
}

fail() {
	echo "$1" >&2
	exit 1
}

log_step() {
	printf '[%s] %s\n' "$1" "$2"
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

prepare_compose_runtime_env() {
	if [ -f "${COMPOSE_RUNTIME_ENV}" ]; then
		COMPOSE_RUNTIME_ENV_BACKUP="${PRODUCTION_DIR}/.env.codex-backup.$$"
		cp "${COMPOSE_RUNTIME_ENV}" "${COMPOSE_RUNTIME_ENV_BACKUP}"
	fi

	cp "${ENV_FILE}" "${COMPOSE_RUNTIME_ENV}"
	chmod 600 "${COMPOSE_RUNTIME_ENV}" >/dev/null 2>&1 || true
}

cleanup_compose_runtime_env() {
	if [ -n "${COMPOSE_RUNTIME_ENV_BACKUP}" ] && [ -f "${COMPOSE_RUNTIME_ENV_BACKUP}" ]; then
		mv "${COMPOSE_RUNTIME_ENV_BACKUP}" "${COMPOSE_RUNTIME_ENV}"
		return
	fi

	rm -f "${COMPOSE_RUNTIME_ENV}"
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
	require_value JWT_ACCESS_SECRET
	require_value AUTH_ENCRYPTION_KEY
	require_value CALLBACK_SIGNING_SECRET
	require_value QRIS_BASE_URL
	require_value QRIS_CLIENT
	require_value QRIS_CLIENT_KEY
	require_value QRIS_GLOBAL_UUID
	require_value QRIS_WEBHOOK_SHARED_SECRET
	require_value NEXUSGGR_BASE_URL
	require_value NEXUSGGR_AGENT_CODE
	require_value NEXUSGGR_AGENT_TOKEN
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

	require_not_placeholder POSTGRES_PASSWORD
	require_not_placeholder REDIS_PASSWORD
	require_not_placeholder JWT_ACCESS_SECRET
	require_not_placeholder AUTH_ENCRYPTION_KEY
	require_not_placeholder CALLBACK_SIGNING_SECRET
	require_not_placeholder QRIS_BASE_URL
	require_not_placeholder QRIS_CLIENT
	require_not_placeholder QRIS_CLIENT_KEY
	require_not_placeholder QRIS_GLOBAL_UUID
	require_not_placeholder QRIS_WEBHOOK_SHARED_SECRET
	require_not_placeholder NEXUSGGR_BASE_URL
	require_not_placeholder NEXUSGGR_AGENT_CODE
	require_not_placeholder NEXUSGGR_AGENT_TOKEN

	require_no_fragment PRODUCTION_DOMAIN localhost
	require_no_fragment PRODUCTION_DOMAIN 127.0.0.1
	require_no_fragment APP_URL localhost
	require_no_fragment APP_URL 127.0.0.1
	require_no_fragment QRIS_BASE_URL localhost
	require_no_fragment QRIS_BASE_URL 127.0.0.1
	require_no_fragment QRIS_BASE_URL mock-upstreams
	require_no_fragment NEXUSGGR_BASE_URL localhost
	require_no_fragment NEXUSGGR_BASE_URL 127.0.0.1
	require_no_fragment NEXUSGGR_BASE_URL mock-upstreams
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

service_container_names() {
	service_name="$1"
	podman ps -a \
		--filter "label=com.docker.compose.project=${COMPOSE_PROJECT_NAME}" \
		--filter "label=com.docker.compose.service=${service_name}" \
		--format '{{.Names}}'
}

service_running() {
	service_name="$1"
	podman ps \
		--filter "label=com.docker.compose.project=${COMPOSE_PROJECT_NAME}" \
		--filter "label=com.docker.compose.service=${service_name}" \
		--format '{{.Names}}' | grep -q .
}

print_service_diagnostics() {
	service_name="$1"
	printf 'service=%s\n' "${service_name}" >&2
	podman ps -a \
		--filter "label=com.docker.compose.project=${COMPOSE_PROJECT_NAME}" \
		--filter "label=com.docker.compose.service=${service_name}" \
		--format 'table {{.Names}}\t{{.Status}}' >&2 || true

	for container_name in $(service_container_names "${service_name}"); do
		printf '\nlogs(%s):\n' "${container_name}" >&2
		podman logs --tail 80 "${container_name}" >&2 || true
	done
}

wait_for_service_running() {
	service_name="$1"
	for _ in $(seq 1 60); do
		if service_running "${service_name}"; then
			return 0
		fi
		sleep 2
	done

	print_service_diagnostics "${service_name}"
	fail "service ${service_name} failed to stay running"
}

wait_for_local_proxy_live() {
	http_port=$(local_http_port)

	for _ in $(seq 1 60); do
		if curl -fsS "http://127.0.0.1:${http_port}/health/live" >/dev/null 2>&1; then
			return 0
		fi
		sleep 2
	done

	fail "cloudflare tunnel origin proxy failed to become ready on 127.0.0.1:${http_port}"
}

wait_for_local_proxy_ready() {
	http_port=$(local_http_port)

	for _ in $(seq 1 60); do
		if curl -fsS "http://127.0.0.1:${http_port}/health/ready" >/dev/null 2>&1; then
			return 0
		fi
		sleep 2
	done

	fail "cloudflare tunnel origin proxy is reachable but not ready on 127.0.0.1:${http_port}"
}

preflight
prepare_compose_runtime_env
trap cleanup_compose_runtime_env EXIT INT TERM

log_step 1 "starting postgres and redis"
compose up -d --build postgres redis
wait_for_postgres
wait_for_redis

log_step 2 "building manage image"
compose build manage

log_step 3 "running migrations"
compose up --no-deps manage

log_step 4 "starting api worker scheduler web proxy"
compose up -d --build api worker scheduler web proxy
wait_for_service_running api
wait_for_service_running web
wait_for_service_running worker
wait_for_service_running scheduler
wait_for_service_running proxy

log_step 5 "waiting for origin live health"
wait_for_local_proxy_live

log_step 6 "waiting for origin ready health"
wait_for_local_proxy_ready

printf '%s\n' \
	"cloudflare tunnel origin deploy complete" \
	"local_origin=http://127.0.0.1:$(local_http_port)" \
	"health=http://127.0.0.1:$(local_http_port)/health/ready" \
	"next_step=create Cloudflare Tunnel public hostname ${PRODUCTION_DOMAIN} -> http://127.0.0.1:$(local_http_port)"
