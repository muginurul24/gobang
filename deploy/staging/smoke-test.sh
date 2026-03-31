#!/usr/bin/env sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
ENV_FILE=${ENV_FILE:-"${SCRIPT_DIR}/env.staging"}

if [ ! -f "${ENV_FILE}" ]; then
  echo "missing ${ENV_FILE}; copy deploy/staging/env.staging.example first" >&2
  exit 1
fi

set -a
. "${ENV_FILE}"
set +a

OWNER_LOGIN=${SMOKE_OWNER_LOGIN:-owner@example.com}
OWNER_PASSWORD=${SMOKE_OWNER_PASSWORD:-OwnerDemo123!}
STORE_ID=${SMOKE_STORE_ID:-cccccccc-cccc-cccc-cccc-cccccccccccc}
STORE_TOKEN=${SMOKE_STORE_TOKEN:-store_live_demo}
STORE_MEMBER=${SMOKE_STORE_MEMBER:-member-demo}
API_CONTAINER="${COMPOSE_PROJECT_NAME}_api_1"
PROXY_CONTAINER="${COMPOSE_PROJECT_NAME}_proxy_1"
PROXY_IP=$(podman inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' "${PROXY_CONTAINER}")

if [ -z "${PROXY_IP}" ]; then
  echo "proxy container has no routable IP" >&2
  exit 1
fi

PORTS=$(podman port "${PROXY_CONTAINER}")
case "${PORTS}" in
  *"80/tcp -> 0.0.0.0:${STAGING_HTTP_PORT}"* ) ;;
  * )
    echo "proxy HTTP port is not published on ${STAGING_HTTP_PORT}" >&2
    exit 1
    ;;
esac

case "${PORTS}" in
  *"443/tcp -> 0.0.0.0:${STAGING_HTTPS_PORT}"* ) ;;
  * )
    echo "proxy HTTPS port is not published on ${STAGING_HTTPS_PORT}" >&2
    exit 1
    ;;
esac

curl_json() {
  method="$1"
  path="$2"
  body="${3:-}"
  shift 3 || true

  if [ -n "${body}" ]; then
    podman exec "${API_CONTAINER}" curl -ksSf --resolve "${STAGING_DOMAIN}:443:${PROXY_IP}" \
      -X "${method}" "https://${STAGING_DOMAIN}${path}" \
      -H 'content-type: application/json' "$@" -d "${body}"
  else
    podman exec "${API_CONTAINER}" curl -ksSf --resolve "${STAGING_DOMAIN}:443:${PROXY_IP}" \
      -X "${method}" "https://${STAGING_DOMAIN}${path}" "$@"
  fi
}

curl_json GET /health/live '' >/dev/null
curl_json GET /health/ready '' >/dev/null
podman exec "${API_CONTAINER}" curl -ksSf --resolve "${STAGING_DOMAIN}:443:${PROXY_IP}" \
  "https://${STAGING_DOMAIN}/" >/dev/null

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
  "smoke_proxy_ports=SUCCESS" \
  "smoke_login=SUCCESS" \
  "smoke_stores=${STORES_COUNT}" \
  "smoke_providers=${PROVIDERS_COUNT}" \
  "smoke_members=${MEMBERS_COUNT}" \
  "smoke_store_api_balance=${BALANCE_STATUS}"
