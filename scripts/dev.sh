#!/usr/bin/env bash
set -euo pipefail
PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DEV_DATA_DIR="${DEV_DATA_DIR:-${PROJECT_ROOT}/.dev-data}"
mkdir -p "${DEV_DATA_DIR}"
export DATA_DIR="${DEV_DATA_DIR}"
export UI_DIR="${PROJECT_ROOT}/src/frontend/dist"
export PORT="${PORT:-8100}"
exec go run "${PROJECT_ROOT}/src/backend"
