#!/usr/bin/env bash
set -euo pipefail

TARGET="${WINDSURF_LS_BINARY_PATH:-/opt/windsurf/language_server_linux_x64}"
OUR_RELEASE='https://github.com/dwgx/WindsurfAPI/releases/latest/download/language_server_linux_x64'
EXAFUNCTION_API='https://api.github.com/repos/Exafunction/codeium/releases/latest'

log() { echo "[windsurf-ls] $*"; }
err() { echo "[windsurf-ls] $*" >&2; }

mkdir -p "$(dirname "$TARGET")"

if [ -x "$TARGET" ]; then
  log "binary already exists at $TARGET"
  exit 0
fi

log "downloading language server to $TARGET"

if curl -fL --progress-bar -o "$TARGET" "$OUR_RELEASE"; then
  chmod +x "$TARGET"
  log "downloaded from WindsurfAPI release"
  exit 0
fi

log "primary release unavailable, falling back to Exafunction latest"
fallback_url="$(
  curl -fsSL "$EXAFUNCTION_API" |
    grep -oE 'https://[^"]+/language_server_linux_x64' |
    head -n 1
)"

if [ -z "$fallback_url" ]; then
  err "could not resolve fallback language server asset"
  exit 1
fi

curl -fL --progress-bar -o "$TARGET" "$fallback_url"
chmod +x "$TARGET"
log "downloaded from Exafunction release"
