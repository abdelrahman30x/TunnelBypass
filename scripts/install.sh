#!/usr/bin/env bash
# TunnelBypass one-line installer for Linux and macOS.
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/abdelrahman30x/TunnelBypass/main/scripts/install.sh | bash
# Default: latest published GitHub release (no version required).
# Environment (optional):
#   INSTALL_OWNER   default: abdelrahman30x
#   INSTALL_REPO    default: TunnelBypass
#   INSTALL_VERSION only if you must pin a tag (e.g. v1.2.1); otherwise omit for latest
#   INSTALL_PREFIX  if set, copy binary to this directory (e.g. $HOME/.local/bin or /usr/local/bin)

set -euo pipefail

OWNER="${INSTALL_OWNER:-abdelrahman30x}"
REPO="${INSTALL_REPO:-TunnelBypass}"
VERSION="${INSTALL_VERSION:-}"
PREFIX="${INSTALL_PREFIX:-}"

say() { printf '%s\n' "$*"; }

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || {
    say "error: required command not found: $1" >&2
    exit 1
  }
}

need_cmd curl

OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64) ARCH_GO="amd64" ;;
  aarch64|arm64) ARCH_GO="arm64" ;;
  *)
    say "error: unsupported architecture: $ARCH" >&2
    exit 1
    ;;
esac

case "$OS" in
  linux) OS_GO="linux" ;;
  darwin) OS_GO="darwin" ;;
  *)
    say "error: unsupported OS: $OS (this script supports Linux and macOS)" >&2
    exit 1
    ;;
esac

if [[ -n "$VERSION" ]]; then
  API_URL="https://api.github.com/repos/${OWNER}/${REPO}/releases/tags/${VERSION}"
else
  API_URL="https://api.github.com/repos/${OWNER}/${REPO}/releases/latest"
fi

TMPJSON="$(mktemp)"
DL=""
WORKDIR=""
cleanup() {
  rm -f "${TMPJSON:-}"
  rm -f "${DL:-}"
  rm -rf "${WORKDIR:-}"
}
trap cleanup EXIT

curl -fsSL -H "Accept: application/vnd.github+json" -H "User-Agent: TunnelBypass-Install" \
  "$API_URL" -o "$TMPJSON"

print_tag() {
  if command -v jq >/dev/null 2>&1; then
    jq -r '.tag_name // empty' "$TMPJSON"
    return
  fi
  if command -v python3 >/dev/null 2>&1; then
    python3 -c "import json,sys; print(json.load(open(sys.argv[1],encoding='utf-8')).get('tag_name') or '')" "$TMPJSON"
    return
  fi
  echo ""
}

TAG="$(print_tag | tr -d '\r')"
if [[ -n "$TAG" ]]; then
  if [[ -n "${VERSION:-}" ]]; then
    say "[*] Release tag: ${TAG} (pinned via INSTALL_VERSION)"
  else
    say "[*] Latest release: ${TAG}"
  fi
fi

pick_url() {
  local want_sub="$1"
  if command -v jq >/dev/null 2>&1; then
    jq -r --arg s "$want_sub" '.assets[] | select(.name | contains($s)) | .browser_download_url' "$TMPJSON" | head -1
    return
  fi
  if command -v python3 >/dev/null 2>&1; then
    python3 - "$TMPJSON" "$want_sub" <<'PY'
import json, sys
path, sub = sys.argv[1], sys.argv[2]
with open(path, encoding="utf-8") as f:
    data = json.load(f)
for a in data.get("assets", []):
    if sub in a.get("name", ""):
        print(a["browser_download_url"])
        break
PY
    return
  fi
  say "error: install jq or python3 to parse the GitHub API response" >&2
  exit 1
}

WANT="_${OS_GO}_${ARCH_GO}"
URL="$(pick_url "$WANT" | head -1 | tr -d '\r')"
if [[ -z "$URL" ]]; then
  say "error: no release asset found for ${OS_GO}/${ARCH_GO}" >&2
  say "  Open: https://github.com/${OWNER}/${REPO}/releases" >&2
  exit 1
fi

say "[*] Downloading: $(basename "$URL")"
DL="$(mktemp)"
curl -fsSL -L -H "User-Agent: TunnelBypass-Install" "$URL" -o "$DL"

WORKDIR="$(mktemp -d)"

case "$URL" in
  *.tar.gz|*.tgz)
    tar -xzf "$DL" -C "$WORKDIR"
    ;;
  *.zip)
    need_cmd unzip
    unzip -q -o "$DL" -d "$WORKDIR"
    ;;
  *)
    cp "$DL" "$WORKDIR/tunnelbypass"
    chmod +x "$WORKDIR/tunnelbypass"
    ;;
esac

BIN="$(find "$WORKDIR" -type f \( -name 'tunnelbypass' -o -name 'tunnelbypass.exe' \) ! -path '*/.*' 2>/dev/null | head -1)"
if [[ -z "$BIN" ]]; then
  BIN="$(find "$WORKDIR" -type f -perm -111 ! -path '*/.*' 2>/dev/null | head -1)"
fi
if [[ -z "$BIN" || ! -f "$BIN" ]]; then
  say "error: could not find tunnelbypass binary inside archive" >&2
  exit 1
fi

chmod +x "$BIN" 2>/dev/null || true

if [[ -n "$PREFIX" ]]; then
  mkdir -p "$PREFIX"
  cp -f "$BIN" "${PREFIX}/tunnelbypass"
  chmod 0755 "${PREFIX}/tunnelbypass"
  say "[+] Installed: ${PREFIX}/tunnelbypass"
  INSTALLED_BIN="${PREFIX}/tunnelbypass"
  case ":$PATH:" in
    *":$PREFIX:"*) ;;
    *) say "[!] Add to PATH, e.g.: export PATH=\"$PREFIX:\$PATH\"" ;;
  esac
else
  OUT="$(pwd)/tunnelbypass"
  cp -f "$BIN" "$OUT"
  chmod +x "$OUT"
  say "[+] Binary ready: $OUT"
  INSTALLED_BIN="$OUT"
fi

VERSION="$("$INSTALLED_BIN" --version 2>/dev/null || true)"
if [[ -n "$VERSION" ]]; then
  say "    Version: $VERSION"
fi

say "    Run: ./tunnelbypass"
