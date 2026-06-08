#!/usr/bin/env bash
# fetch-native.sh — stage the native libs the embedding engine needs for one
# build target into a destination directory:
#
#   - libtokenizers.a       linked statically at BUILD time (CGO_LDFLAGS)
#   - onnxruntime shared lib loaded at RUN time (shipped in the release archive)
#
# Usage: scripts/fetch-native.sh <os> <arch> <dest-dir>
#   os   ∈ linux | darwin | windows
#   arch ∈ amd64 | arm64
#
# Versions are overridable via env: ORT_VERSION, TOKENIZERS_TAG.
set -euo pipefail

os="${1:?usage: fetch-native.sh <os> <arch> <dest>}"
arch="${2:?usage: fetch-native.sh <os> <arch> <dest>}"
dest="${3:?usage: fetch-native.sh <os> <arch> <dest>}"

# ORT 1.26.0 is the first release exposing the API version (26) the Go binding
# (yalue/onnxruntime_go) requires; older libs load but refuse the session.
ORT_VERSION="${ORT_VERSION:-1.26.0}"
TOKENIZERS_TAG="${TOKENIZERS_TAG:-v1.27.0}"

mkdir -p "$dest"

# Map go os/arch -> upstream asset slugs and library file names.
case "$os/$arch" in
  darwin/arm64)  ort_slug="osx-arm64";     tok_slug="darwin-aarch64"; ort_lib="libonnxruntime.${ORT_VERSION}.dylib"; ort_link="libonnxruntime.dylib" ;;
  # darwin/amd64 (Intel mac) is unsupported: onnxruntime stopped publishing an
  # osx-x86_64 build at 1.26.0 (the version the Go binding needs), so there is no
  # runtime to ship. Falls through to the unsupported-target error below.
  linux/amd64)   ort_slug="linux-x64";     tok_slug="linux-x86_64";   ort_lib="libonnxruntime.so.${ORT_VERSION}";    ort_link="libonnxruntime.so" ;;
  linux/arm64)   ort_slug="linux-aarch64"; tok_slug="linux-aarch64";  ort_lib="libonnxruntime.so.${ORT_VERSION}";    ort_link="libonnxruntime.so" ;;
  windows/amd64) ort_slug="win-x64";       tok_slug="windows-x86_64"; ort_lib="onnxruntime.dll";                     ort_link="onnxruntime.dll" ;;
  *) echo "unsupported target: $os/$arch" >&2; exit 1 ;;
esac

echo "==> libtokenizers ($tok_slug $TOKENIZERS_TAG)"
curl -fSL --retry 3 \
  "https://github.com/daulet/tokenizers/releases/download/${TOKENIZERS_TAG}/libtokenizers.${tok_slug}.tar.gz" \
  | tar -xz -C "$dest" libtokenizers.a

echo "==> onnxruntime ($ort_slug $ORT_VERSION)"
# Extract to a scratch dir then copy out the one library we need. The official
# archives carry a leading "./", a versioned top dir, a lib/ subdir and debug
# symbols (.dSYM/DWARF), so a path-strip glob is fragile across tar variants;
# find-and-copy is robust everywhere.
tmp="$(mktemp -d)"
if [ "$os" = "windows" ]; then
  curl -fSL --retry 3 -o "$tmp/ort.zip" \
    "https://github.com/microsoft/onnxruntime/releases/download/v${ORT_VERSION}/onnxruntime-${ort_slug}-${ORT_VERSION}.zip"
  unzip -q "$tmp/ort.zip" -d "$tmp"
else
  curl -fSL --retry 3 \
    "https://github.com/microsoft/onnxruntime/releases/download/v${ORT_VERSION}/onnxruntime-${ort_slug}-${ORT_VERSION}.tgz" \
    | tar -xz -C "$tmp"
fi
src="$(find "$tmp" -path "*/lib/${ort_lib}" -type f | head -1)"
if [ -z "$src" ]; then
  echo "onnxruntime lib ${ort_lib} not found in archive" >&2
  exit 1
fi
cp "$src" "$dest/$ort_lib"
[ "$ort_lib" != "$ort_link" ] && ln -sf "$ort_lib" "$dest/$ort_link"
rm -rf "$tmp"

echo "staged into $dest:"
ls -1 "$dest"
