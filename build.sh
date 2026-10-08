#!/usr/bin/env bash
set -euo pipefail

if [ $# -ne 1 ]; then
    echo "Usage: $0 <version>"
    echo "Example: $0 0.1.3"
    exit 1
fi

VERSION="$1"
PLATFORMS="linux/amd64,linux/arm64"

echo "Building version $VERSION..."

docker buildx build \
  --platform $PLATFORMS \
  -t cp0x/pi-rwa-interface:$VERSION \
  --push \
  .

echo "✅ Successfully built and pushed version $VERSION"