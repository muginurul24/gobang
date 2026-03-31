#!/usr/bin/env sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
ENV_FILE=${ENV_FILE:-"${SCRIPT_DIR}/env.production"}

if [ ! -f "${ENV_FILE}" ]; then
  echo "missing ${ENV_FILE}; copy deploy/production/env.production.example first" >&2
  exit 1
fi

set -a
. "${ENV_FILE}"
set +a

OWNER_LOGIN=${SMOKE_OWNER_LOGIN:-}
OWNER_PASSWORD=${SMOKE_OWNER_PASSWORD:-}
STORE_ID=${SMOKE_STORE_ID:-}
STORE_TOKEN=${SMOKE_STORE_TOKEN:-}
STORE_MEMBER=${SMOKE_STORE_MEMBER:-}
BASE_URL=${SMOKE_BASE_URL:-${APP_URL}}

if [ -z "${OWNER_LOGIN}" ] || [ -z "${OWNER_PASSWORD}" ] || [ -z "${STORE_ID}" ] || [ -z "${STORE_TOKEN}" ] || [ -z "${STORE_MEMBER}" ]; then
  echo "SMOKE_OWNER_LOGIN, SMOKE_OWNER_PASSWORD, SMOKE_STORE_ID, SMOKE_STORE_TOKEN, and SMOKE_STORE_MEMBER must be set" >&2
  exit 1
fi

curl_json() {
  method="$1"
  path="$2"
  body="${3:-}"
  shift 3 || true

  if [ "${SMOKE_RESOLVE_LOOPBACK:-0}" = "1" ]; then
    resolve_args="--resolve ${PRODUCTION_DOMAIN}:${PRODUCTION_HTTPS_PORT:-443}:127.0.0.1"
  else
    resolve_args=
  fi

  if [ -n "${body}" ]; then
    curl -ksSf ${resolve_args} -X "${method}" "${BASE_URL}${path}" \
      -H 'content-type: application/json' "$@" -d "${body}"
  else
    curl -ksSf ${resolve_args} -X "${method}" "${BASE_URL}${path}" "$@"
  fi
}

curl_json GET /health/live '' >/dev/null
curl_json GET /health/ready '' >/dev/null
curl_json GET / '' >/dev/null

LOGIN_JSON=$(curl_json POST /v1/auth/login \
  "$(jq -nc --arg login "${OWNER_LOGIN}" --arg password "${OWNER_PASSWORD}" '{login:$login,password:$password}')")
ACCESS_TOKEN=$(printf '%s' "${LOGIN_JSON}" | jq -r '.data.access_token')

if [ -z "${ACCESS_TOKEN}" ] || [ "${ACCESS_TOKEN}" = "null" ]; then
  echo "login failed: ${LOGIN_JSON}" >&2
  exit 1
fi

STORES_JSON=$(curl_json GET /v1/stores '' -H "Authorization: Bearer ${ACCESS_TOKEN}")
PROVIDERS_JSON=$(curl_json GET /v1/catalog/providers '' -H "Authorization: Bearer ${ACCESS_TOKEN}")
MEMBERS_JSON=$(curl_json GET "/v1/stores/${STORE_ID}/members" '' -H "Authorization: Bearer ${ACCESS_TOKEN}")
BALANCE_JSON=$(curl_json GET "/v1/store-api/game/balance?username=${STORE_MEMBER}" '' -H "Authorization: Bearer ${STORE_TOKEN}")

STORES_COUNT=$(printf '%s' "${STORES_JSON}" | jq -r '.data | length')
PROVIDERS_COUNT=$(printf '%s' "${PROVIDERS_JSON}" | jq -r '.data | length')
MEMBERS_COUNT=$(printf '%s' "${MEMBERS_JSON}" | jq -r '.data | length')
BALANCE_STATUS=$(printf '%s' "${BALANCE_JSON}" | jq -r '.status')

if [ "${STORES_COUNT}" -lt 1 ] || [ "${PROVIDERS_COUNT}" -lt 1 ] || [ "${MEMBERS_COUNT}" -lt 1 ] || [ "${BALANCE_STATUS}" != "true" ]; then
  printf '%s\n' \
    "stores_json=${STORES_JSON}" \
    "providers_json=${PROVIDERS_JSON}" \
    "members_json=${MEMBERS_JSON}" \
    "balance_json=${BALANCE_JSON}" >&2
  exit 1
fi

printf '%s\n' \
  "smoke_login=SUCCESS" \
  "smoke_stores=${STORES_COUNT}" \
  "smoke_providers=${PROVIDERS_COUNT}" \
  "smoke_members=${MEMBERS_COUNT}" \
  "smoke_store_api_balance=${BALANCE_STATUS}"
