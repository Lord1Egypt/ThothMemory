#!/usr/bin/env bash
set -euo pipefail
cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.."
if [[ -z ${THOTH_TEST_DATABASE_URL:-} ]]; then
  id=$(docker run -d --rm -e POSTGRES_PASSWORD=disposable-test-password -p 127.0.0.1::5432 postgres:18-alpine)
  trap 'docker stop "$id" >/dev/null' EXIT
  for ((i=0;i<60;i++)); do if docker exec "$id" pg_isready -U postgres >/dev/null 2>&1; then break; fi; sleep 1; done
  port=$(docker port "$id" 5432/tcp); port=${port##*:}
  export THOTH_TEST_DATABASE_URL="postgres://postgres:disposable-test-password@127.0.0.1:$port/postgres?sslmode=disable"
fi
go test -race -count=1 -v ./...
