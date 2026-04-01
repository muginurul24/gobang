#!/usr/bin/env sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
ENV_FILE=${ENV_FILE:-"${SCRIPT_DIR}/env.production"}
STACK_FILE="docker-compose.yml"
TUNNEL_OVERRIDE_FILE="../cloudflare-tunnel/docker-compose.override.yml"
COMPOSE_RUNTIME_ENV="${SCRIPT_DIR}/.env"
COMPOSE_RUNTIME_ENV_BACKUP=
BACKUP_SUMMARY=
PREVIOUS_STACK_RUNNING=0

if [ ! -f "${ENV_FILE}" ]; then
	echo "missing ${ENV_FILE}; copy deploy/production/env.production.example first" >&2
	exit 1
fi

set -a
. "${ENV_FILE}"
set +a

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

require_https_url() {
	var_name="$1"
	value=$(read_var "${var_name}")
	case "${value}" in
		https://*)
			;;
		*)
			fail "env ${var_name} must start with https://"
			;;
	esac
}

is_tunnel_mode() {
	case "${PRODUCTION_HTTP_PORT:-}:${PRODUCTION_HTTPS_PORT:-}" in
		127.0.0.1:*:127.0.0.1:*)
			return 0
			;;
		*)
			return 1
			;;
	esac
}

compose() {
	(
		cd "${SCRIPT_DIR}"
		if is_tunnel_mode; then
			podman compose -f "${STACK_FILE}" -f "${TUNNEL_OVERRIDE_FILE}" "$@"
		else
			podman compose -f "${STACK_FILE}" "$@"
		fi
	)
}

prepare_compose_runtime_env() {
	if [ -f "${COMPOSE_RUNTIME_ENV}" ]; then
		COMPOSE_RUNTIME_ENV_BACKUP="${SCRIPT_DIR}/.env.codex-backup.$$"
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

local_http_port() {
	printf '%s' "${PRODUCTION_HTTP_PORT##*:}"
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

	if [ "${APP_ENV}" != "production" ]; then
		fail "APP_ENV must be production"
	fi

	require_https_url APP_URL
	require_not_placeholder APP_URL
	require_not_placeholder PRODUCTION_DOMAIN
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

	if ! is_tunnel_mode; then
		require_value TLS_EMAIL
		require_not_placeholder TLS_EMAIL
		require_value METRICS_ENABLED
		if [ "${METRICS_ENABLED}" != "true" ]; then
			fail "METRICS_ENABLED must stay true in production"
		fi
	fi

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

service_container_names() {
	service_name="$1"
	container_name="${COMPOSE_PROJECT_NAME}_${service_name}_1"
	if podman container exists "${container_name}" >/dev/null 2>&1; then
		printf '%s\n' "${container_name}"
	fi
}

ensure_service_started() {
	service_name="$1"
	container_name="${COMPOSE_PROJECT_NAME}_${service_name}_1"

	if podman container exists "${container_name}" >/dev/null 2>&1; then
		podman start "${container_name}" >/dev/null 2>&1 || true
		return 0
	fi

	compose up -d "${service_name}"
}

service_running() {
	service_name="$1"
	podman ps \
		--filter "label=com.docker.compose.project=${COMPOSE_PROJECT_NAME}" \
		--filter "label=com.docker.compose.service=${service_name}" \
		--format '{{.Names}}' | grep -q .
}

remove_service_containers() {
	service_name="$1"

	for container_name in $(service_container_names "${service_name}"); do
		podman rm -f "${container_name}" >/dev/null 2>&1 || true
	done
}

recreate_services() {
	for service_name in "$@"; do
		remove_service_containers "${service_name}"
	done
}

run_manage_migrations() {
	container_name="${COMPOSE_PROJECT_NAME}_manage_1"
	compose up -d --no-deps manage >/dev/null

	for _ in $(seq 1 120); do
		state=$(podman inspect -f '{{.State.Status}}' "${container_name}" 2>/dev/null || printf 'missing')
		case "${state}" in
			exited)
				exit_code=$(podman inspect -f '{{.State.ExitCode}}' "${container_name}" 2>/dev/null || printf '1')
				if [ "${exit_code}" = "0" ]; then
					return 0
				fi

				podman logs --tail 120 "${container_name}" >&2 || true
				fail "manage migration container exited with code ${exit_code}"
				;;
			running|configured|created)
				sleep 1
				;;
			*)
				sleep 1
				;;
		esac
	done

	podman logs --tail 120 "${container_name}" >&2 || true
	fail "manage migration container did not finish in time"
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

	fail "production origin failed to become live on 127.0.0.1:${http_port}"
}

wait_for_local_proxy_ready() {
	http_port=$(local_http_port)

	for _ in $(seq 1 60); do
		if curl -fsS "http://127.0.0.1:${http_port}/health/ready" >/dev/null 2>&1; then
			return 0
		fi
		sleep 2
	done

	fail "production origin is reachable but not ready on 127.0.0.1:${http_port}"
}

wait_for_public_proxy() {
	target=$(base_url)
	for _ in $(seq 1 60); do
		if curl -ksSf --resolve "${PRODUCTION_DOMAIN}:${PRODUCTION_HTTPS_PORT:-443}:127.0.0.1" \
			"${target}/health/ready" >/dev/null 2>&1; then
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
prepare_compose_runtime_env
trap cleanup_compose_runtime_env EXIT INT TERM

if podman container exists "${COMPOSE_PROJECT_NAME}_postgres_1"; then
	PREVIOUS_STACK_RUNNING=1
fi

log_step 1 "starting postgres and redis"
ensure_service_started postgres
ensure_service_started redis
wait_for_postgres
wait_for_redis

pre_deploy_backup

log_step 2 "building manage image"
compose build manage

log_step 3 "running migrations"
recreate_services manage
run_manage_migrations

log_step 4 "building api worker scheduler web"
compose build api worker scheduler web

log_step 5 "starting api worker scheduler web proxy"
recreate_services proxy web scheduler worker api
compose up -d api worker scheduler web proxy
wait_for_service_running api
wait_for_service_running web
wait_for_service_running worker
wait_for_service_running scheduler
wait_for_service_running proxy

if is_tunnel_mode; then
	log_step 6 "waiting for origin live health"
	wait_for_local_proxy_live
	log_step 7 "waiting for origin ready health"
	wait_for_local_proxy_ready

	printf '%s\n' \
		"production deploy complete" \
		"mode=cloudflare_tunnel" \
		"local_origin=http://127.0.0.1:$(local_http_port)" \
		"health=http://127.0.0.1:$(local_http_port)/health/ready"
else
	log_step 6 "waiting for public-ready proxy"
	wait_for_public_proxy

	printf '%s\n' \
		"production deploy complete" \
		"mode=direct_tls" \
		"proxy=$(base_url)" \
		"health=$(base_url)/health/ready"
fi

if [ -n "${BACKUP_SUMMARY}" ]; then
	printf '%s\n' "${BACKUP_SUMMARY}"
fi
