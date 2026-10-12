#!/usr/bin/env bash
set -euo pipefail
: "${IMAGE:?IMAGE must be a validated candidate tag or digest}"
network=thoth-smoke-$(date +%s)-$$
docker network create "$network" >/dev/null
container=
cleanup(){ if [[ -n "$container" ]]; then docker rm -f "$container" >/dev/null; fi; docker rm -f "$network-db" >/dev/null 2>&1 || true; docker network rm "$network" >/dev/null; }
trap cleanup EXIT
docker run -d --name "$network-db" --network "$network" --network-alias db -e POSTGRES_PASSWORD=disposable-smoke-password postgres:18-alpine@sha256:77f585114c32fbca283dc835b0596f4e52b51b4c6662d7810b2f4084f60a1873 >/dev/null
for ((i=0;i<60;i++)); do if docker exec "$network-db" pg_isready -U postgres >/dev/null 2>&1; then break; fi; sleep 1; done
work=$(mktemp -d)
chmod 755 "$work"
printf '[{"id":"smoke","token":"disposable-smoke-token-00000000000000000000000","write":true,"projects":["*"]}]' > "$work/tokens.json"
chmod 644 "$work/tokens.json"
for arch in amd64 arm64; do
  platform_image="$IMAGE"
  if [[ ${SMOKE_LOCAL_IMAGES:-0} == 1 ]]; then platform_image="$IMAGE-$arch"; else docker pull --platform "linux/$arch" "$IMAGE"; fi
  docker exec "$network-db" createdb -U postgres "thoth_smoke_$arch"
  docker run --rm --platform "linux/$arch" --network "$network" -e THOTH_DB_USER=postgres -e THOTH_DB_PASSWORD=disposable-smoke-password -e THOTH_RUNTIME_PASSWORD=disposable-runtime-password -e THOTH_DB_NAME=thoth_smoke_$arch "$platform_image" migrate
  container=$network-$arch
  docker run -d --name "$container" --platform "linux/$arch" --network "$network" -p 127.0.0.1::7319 -v "$work/tokens.json:/run/secrets/tokens:ro" -e THOTH_TOKENS_FILE=/run/secrets/tokens -e THOTH_DB_PASSWORD=disposable-runtime-password -e THOTH_DB_NAME=thoth_smoke_$arch -e THOTH_LISTEN=0.0.0.0:7319 "$platform_image" >/dev/null
  for ((i=0;i<60;i++)); do if docker exec "$container" /usr/local/bin/thothmemory healthcheck; then break; fi; sleep 1; done
  port=$(docker port "$container" 7319/tcp);port=${port##*:}
  printf '%s' disposable-smoke-token-00000000000000000000000 > "$work/token"
  printf '{"endpoint":"http://127.0.0.1:%s/mcp","token_file":"%s/token"}' "$port" "$work" > "$work/client.json"
  ./bin/thothmemory doctor --config "$work/client.json"
  ./bin/thothmemory demo --config "$work/client.json"
  docker rm -f "$container" >/dev/null
  container=
  echo "linux/$arch: real server, migration, MCP handshake, tool discovery, writes and handoff PASS"
done
