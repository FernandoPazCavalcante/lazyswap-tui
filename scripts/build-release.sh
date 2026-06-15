#!/usr/bin/env bash
# Cross-compile release tarballs for every supported target.
# Output: dist/lazyswap-<target>.tar.gz (+ .sha256), binary named `lazyswap`.
# Pure-Go build (CGO_ENABLED=0) — go-ethereum + modernc/sqlite cross-compile clean.
set -euo pipefail

BIN=lazyswap
OUT=dist
mkdir -p "$OUT"

# target-name : GOOS : GOARCH   (names match install.sh's os_id-arch_id)
targets="
linux-x64:linux:amd64
linux-arm64:linux:arm64
darwin-x64:darwin:amd64
darwin-arm64:darwin:arm64
"

for t in $targets; do
  name="${t%%:*}"; rest="${t#*:}"; goos="${rest%%:*}"; goarch="${rest##*:}"
  echo "building ${name} (${goos}/${goarch})"
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" \
    go build -trimpath -ldflags "-s -w" -o "$OUT/$BIN" .
  tar -C "$OUT" -czf "$OUT/lazyswap-${name}.tar.gz" "$BIN"
  rm "$OUT/$BIN"
  ( cd "$OUT" && sha256sum "lazyswap-${name}.tar.gz" > "lazyswap-${name}.tar.gz.sha256" )
done

echo "---"
ls -la "$OUT"
