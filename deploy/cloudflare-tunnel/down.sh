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

podman compose -f "${STACK_FILE}" -f "${OVERRIDE_FILE}" down "$@"
