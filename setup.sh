#!/usr/bin/env bash
set -euo pipefail
cd -- "$(dirname -- "${BASH_SOURCE[0]}")"
command -v docker >/dev/null || { echo 'Install Docker Engine + Compose, or Docker Desktop with WSL2 integration.' >&2; exit 1; }
docker info >/dev/null 2>&1 || { echo 'Start Docker and retry.' >&2; exit 1; }
docker compose version >/dev/null
# Read only known, nonsecret settings; never execute .env as shell code.
if [[ -f .env ]]; then
  while IFS='=' read -r key value; do
    case "$key" in
      THOTH_IMAGE) export THOTH_IMAGE=${THOTH_IMAGE:-$value} ;;
      THOTH_PORT) export THOTH_PORT=${THOTH_PORT:-$value} ;;
    esac
  done < .env
fi
build=false
if [[ ${1:-} == --build ]]; then build=true; export THOTH_IMAGE=${THOTH_IMAGE:-thoth-memory:dev}; fi
image=${THOTH_IMAGE:-lord1egypt/thoth-memory:0.2.0}
if $build; then docker build -t "$image" .; else docker pull "$image"; fi
mkdir -p bin
container=$(docker create "$image")
trap 'docker rm "$container" >/dev/null 2>&1 || true' EXIT
docker cp "$container:/usr/local/bin/thothmemory" bin/thothmemory
chmod 755 bin/thothmemory
docker rm "$container" >/dev/null
trap - EXIT
./bin/thothmemory init-config
./bin/thothmemory preflight --config client.json
# Compose file-backed secrets retain host permissions. Grant only the container UID
# read access through an ACL when supported, or use Docker Desktop's secret mounts.
if command -v setfacl >/dev/null; then
  setfacl -m u:65532:r .secrets/db_app_password .secrets/tokens.json .secrets/db_owner_password
fi
# Docker Desktop virtualizes bind permissions; native Engine needs readable copies.
# Keep the containing directory owner-only; files are never reachable by other users.
chmod 700 .secrets
chmod 644 .secrets/db_app_password .secrets/tokens.json .secrets/db_owner_password
# Compose itself detects occupied ports without killing or replacing their owner.
docker compose up -d --no-build --wait --wait-timeout 120
./bin/thothmemory doctor --config client.json
printf '\nDashboard: http://127.0.0.1:%s\n' "${THOTH_PORT:-7319}"
printf 'Unlock using .secrets/agent_token. Connect clients: ./bin/thothmemory connect all\n'
if command -v xdg-open >/dev/null; then xdg-open "http://127.0.0.1:${THOTH_PORT:-7319}" >/dev/null 2>&1 || true; fi
