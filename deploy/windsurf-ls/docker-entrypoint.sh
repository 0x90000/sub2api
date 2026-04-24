#!/usr/bin/env bash
set -euo pipefail

BINARY_PATH="${WINDSURF_LS_BINARY_PATH:-/opt/windsurf/language_server_linux_x64}"
PORT="${WINDSURF_LS_PORT:-42100}"
INTERNAL_PORT="${WINDSURF_LS_INTERNAL_PORT:-42101}"
CSRF_TOKEN="${WINDSURF_LS_CSRF_TOKEN:-windsurf-api-csrf-fixed-token}"
API_SERVER_URL="${WINDSURF_LS_API_SERVER_URL:-https://server.self-serve.windsurf.com}"
REGISTER_USER_URL="${WINDSURF_LS_REGISTER_USER_URL:-https://api.codeium.com/register_user/}"
DATA_DIR="${WINDSURF_LS_DATA_DIR:-/opt/windsurf/data}"
WORKSPACE_DIR="${WINDSURF_LS_WORKSPACE_DIR:-/tmp/windsurf-workspace}"

mkdir -p "${DATA_DIR}/db" "${WORKSPACE_DIR}"

if [ ! -x "$BINARY_PATH" ] && [ "${WINDSURF_LS_AUTO_SETUP:-1}" = "1" ]; then
  /app/install-ls.sh
fi

if [ ! -x "$BINARY_PATH" ]; then
  echo "[windsurf-ls] language server binary missing at ${BINARY_PATH}" >&2
  exit 1
fi

if [ "$PORT" = "$INTERNAL_PORT" ]; then
  echo "[windsurf-ls] WINDSURF_LS_PORT and WINDSURF_LS_INTERNAL_PORT must differ" >&2
  exit 1
fi

cleanup() {
  if [ -n "${SOCAT_PID:-}" ]; then
    kill "${SOCAT_PID}" 2>/dev/null || true
  fi
  if [ -n "${LS_PID:-}" ]; then
    kill "${LS_PID}" 2>/dev/null || true
  fi
}

trap cleanup EXIT INT TERM

echo "[windsurf-ls] starting ${BINARY_PATH} on 127.0.0.1:${INTERNAL_PORT}"
"$BINARY_PATH" \
  "--api_server_url=${API_SERVER_URL}" \
  "--server_port=${INTERNAL_PORT}" \
  "--csrf_token=${CSRF_TOKEN}" \
  "--register_user_url=${REGISTER_USER_URL}" \
  "--codeium_dir=${DATA_DIR}" \
  "--database_dir=${DATA_DIR}/db" \
  "--detect_proxy=false" &
LS_PID=$!

for _ in $(seq 1 60); do
  if nc -z 127.0.0.1 "${INTERNAL_PORT}" >/dev/null 2>&1; then
    break
  fi
  sleep 1
done

if ! nc -z 127.0.0.1 "${INTERNAL_PORT}" >/dev/null 2>&1; then
  echo "[windsurf-ls] language server did not become ready on 127.0.0.1:${INTERNAL_PORT}" >&2
  exit 1
fi

echo "[windsurf-ls] forwarding 0.0.0.0:${PORT} -> 127.0.0.1:${INTERNAL_PORT}"
socat "TCP-LISTEN:${PORT},fork,reuseaddr,bind=0.0.0.0" "TCP:127.0.0.1:${INTERNAL_PORT}" &
SOCAT_PID=$!

wait -n "${LS_PID}" "${SOCAT_PID}"
