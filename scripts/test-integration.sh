#!/usr/bin/env bash
# Runs every test against disposable PostgreSQL 18 (Full) and MariaDB 11.4
# (Mini) servers unless THOTH_TEST_DATABASE_URL / THOTH_TEST_MARIADB_ADDR are set.
set -euo pipefail
cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.."
cleanup=()
trap 'for id in "${cleanup[@]}"; do docker rm -f -v "$id" >/dev/null 2>&1 || true; done' EXIT
if [[ -z ${THOTH_TEST_DATABASE_URL:-} ]]; then
  id=$(docker run -d -e POSTGRES_PASSWORD=disposable-test-password -p 127.0.0.1::5432 postgres:18-alpine@sha256:77f585114c32fbca283dc835b0596f4e52b51b4c6662d7810b2f4084f60a1873)
  cleanup+=("$id")
  ready=0
  # Check TCP from the final server, not the socket of the initialization server.
  for ((i=0;i<60;i++)); do if docker exec "$id" pg_isready -h 127.0.0.1 -U postgres >/dev/null 2>&1; then ready=1; break; fi; sleep 1; done
  if [[ $ready != 1 ]]; then echo 'PostgreSQL integration readiness timed out' >&2; exit 1; fi
  port=$(docker port "$id" 5432/tcp); port=${port##*:}
  export THOTH_TEST_DATABASE_URL="postgres://postgres:disposable-test-password@127.0.0.1:$port/postgres?sslmode=disable"
fi
if [[ -z ${THOTH_TEST_MARIADB_ADDR:-} ]]; then
  id=$(docker run -d -e MARIADB_ROOT_PASSWORD=disposable-test-password -p 127.0.0.1::3306 mariadb:11.4.12@sha256:4f1d8d202fcf7bcb3902f63af09f9c1a050c2922a89652f22abaec0d4f015e83)
  cleanup+=("$id")
  ready=0
  for ((i=0;i<90;i++)); do if docker exec "$id" healthcheck.sh --connect --innodb_initialized >/dev/null 2>&1; then ready=1; break; fi; sleep 1; done
  if [[ $ready != 1 ]]; then echo 'MariaDB integration readiness timed out' >&2; exit 1; fi
  port=$(docker port "$id" 3306/tcp); port=${port##*:}
  export THOTH_TEST_MARIADB_ADDR="127.0.0.1:$port" THOTH_TEST_MARIADB_ROOT_PASSWORD=disposable-test-password
fi
go test -race -count=1 -v ${THOTH_TEST_RUN:+-run "$THOTH_TEST_RUN"} ./...
