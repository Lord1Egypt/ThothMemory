#!/usr/bin/env bash
# Runs scripts/compose-isolation-test.sh against a DISPOSABLE Docker-in-Docker daemon, so the
# historical volume name `thothmemory_memory` can be created without touching a real daemon.
#   THOTH_NEW_IMAGE=<local candidate Full image>  scripts/compose-isolation-dind.sh
# The test runs in a `docker:cli` container on a private network that talks to the throwaway
# daemon (no ports published, the host socket is never mounted). Images are copied in with
# `docker save | docker exec -i dind docker load`; nothing is published.
set -euo pipefail
cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.."
: "${THOTH_NEW_IMAGE:?local candidate Full image tag}"
PG=$(sed -n 's/^const GuardImage = "\(.*\)"/\1/p' internal/cli/volume.go)
id=$$
net=thoth-iso-net-$id; dind=thoth-iso-dind-$id; wv=thoth-iso-work-$id
work=$(mktemp -d)
cleanup() { docker rm -f -v "$dind" >/dev/null 2>&1 || true; docker network rm "$net" >/dev/null 2>&1 || true; docker volume rm "$wv" >/dev/null 2>&1 || true; rm -rf "$work"; }
trap cleanup EXIT
CGO_ENABLED=0 go build -buildvcs=false -o "$work/thothmemory" ./cmd/thothmemory
# Legacy fixture inputs: the published compose file and CLI of the legacy release, never edited.
: "${THOTH_LEGACY_TAG:=v0.2.1}"
: "${THOTH_LEGACY_IMAGE:=lord1egypt/thoth-memory@sha256:73dc7b329b3da64817497bec23fb23ab946a2762bb6b7524ce9a63f7ae54b86d}"
: "${THOTH_MID_IMAGE:=lord1egypt/thoth-memory@sha256:1505d340cb6f59ed273bd1d7532671f738ad8fc7e198bfc9c74b00e33b1852d5}"
: "${THOTH_OLDEST_IMAGE:=lord1egypt/thoth-memory@sha256:c6c7d7e0803afe1e8703f1fd6d8cf80f7427cf2d3447a07316ac378afdafd245}"
export THOTH_LEGACY_IMAGE THOTH_MID_IMAGE THOTH_OLDEST_IMAGE
git show "$THOTH_LEGACY_TAG:compose.yaml" > "$work/legacy-compose.yaml"
docker pull -q "$PG" >/dev/null; docker pull -q "$THOTH_LEGACY_IMAGE" >/dev/null; docker pull -q "$THOTH_MID_IMAGE" >/dev/null; docker pull -q "$THOTH_OLDEST_IMAGE" >/dev/null
cid=$(docker create "$THOTH_LEGACY_IMAGE"); docker cp "$cid:/usr/local/bin/thothmemory" "$work/legacy-thothmemory"; docker rm "$cid" >/dev/null
docker network create "$net" >/dev/null
# /work is mounted in both containers: Compose file secrets are bind mounts resolved by the daemon.
docker volume create "$wv" >/dev/null
docker run -d --privileged --name "$dind" --network "$net" --network-alias dind -v "$wv:/work" -e DOCKER_TLS_CERTDIR= docker:dind >/dev/null
for _ in $(seq 90); do docker exec "$dind" docker info >/dev/null 2>&1 && break; sleep 1; done
docker exec "$dind" docker info >/dev/null 2>&1 || { echo "disposable daemon did not start" >&2; exit 1; }
images=("$THOTH_NEW_IMAGE" "$PG" "$THOTH_LEGACY_IMAGE" "$THOTH_MID_IMAGE" "$THOTH_OLDEST_IMAGE")
for extra in ${THOTH_EXTRA_IMAGES:-}; do images+=("$extra"); done
docker save "${images[@]}" | docker exec -i "$dind" docker load >/dev/null
docker run --rm --network "$net" -e DOCKER_HOST=tcp://dind:2375 -e THOTH_DISPOSABLE_DAEMON=1 -e THOTH_NEW_IMAGE -e THOTH_CLI_BIN=/cli/thothmemory -e THOTH_LEGACY_CLI=/cli/legacy-thothmemory -e THOTH_LEGACY_COMPOSE=/cli/legacy-compose.yaml -e THOTH_LEGACY_IMAGE -e THOTH_MID_IMAGE -e THOTH_OLDEST_IMAGE \
  -v "$PWD:/repo:ro" -v "$work:/cli:ro" -v "$wv:/work" --entrypoint sh docker:cli -c 'apk add --no-cache bash coreutils findutils grep sed >/dev/null && bash /repo/scripts/compose-isolation-test.sh'
