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

ARGS=
for arg in "$@"; do
	case "${arg}" in
		--remove-orphans)
			echo "ignoring --remove-orphans because podman-compose 1.0.6 does not support it safely" >&2
			;;
		*)
			if [ -n "${ARGS}" ]; then
				ARGS="${ARGS} ${arg}"
			else
				ARGS="${arg}"
			fi
			;;
	esac
done

prepare_compose_runtime_env
trap cleanup_compose_runtime_env EXIT INT TERM

if [ -n "${ARGS}" ]; then
	# shellcheck disable=SC2086
	(
		cd "${PRODUCTION_DIR}"
		podman compose -f "${STACK_FILE}" -f "${OVERRIDE_FILE}" down ${ARGS}
	)
else
	(
		cd "${PRODUCTION_DIR}"
		podman compose -f "${STACK_FILE}" -f "${OVERRIDE_FILE}" down
	)
fi
