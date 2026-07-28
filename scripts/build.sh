#!/usr/bin/env bash
set -euo pipefail

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DIST_DIR="${DIST_DIR:-${PROJECT_ROOT}/dist}"
PACKAGE_DIR="${PACKAGE_DIR:-${PROJECT_ROOT}/packaging}"
APP_STAGE="${PACKAGE_DIR}/app"
FRONTEND_DIR="${PROJECT_ROOT}/src/frontend"
FNPACK_BIN="${FNPACK:-fnpack}"
TARGET_GOOS="${GOOS:-linux}"
TARGET_GOARCH="${GOARCH:-amd64}"

mkdir -p "${DIST_DIR}"

if [ "${SKIP_TEST:-0}" != "1" ]; then
  echo "==> Go tests"
  (cd "${PROJECT_ROOT}" && go test ./src/backend/... -count=1 -timeout 120s)
  echo "==> fnOS lifecycle test"
  "${PROJECT_ROOT}/scripts/test-lifecycle.sh"
  echo "==> Frontend checks"
  pnpm --dir "${FRONTEND_DIR}" install --frozen-lockfile
  pnpm --dir "${FRONTEND_DIR}" lint
  pnpm --dir "${FRONTEND_DIR}" typecheck
  pnpm --dir "${FRONTEND_DIR}" test
fi

echo "==> Frontend production build"
pnpm --dir "${FRONTEND_DIR}" build

echo "==> Go static build (${TARGET_GOOS}/${TARGET_GOARCH})"
(cd "${PROJECT_ROOT}" && CGO_ENABLED=0 GOOS="${TARGET_GOOS}" GOARCH="${TARGET_GOARCH}" \
  go build -trimpath -ldflags="-s -w" -o "${DIST_DIR}/server" ./src/backend)

echo "==> Stage fnOS application"
rm -rf "${APP_STAGE}"
mkdir -p "${APP_STAGE}/ui/images"
cp "${DIST_DIR}/server" "${APP_STAGE}/server"
chmod +x "${APP_STAGE}/server"
cp -R "${FRONTEND_DIR}/dist/." "${APP_STAGE}/ui/"
cp "${PACKAGE_DIR}/ui/config" "${APP_STAGE}/ui/config"
cp "${PACKAGE_DIR}/ICON.PNG" "${APP_STAGE}/ui/images/icon_32.png"
cp "${PACKAGE_DIR}/ICON_256.PNG" "${APP_STAGE}/ui/images/icon_256.png"

if [ "${SKIP_FNPACK:-0}" = "1" ]; then
  echo "==> SKIP_FNPACK=1; staged runtime is ready at ${APP_STAGE}"
  exit 0
fi

if ! command -v "${FNPACK_BIN}" >/dev/null 2>&1; then
  echo "fnpack not found; set FNPACK=/path/to/fnpack or SKIP_FNPACK=1" >&2
  exit 1
fi

echo "==> Build .fpk"
if "${FNPACK_BIN}" build --help 2>&1 | grep -q -- '--directory'; then
  "${FNPACK_BIN}" build --directory "${PACKAGE_DIR}"
else
  "${FNPACK_BIN}" build -d "${PACKAGE_DIR}"
fi

shopt -s nullglob
for package_file in "${PACKAGE_DIR}"/*.fpk "${PROJECT_ROOT}"/*.fpk; do
  mv -f "${package_file}" "${DIST_DIR}/"
done
shopt -u nullglob
echo "==> Outputs"
ls -lh "${DIST_DIR}"
