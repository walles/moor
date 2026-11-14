#!/bin/bash

# Build release binaries for all supported platforms into releases/

set -e -o pipefail

# linux/arm ref: https://github.com/walles/moor/issues/122
for TARGET in linux/386 linux/amd64 linux/arm linux/arm64 darwin/amd64 darwin/arm64 windows/amd64; do
  echo "  ${TARGET}..."
  GOOS="${TARGET%/*}" GOARCH="${TARGET#*/}" ./build.sh
done
