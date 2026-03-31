#!/usr/bin/env sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
ENV_FILE=${ENV_FILE:-"${SCRIPT_DIR}/env.staging"}
STACK_FILE="${SCRIPT_DIR}/docker-compose.yml"
BACKUP_DIR="${SCRIPT_DIR}/backups"
TIMESTAMP=$(date +%Y%m%d%H%M%S)
BACKUP_FILE=${1:-"${BACKUP_DIR}/onixggr-staging-${TIMESTAMP}.dump"}

if [ ! -f "${ENV_FILE}" ]; then
  echo "missing ${ENV_FILE}; copy deploy/staging/env.staging.example first" >&2
  exit 1
fi

set -a
. "${ENV_FILE}"
set +a

mkdir -p "${BACKUP_DIR}"

podman compose --env-file "${ENV_FILE}" -f "${STACK_FILE}" exec -T postgres \
  sh -c "PGPASSWORD=\"${POSTGRES_PASSWORD}\" pg_dump -U \"${POSTGRES_USER}\" -d \"${POSTGRES_DB}\" -Fc" >"${BACKUP_FILE}"

printf 'backup_file=%s\n' "${BACKUP_FILE}"
