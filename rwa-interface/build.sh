#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")"

ALLOW_DIRTY=0
ARGS=()
for a in "$@"; do
    case "$a" in
        --allow-dirty) ALLOW_DIRTY=1 ;;
        *) ARGS+=("$a") ;;
    esac
done

if [ ${#ARGS[@]} -ne 1 ]; then
    echo "Usage: $0 [--allow-dirty] <version>"
    echo "Example: $0 2.0.0"
    exit 1
fi

if [ "$ALLOW_DIRTY" -ne 1 ] && [ -n "$(git status --porcelain -- .)" ]; then
    echo "Working tree has uncommitted changes in $(pwd); commit them or pass --allow-dirty" >&2
    exit 1
fi

VERSION="${ARGS[0]}"
PLATFORMS="linux/amd64,linux/arm64"

echo "Building version $VERSION..."

docker buildx build \
  --platform $PLATFORMS \
  --build-arg VITE_APP_VERSION="v$VERSION" \
  -t cp0x/pi-rwa-interface:$VERSION \
  -t cp0x/pi-rwa-interface:latest \
  --push \
  .

echo "Successfully built and pushed version $VERSION"
