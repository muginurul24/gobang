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

podman compose --env-file "${ENV_FILE}" -f "${STACK_FILE}" down "$@"
