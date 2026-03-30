#!/usr/bin/env sh
set -eu

./appctl migrate up
./appctl seed demo

printf '%s\n' \
  'Staging bootstrap complete.' \
  'Demo dashboard accounts and seeded store rows are ready.' \
  'Store API token:' \
  '  store_live_demo'
