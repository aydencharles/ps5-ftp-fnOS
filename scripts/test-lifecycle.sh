#!/usr/bin/env bash
set -euo pipefail

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TEST_ROOT="$(mktemp -d)"
APP_DIR="${TEST_ROOT}/app"
DATA_DIR="${TEST_ROOT}/data"
TEMP_DIR="${TEST_ROOT}/tmp"
SHIM_DIR="${TEST_ROOT}/bin"
PID_FILE="${DATA_DIR}/ps5-ftp-manager.pid"

cleanup() {
  if [ -f "${PID_FILE}" ]; then
    TEST_PID="$(cat "${PID_FILE}" 2>/dev/null || true)"
    [ -n "${TEST_PID}" ] && kill "${TEST_PID}" 2>/dev/null || true
  fi
  rm -rf "${TEST_ROOT}"
}
trap cleanup EXIT

mkdir -p "${APP_DIR}/ui" "${DATA_DIR}" "${TEMP_DIR}" "${SHIM_DIR}"

printf '%s\n' \
  '#!/bin/sh' \
  "trap 'exit 0' TERM INT" \
  'while :; do /bin/sleep 1; done' > "${APP_DIR}/server"
printf '%s\n' '#!/bin/sh' 'exit 1' > "${SHIM_DIR}/wget"
printf '%s\n' '#!/bin/sh' 'exit 0' > "${SHIM_DIR}/sleep"
chmod +x "${APP_DIR}/server" "${SHIM_DIR}/wget" "${SHIM_DIR}/sleep"

export PATH="${SHIM_DIR}:/usr/bin:/bin:/usr/sbin:/sbin"
export TRIM_APPNAME="ps5-ftp-manager"
export TRIM_APPDEST="${APP_DIR}"
export TRIM_PKGVAR="${DATA_DIR}"
export TRIM_PKGTMP="${TEMP_DIR}"
export TRIM_SERVICE_PORT="18100"
export TRIM_TEMP_LOGFILE="${TEST_ROOT}/lifecycle.log"

"${PROJECT_ROOT}/packaging/cmd/main" start
"${PROJECT_ROOT}/packaging/cmd/main" status
"${PROJECT_ROOT}/packaging/cmd/main" stop

if [ -f "${PID_FILE}" ]; then
  echo "PID file was not removed after stop" >&2
  exit 1
fi

echo "lifecycle test passed"
