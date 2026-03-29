#!/usr/bin/env sh
set -eu

podman compose -f docker-compose.yml up -d postgres redis api web
