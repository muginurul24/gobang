#!/usr/bin/env sh
set -eu

config_keys_file="$(mktemp)"
example_keys_file="$(mktemp)"
env_keys_file="$(mktemp)"
production_keys_file="$(mktemp)"
ignored_production_keys_file="$(mktemp)"

cleanup() {
	rm -f "$config_keys_file" "$example_keys_file" "$env_keys_file" "$production_keys_file" "$ignored_production_keys_file"
}

trap cleanup EXIT INT TERM

rg -o 'env(?:String|Int|Int64|Bool|Duration)\("([A-Z0-9_]+)"' -r '$1' internal/platform/config/config.go | sort -u >"$config_keys_file"
awk -F= '/^[A-Z0-9_]+=/{print $1}' .env.example | sort -u >"$example_keys_file"

missing_config_keys="$(comm -23 "$config_keys_file" "$example_keys_file" || true)"
if [ -n "$missing_config_keys" ]; then
	echo ".env.example is missing config keys:"
	printf '%s\n' "$missing_config_keys"
	exit 1
fi

cat <<'EOF' >"$ignored_production_keys_file"
DATABASE_URL
HTTP_ADDRESS
REDIS_URL
EOF

awk -F= '/^[A-Z0-9_]+=/{print $1}' deploy/production/env.production.example | sort -u >"$production_keys_file"
missing_production_keys="$(comm -23 "$config_keys_file" "$production_keys_file" | comm -23 - "$ignored_production_keys_file" || true)"
if [ -n "$missing_production_keys" ]; then
	echo "deploy/production/env.production.example is missing config keys:"
	printf '%s\n' "$missing_production_keys"
	exit 1
fi

if [ -f .env ]; then
	awk -F= '/^[A-Z0-9_]+=/{print $1}' .env | sort -u >"$env_keys_file"
	missing_env_keys="$(comm -12 "$config_keys_file" "$env_keys_file" | comm -23 - "$example_keys_file" || true)"
	if [ -n "$missing_env_keys" ]; then
		echo ".env contains runtime config keys that are missing from .env.example:"
		printf '%s\n' "$missing_env_keys"
		exit 1
	fi
fi

echo "Environment key sync OK."
