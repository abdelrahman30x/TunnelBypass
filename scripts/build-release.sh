#!/usr/bin/env bash
# Build and package release assets into ./build/<version>/ (Linux and Windows; run from Git Bash on Windows if needed).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

VERSION="${1:-}"
if [[ -z "${VERSION}" ]] && [[ -f VERSION ]]; then
  VERSION="$(tr -d '\r\n' < VERSION)"
fi
if [[ -z "${VERSION}" ]]; then
  VERSION="$(git describe --tags --abbrev=0 2>/dev/null || true)"
fi
if [[ -z "${VERSION}" ]]; then
  VERSION="v0.0.0"
fi

OUT_DIR="${ROOT}/build/${VERSION}"
mkdir -p "${OUT_DIR}"

echo "Building release for version: ${VERSION}"
echo "Output directory: ${OUT_DIR}"

build_one() {
  local goos="$1" goarch="$2" ext="$3"
  local bin="tunnelbypass${ext}"
  local binpath="${OUT_DIR}/${bin}"
  local asset

  if [[ "${ext}" == ".exe" ]]; then
    asset="tunnelbypass_${VERSION}_${goos}_${goarch}.exe"
  else
    asset="tunnelbypass_${VERSION}_${goos}_${goarch}.tar.gz"
  fi
  local assetpath="${OUT_DIR}/${asset}"

  echo "-> Building ${goos}/${goarch} as ${asset}"

  GOOS="${goos}" GOARCH="${goarch}" go build -trimpath \
    -ldflags "-s -w -X main.Version=${VERSION}" \
    -o "${binpath}" ./cmd

  if [[ "${goos}" != "windows" ]]; then
    if [[ -f "${binpath}" ]]; then
      tar -czf "${assetpath}" -C "${OUT_DIR}" "${bin}"
      rm -f "${binpath}"
    else
      echo "Warning: binary ${binpath} not found, skipping ${goos}/${goarch}" >&2
    fi
  else
    rm -f "${assetpath}"
    if [[ -f "${binpath}" ]]; then
      mv -f "${binpath}" "${assetpath}"
    else
      echo "Warning: binary ${binpath} not found, skipping ${goos}/${goarch}" >&2
    fi
  fi
}

build_one linux amd64 ""
build_one linux arm64 ""
build_one windows amd64 ".exe"
build_one windows arm64 ".exe"

echo "=================="
echo "All release assets created successfully:"
ls -la "${OUT_DIR}"/tunnelbypass_"${VERSION}"_* 2>/dev/null || true
