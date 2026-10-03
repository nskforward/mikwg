#!/usr/bin/env bash
# Build the linux/arm64 container image and save it as a tar for upload to the
# router via Winbox/Files.
set -euo pipefail

VERSION="${VERSION:-1.0.0}"
IMAGE="${IMAGE:-mikwg/awg-converter}"
TAR="${TAR:-awg-converter-arm64.tar}"

docker buildx build \
    --platform linux/arm64 \
    --build-arg "VERSION=${VERSION}" \
    -t "${IMAGE}:${VERSION}" \
    --load .

docker save "${IMAGE}:${VERSION}" -o "${TAR}"
echo "==> ${TAR} ready ($(du -h "${TAR}" | cut -f1)). Upload it to RouterOS Files."
