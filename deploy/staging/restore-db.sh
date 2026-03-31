#!/usr/bin/env sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
ENV_FILE=${ENV_FILE:-"${SCRIPT_DIR}/env.staging"}
STACK_FILE="${SCRIPT_DIR}/docker-compose.yml"
BACKUP_FILE=${1:-}
RESTORE_DB=${RESTORE_DB:-onixggr_restore_check}

if [ -z "${BACKUP_FILE}" ]; then
  echo "usage: ${0} <backup-file>" >&2
  exit 1
fi

if [ ! -f "${BACKUP_FILE}" ]; then
  echo "backup file not found: ${BACKUP_FILE}" >&2
  exit 1
fi

if [ ! -f "${ENV_FILE}" ]; then
  echo "missing ${ENV_FILE}; copy deploy/staging/env.staging.example first" >&2
  exit 1
fi

set -a
. "${ENV_FILE}"
set +a

compose() {
  podman compose --env-file "${ENV_FILE}" -f "${STACK_FILE}" "$@"
}

compose exec -T postgres sh -c \
  "PGPASSWORD=\"${POSTGRES_PASSWORD}\" psql -U \"${POSTGRES_USER}\" -d postgres -v ON_ERROR_STOP=1 \
    -c 'DROP DATABASE IF EXISTS \"${RESTORE_DB}\";' \
    -c 'CREATE DATABASE \"${RESTORE_DB}\";'"

compose exec -T postgres sh -c \
  "PGPASSWORD=\"${POSTGRES_PASSWORD}\" pg_restore -U \"${POSTGRES_USER}\" -d \"${RESTORE_DB}\" --clean --if-exists --no-owner --no-privileges" <"${BACKUP_FILE}"

USERS_COUNT=$(compose exec -T postgres sh -c \
  "PGPASSWORD=\"${POSTGRES_PASSWORD}\" psql -U \"${POSTGRES_USER}\" -d \"${RESTORE_DB}\" -Atqc 'SELECT COUNT(*) FROM users;'" | tr -d '[:space:]')
STORES_COUNT=$(compose exec -T postgres sh -c \
  "PGPASSWORD=\"${POSTGRES_PASSWORD}\" psql -U \"${POSTGRES_USER}\" -d \"${RESTORE_DB}\" -Atqc 'SELECT COUNT(*) FROM stores;'" | tr -d '[:space:]')

compose exec -T postgres sh -c \
  "PGPASSWORD=\"${POSTGRES_PASSWORD}\" psql -U \"${POSTGRES_USER}\" -d postgres -v ON_ERROR_STOP=1 \
    -c 'DROP DATABASE IF EXISTS \"${RESTORE_DB}\";'"

printf 'restore_db=%s users=%s stores=%s\n' "${RESTORE_DB}" "${USERS_COUNT}" "${STORES_COUNT}"
