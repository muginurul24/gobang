#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT_DIR"

API_PORT="${API_PORT:-18080}"
WEB_PORT="${WEB_PORT:-4174}"
BASE_URL="${BASE_URL:-http://127.0.0.1:${WEB_PORT}}"
OUT_DIR="${OUT_DIR:-.tmp/frontend-pass-local}"
LOGIN_EMAIL="${LOGIN_EMAIL:-dev@example.com}"
LOGIN_PASSWORD="${LOGIN_PASSWORD:-DevDemo123!}"
THEMES="${THEMES:-dark,light}"
API_TIMEOUT_SECONDS="${API_TIMEOUT_SECONDS:-120}"
CHROMIUM_PATH="${CHROMIUM_PATH:-/usr/bin/chromium-browser}"

HTTP_ADDRESS=":${API_PORT}" timeout "${API_TIMEOUT_SECONDS}s" npm run dev:api >/tmp/onixggr-local-api.log 2>&1 &
API_PID=$!

VITE_API_PROXY_TARGET="http://127.0.0.1:${API_PORT}" npm --workspace @onixggr/web run dev -- --host 127.0.0.1 --port "${WEB_PORT}" >/tmp/onixggr-local-web.log 2>&1 &
WEB_PID=$!

cleanup() {
  kill "$WEB_PID" >/dev/null 2>&1 || true
  wait "$WEB_PID" >/dev/null 2>&1 || true
  kill "$API_PID" >/dev/null 2>&1 || true
  wait "$API_PID" >/dev/null 2>&1 || true
}
trap cleanup EXIT

for i in $(seq 1 90); do
  if curl -fsS "http://127.0.0.1:${API_PORT}/health/live" >/tmp/onixggr-local-health.json 2>/dev/null \
    && curl -IksS "${BASE_URL}/login" >/tmp/onixggr-local-login-head.txt 2>/dev/null; then
    break
  fi

  sleep 1

  if [ "$i" = "90" ]; then
    cat /tmp/onixggr-local-api.log
    cat /tmp/onixggr-local-web.log
    exit 1
  fi
done

BASE_URL="$BASE_URL" \
LOGIN_EMAIL="$LOGIN_EMAIL" \
LOGIN_PASSWORD="$LOGIN_PASSWORD" \
OUT_DIR="$OUT_DIR" \
THEMES="$THEMES" \
CHROMIUM_PATH="$CHROMIUM_PATH" \
node scripts/frontend-screenshot-pass.mjs

cat "${OUT_DIR}/manifest.json"
