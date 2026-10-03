#!/usr/bin/env bash
# Build the linux/arm64 image tar without a Docker daemon, using imagetool.
# Useful when only the Docker CLI is present but no engine is running.
#
# The tar is produced from a single UNCOMPRESSED rootfs layer, which is the
# only shape RouterOS 7.24 can import via /container/add file=.
set -euo pipefail

GO="${GO:-/usr/local/go/bin/go}"
VERSION="${VERSION:-1.0.0}"
IMAGE="${IMAGE:-nskforward/mikwg}"
TAR="${TAR:-awg-converter-arm64.tar}"

mkdir -p bin
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 "$GO" build -trimpath \
    -ldflags "-s -w -X main.version=${VERSION}" \
    -o bin/awg-converter-linux-arm64 ./cmd/awg-converter

"$GO" run ./cmd/imagetool \
    -binary bin/awg-converter-linux-arm64 \
    -image "$IMAGE" \
    -out "$TAR" \
    -version "$VERSION"

echo "==> ${TAR} ready ($(du -h "${TAR}" | cut -f1)). Upload it to RouterOS Files."
