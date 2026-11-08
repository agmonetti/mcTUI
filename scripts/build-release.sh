#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."
version=${1:-dev}
if [[ "$version" != dev && ! "$version" =~ ^v[0-9]+\.[0-9]+(\.[0-9]+)?(-[0-9A-Za-z]+([.-][0-9A-Za-z]+)*)?$ ]]; then
  printf 'Invalid version: %s (expected dev, v1.6, v1.6.0 or v1.6.0-rc.1)\n' "$version" >&2
  exit 1
fi

mkdir -p dist
targets=(linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64)
files=()
for target in "${targets[@]}"; do
  os=${target%/*}
  arch=${target#*/}
  filename="mctui-${os}-${arch}"
  if [[ "$os" == windows ]]; then
    filename+=.exe
  fi
  CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build \
    -mod=readonly -trimpath -ldflags="-s -w -X main.Version=$version" \
    -o "dist/$filename" .
  files+=("$filename")
done

# Hash exactly this build's outputs, excluding any unrelated files in dist/.
cd dist
sha256sum "${files[@]}" > checksums.txt
