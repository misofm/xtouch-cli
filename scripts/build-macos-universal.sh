#!/bin/sh

set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
PROJECT_DIR=$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)
BUILD_VERSION=${VERSION:-dev}
OUTPUT_DIR=${OUTPUT_DIR:-"$PROJECT_DIR/dist"}

mkdir -p "$OUTPUT_DIR"
cd "$PROJECT_DIR"

for BUILD_ARCH in arm64 amd64; do
  CGO_ENABLED=1 GOOS=darwin GOARCH="$BUILD_ARCH" \
    go build \
      -trimpath \
      -ldflags "-s -w -X main.version=$BUILD_VERSION" \
      -o "$OUTPUT_DIR/xtouch-cli-darwin-$BUILD_ARCH" \
      ./cmd/xtouch-cli
done

lipo -create \
  -output "$OUTPUT_DIR/xtouch-cli-darwin-universal" \
  "$OUTPUT_DIR/xtouch-cli-darwin-arm64" \
  "$OUTPUT_DIR/xtouch-cli-darwin-amd64"

shasum -a 256 "$OUTPUT_DIR"/xtouch-cli-darwin-*
