#!/usr/bin/env sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
ENV_FILE=${ENV_FILE:-"${SCRIPT_DIR}/env.production"}
STACK_FILE="${SCRIPT_DIR}/docker-compose.yml"
BACKUP_DIR="${SCRIPT_DIR}/backups"
TIMESTAMP=$(date +%Y%m%d%H%M%S)
BACKUP_FILE=${1:-"${BACKUP_DIR}/onixggr-production-${TIMESTAMP}.dump"}
RETENTION_DAYS=${PRODUCTION_BACKUP_RETENTION_DAYS:-14}

if [ ! -f "${ENV_FILE}" ]; then
  echo "missing ${ENV_FILE}; copy deploy/production/env.production.example first" >&2
  exit 1
fi

set -a
. "${ENV_FILE}"
set +a

mkdir -p "${BACKUP_DIR}"

podman compose --env-file "${ENV_FILE}" -f "${STACK_FILE}" exec -T postgres \
  sh -c "PGPASSWORD=\"${POSTGRES_PASSWORD}\" pg_dump -U \"${POSTGRES_USER}\" -d \"${POSTGRES_DB}\" -Fc" >"${BACKUP_FILE}"

find "${BACKUP_DIR}" -type f -name '*.dump' -mtime +"${RETENTION_DAYS}" -delete

printf 'backup_file=%s\n' "${BACKUP_FILE}"
