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

if [ -n "${ARGS}" ]; then
	# shellcheck disable=SC2086
	podman compose -f "${STACK_FILE}" -f "${OVERRIDE_FILE}" down ${ARGS}
else
	podman compose -f "${STACK_FILE}" -f "${OVERRIDE_FILE}" down
fi
