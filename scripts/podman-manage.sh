#!/usr/bin/env sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
ROOT_DIR=$(CDPATH= cd -- "${SCRIPT_DIR}/.." && pwd)
ENV_FILE=${ENV_FILE:-"${ROOT_DIR}/.env"}
POSTGRES_CONTAINER=${POSTGRES_CONTAINER:-onixggr-postgres}
REDIS_CONTAINER=${REDIS_CONTAINER:-onixggr-redis}
API_CONTAINER=${API_CONTAINER:-onixggr-api}
POSTGRES_DB=${POSTGRES_DB:-onixggr}
POSTGRES_USER=${POSTGRES_USER:-postgres}

if [ $# -eq 0 ]; then
  echo "usage: ./scripts/podman-manage.sh <manage-args...>" >&2
  exit 1
fi

if [ -f "${ENV_FILE}" ]; then
  set -a
  . "${ENV_FILE}"
  set +a
fi

wait_for_postgres() {
  for _ in $(seq 1 60); do
    if podman exec "${POSTGRES_CONTAINER}" pg_isready -U "${POSTGRES_USER}" -d "${POSTGRES_DB}" >/dev/null 2>&1; then
      return 0
    fi
    sleep 2
  done

  echo "postgres failed to become ready" >&2
  exit 1
}

wait_for_redis() {
  for _ in $(seq 1 60); do
    if podman exec "${REDIS_CONTAINER}" redis-cli ping >/dev/null 2>&1; then
      return 0
    fi
    sleep 2
  done

  echo "redis failed to become ready" >&2
  exit 1
}

wait_for_api_container() {
  for _ in $(seq 1 60); do
    if podman ps --format '{{.Names}}' | grep -Fx "${API_CONTAINER}" >/dev/null 2>&1; then
      return 0
    fi
    sleep 2
  done

  echo "api container failed to become ready" >&2
  exit 1
}

if ! podman container exists "${POSTGRES_CONTAINER}" >/dev/null 2>&1 \
  || ! podman container exists "${REDIS_CONTAINER}" >/dev/null 2>&1 \
  || ! podman container exists "${API_CONTAINER}" >/dev/null 2>&1; then
  "${SCRIPT_DIR}/podman-up.sh" >/dev/null
fi

podman start "${POSTGRES_CONTAINER}" >/dev/null 2>&1 || true
podman start "${REDIS_CONTAINER}" >/dev/null 2>&1 || true
podman start "${API_CONTAINER}" >/dev/null 2>&1 || true

wait_for_postgres
wait_for_redis
wait_for_api_container

podman exec -i \
  -w /workspace \
  "${API_CONTAINER}" \
  env GOCACHE=/tmp/onixggr-go-build go run ./apps/manage "$@"
