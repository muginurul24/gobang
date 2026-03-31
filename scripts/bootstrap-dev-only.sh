#!/usr/bin/env sh
set -eu

./scripts/podman-up.sh
./scripts/podman-manage.sh migrate fresh --seed=dev-only

printf '%s\n' \
  'Dev-only bootstrap complete.' \
  'Dashboard account:' \
  '  dev@example.com / DevDemo123!'

