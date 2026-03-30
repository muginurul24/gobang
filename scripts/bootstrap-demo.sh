#!/usr/bin/env sh
set -eu

./scripts/podman-up.sh
./appctl migrate fresh --seed

printf '%s\n' \
  'Demo bootstrap complete.' \
  'Dashboard accounts:' \
  '  dev@example.com / DevDemo123!' \
  '  superadmin@example.com / SuperadminDemo123!' \
  '  owner@example.com / OwnerDemo123!' \
  '  staff@example.com / StaffDemo123!' \
  'Store API token:' \
  '  store_live_demo'
