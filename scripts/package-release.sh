#!/usr/bin/env bash
# Build the complete family from the checked-out release revision. No publication.
set -euo pipefail
cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.."
out=${1:-dist}
tag=${GITHUB_REF_NAME:-v0.2.3}
[[ $tag =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]
mkdir -p "$out"
flags='-s -w -buildid='
for edition in full ultralite; do
  cmd=thothmemory; prefix=thothmemory
  targets='linux/amd64 linux/arm64 windows/amd64 windows/arm64'
  if [[ $edition == ultralite ]]; then
    cmd=thothmemory-ultralite; prefix=$cmd
    targets='linux/amd64 linux/arm64 windows/amd64 darwin/arm64 android/arm64'
  fi
  for target in $targets; do
    os=${target%/*}; arch=${target#*/}; suffix=
    [[ $os != windows ]] || suffix=.exe
    CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build -trimpath -buildvcs=false -ldflags="$flags" -o "$out/$prefix-$os-$arch$suffix" "./cmd/$cmd"
  done
done
# Verify reproducibility rather than merely selecting deterministic flags.
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -buildvcs=false -ldflags="$flags" -o "$out/repro-check" ./cmd/thothmemory-ultralite
cmp "$out/repro-check" "$out/thothmemory-ultralite-linux-amd64"
rm "$out/repro-check"
cp scripts/thothmini.sh "$out/thothmini.sh"
cp docs/PLATFORMS.md "$out/PLATFORMS.md"
cp docs/RELEASE_0.2.3.md "$out/RELEASE_NOTES.md"
git archive --format=tar.gz --prefix="thothmemory-$tag/" HEAD > "$out/thothmemory-$tag-source.tar.gz"
(cd "$out" && sha256sum thothmemory-* thothmini.sh PLATFORMS.md RELEASE_NOTES.md > SHA256SUMS)
