#!/usr/bin/env bash
# Build the linux/arm64 image tar for RouterOS.
#
# Historically this used `docker buildx` + `docker save`, but that produces
# gzip-compressed layers, which RouterOS 7.24 cannot import ("error getting
# layer file / failed to load next entry"). The RouterOS-compatible artifact is
# built by imagetool with a single UNCOMPRESSED layer, so this script now
# delegates to build-nodocker.sh. Use `make image` if you want a Docker image
# for a local smoke test.
set -euo pipefail
exec "$(dirname "$0")/build-nodocker.sh"
