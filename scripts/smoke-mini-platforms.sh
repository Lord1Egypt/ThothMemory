#!/usr/bin/env bash
# Exercise the exact published Mini candidate by digest, in disposable fixtures.
set -euo pipefail
: "${MINI_IMAGE:?digest-pinned Mini candidate required}"
[[ $MINI_IMAGE =~ @sha256:[0-9a-f]{64}$ ]]
work=$(mktemp -d)
name=thothmini-release-$(date +%s)-$$
cleanup() { docker rm -f -v "$name" >/dev/null 2>&1 || true; rm -rf "$work"; }
trap cleanup EXIT
for arch in amd64 arm64; do
  docker pull --platform "linux/$arch" "$MINI_IMAGE"
  docker create --platform "linux/$arch" --name "$name" -p 127.0.0.1::7319 -e THOTH_LISTEN=0.0.0.0:7319 "$MINI_IMAGE" >/dev/null
  docker cp "$name:/usr/local/bin/thothmemory-mini" "$work/app"
  read -r low high < <(od -An -tu1 -j18 -N2 "$work/app")
  expected=62; [[ $arch != arm64 ]] || expected=183
  [[ $low == "$expected" && $high == 0 ]]
  docker start "$name" >/dev/null
  port=$(docker port "$name" 7319/tcp); port=${port##*:}
  ready=false
  for _ in $(seq 180); do
    if curl -fsS "http://127.0.0.1:$port/healthz" >/dev/null; then ready=true; break; fi
    sleep 1
  done
  $ready || { docker logs "$name"; exit 1; }
  umask 077
  docker exec "$name" thothmemory-mini token local-agent > "$work/agent"
  docker exec "$name" thothmemory-mini token local-reader > "$work/reader"
  python3 scripts/mini-acceptance.py --endpoint "http://127.0.0.1:$port/mcp" --token-file "$work/agent" --reader-token-file "$work/reader"
  docker rm -f -v "$name" >/dev/null
  echo "linux/$arch Mini ELF and 11 MCP gates PASS (arm64 is emulated on amd64)"
done
# Complete backup/restore and recovery lifecycle against this exact digest, per architecture
# (arm64 runs under QEMU on the amd64 runner). Any failed gate aborts the publication.
volume=$name-data
trap 'for a in amd64 arm64; do docker rm -f -v "$name-$a" >/dev/null 2>&1 || true; docker volume rm "$name-$a-data" >/dev/null 2>&1 || true; done; docker rm -f -v "$name" >/dev/null 2>&1 || true; rm -rf "$work"' EXIT
for arch in amd64 arm64; do
  docker pull --platform "linux/$arch" "$MINI_IMAGE" >/dev/null
  PLATFORM="linux/$arch" THOTH_LIFECYCLE_MCP=1 THOTH_SKEW_GATE=1 scripts/mini-lifecycle.sh "$name-$arch" "$name-$arch-data" "$MINI_IMAGE" 27419
  docker rm -f -v "$name-$arch" >/dev/null  # the next architecture's orphan scan must not see this fixture
  echo "linux/$arch Mini backup, verified restore and recovery lifecycle PASS (arm64 is emulated on amd64)"
done
