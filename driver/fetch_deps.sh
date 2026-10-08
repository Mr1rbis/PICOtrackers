#!/usr/bin/env bash
# Качает заголовок OpenVR (openvr_driver.h) в third_party/openvr/headers.
set -euo pipefail
cd "$(dirname "$0")"

OPENVR_TAG=v2.15.6
OPENVR_URL=https://codeload.github.com/ValveSoftware/openvr/tar.gz/refs/tags/$OPENVR_TAG
OPENVR_SHA256=e184cb625010fab7043a9d5e1e000fdeb3067a152bb3169ef53f64dfac37164c

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

curl -fsSL --retry 3 -o "$tmp/openvr.tar.gz" "$OPENVR_URL"
echo "$OPENVR_SHA256  $tmp/openvr.tar.gz" | sha256sum -c --quiet
tar xzf "$tmp/openvr.tar.gz" -C "$tmp" "openvr-${OPENVR_TAG#v}/headers/openvr_driver.h"
rm -rf third_party/openvr
mkdir -p third_party/openvr/headers
cp "$tmp/openvr-${OPENVR_TAG#v}/headers/openvr_driver.h" third_party/openvr/headers/
echo "OpenVR $OPENVR_TAG → third_party/openvr"
