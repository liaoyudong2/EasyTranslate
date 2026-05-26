#!/usr/bin/env sh
set -eu

cd "$(dirname "$0")/.."
go run ./scripts/package.go "$@"
