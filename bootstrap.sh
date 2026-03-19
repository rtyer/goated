#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"

./build.sh
exec ./goated bootstrap
