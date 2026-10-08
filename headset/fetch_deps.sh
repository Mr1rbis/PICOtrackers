#!/usr/bin/env bash
# Качает зависимости в third_party/:
#  * pico_openxr — libopenxr_loader.so (arm64) из PICO OpenXR SDK 2.2.0, его ALVR использует для Pico 4 / 4 Pro;
#  * openxr      — заголовки Khronos OpenXR-SDK (в них есть XR_BD_body_tracking и XR_MND_headless).
set -euo pipefail
cd "$(dirname "$0")"

PICO_URL=https://sdk.picovr.com/developer-platform/sdk/PICO_OpenXR_SDK_220.zip
PICO_SHA256=2524927c1b791521c5b7bb518878de0a428793f2d91d60b7a3d84bb7eb6f1e9c
OPENXR_TAG=release-1.1.63
OPENXR_URL=https://codeload.github.com/KhronosGroup/OpenXR-SDK/tar.gz/refs/tags/$OPENXR_TAG
OPENXR_SHA256=5b8dac0608f49498fe8e95f562fe6bcf0bc781e561f137348a8c251d06a88b6f

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

curl -fsSL --retry 3 -o "$tmp/pico.zip" "$PICO_URL"
echo "$PICO_SHA256  $tmp/pico.zip" | sha256sum -c --quiet
unzip -q "$tmp/pico.zip" 'libs/android.arm64-v8a/*' -d "$tmp/pico"
rm -rf third_party/pico_openxr
mkdir -p third_party/pico_openxr/libs
cp -r "$tmp/pico/libs/android.arm64-v8a" third_party/pico_openxr/libs/

curl -fsSL --retry 3 -o "$tmp/openxr.tar.gz" "$OPENXR_URL"
echo "$OPENXR_SHA256  $tmp/openxr.tar.gz" | sha256sum -c --quiet
tar xzf "$tmp/openxr.tar.gz" -C "$tmp" --wildcards "OpenXR-SDK-$OPENXR_TAG/include/openxr/*.h"
rm -rf third_party/openxr
mkdir -p third_party/openxr
cp -r "$tmp/OpenXR-SDK-$OPENXR_TAG/include" third_party/openxr/

echo "зависимости → third_party/"
