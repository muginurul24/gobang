#!/usr/bin/env sh
set -eu

./appctl migrate fresh "$@"
