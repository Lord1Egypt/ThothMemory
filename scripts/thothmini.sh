#!/bin/sh
# ThothMemory Mini manager for ThothDock (Android) or any Docker-compatible CLI.
# Uses only plain docker commands: no Compose, secrets, health checks,
# resource limits or --read-only, none of which ThothDock v0.2.1 provides.
#
#   thothmini.sh install --image IMAGE [--port 7319] [--restart]
#   thothmini.sh start | stop | status | logs | doctor
#   thothmini.sh clients                 # native binary + per-client configs
#   thothmini.sh backup [DIR]            # SQL dump + sha256 sidecar
#   thothmini.sh restore FILE            # into a NEW database, then activation
#   thothmini.sh uninstall [--delete-data]
#
# Touches only the container "thothmemory-mini" and volume
# "thothmemory-mini-data". Never prunes and never removes other objects.
set -eu
NAME=${THOTHMINI_NAME:-thothmemory-mini}
VOLUME=${THOTHMINI_VOLUME:-thothmemory-mini-data}
CONF=${THOTHMINI_HOME:-$HOME/.thothmemory-mini}
PORT=7319

die() { echo "thothmini: $*" >&2; exit 1; }
need_docker() {
  command -v docker >/dev/null 2>&1 || die "docker CLI not found (in ThothTerm, start ThothDock first)"
  docker version >/dev/null 2>&1 || die "Docker engine unreachable (DOCKER_HOST=${DOCKER_HOST:-unset})"
}
engine() { docker version --format '{{.Server.Platform.Name}} {{.Server.Version}}' 2>/dev/null || echo unknown; }
exists() { docker inspect "$NAME" >/dev/null 2>&1; }
port() {
  value=$(docker inspect -f '{{index .Config.Labels "org.thothmemory.host-port"}}' "$NAME")
  case "$value" in ''|'<no value>') docker inspect -f '{{range .Config.Env}}{{println .}}{{end}}' "$NAME" | sed -n 's/^THOTH_LISTEN=127.0.0.1://p' ;; *) echo "$value" ;; esac
}
ready() {
  p=$(port); i=0
  while [ $i -lt 300 ]; do
    if docker exec "$NAME" thothmemory-mini healthcheck >/dev/null 2>&1; then return 0; fi
    [ "$(docker inspect -f '{{.State.Running}}' "$NAME" 2>/dev/null)" = true ] || return 1
    i=$((i+1)); sleep 1
  done
  return 1
}

cmd=${1:-help}; [ $# -gt 0 ] && shift
case "$cmd" in
install)
  image=""; restart=""
  while [ $# -gt 0 ]; do
    case "$1" in
      --image) image=${2:?}; shift 2 ;;
      --port) PORT=${2:?}; shift 2 ;;
      --restart) restart="--restart unless-stopped"; shift ;;
      *) die "unknown option $1" ;;
    esac
  done
  [ -n "$image" ] || die "pass --image with the digest-pinned reference from MINI_IMAGE_DIGEST.txt"
  case "$PORT" in *[!0-9]*|'') die "invalid port" ;; esac
  need_docker
  exists && die "$NAME already exists; use start/stop/status (or uninstall first)"
  echo "Engine: $(engine)"
  docker pull "$image"
  docker volume inspect "$VOLUME" >/dev/null 2>&1 || docker volume create "$VOLUME" >/dev/null
  # Loopback only. On ThothDock containers share the device network, so the
  # service binds 127.0.0.1 itself; no -p forwarder or LAN listener is used.
  # shellcheck disable=SC2086
  case "$(engine)" in
    *ThothDock*) docker run -d --name "$NAME" $restart -v "$VOLUME:/data" --label "org.thothmemory.host-port=$PORT" -e "THOTH_LISTEN=127.0.0.1:$PORT" "$image" >/dev/null ;;
    *) docker run -d --name "$NAME" $restart -v "$VOLUME:/data" --label "org.thothmemory.host-port=$PORT" -p "127.0.0.1:$PORT:7319" -e THOTH_LISTEN=0.0.0.0:7319 "$image" >/dev/null ;;
  esac
  echo "Initializing (first start creates the database; allow up to 5 minutes on a phone)..."
  ready || { docker logs --tail 30 "$NAME" >&2; die "service did not become ready"; }
  echo "Ready on http://127.0.0.1:$PORT (MCP: /mcp). Next: thothmini.sh clients"
  ;;
start) need_docker; docker start "$NAME" >/dev/null; ready && echo ready ;;
stop) need_docker; docker stop -t 60 "$NAME" >/dev/null && echo stopped ;;
logs) need_docker; docker logs --tail "${1:-100}" "$NAME" ;;
status) need_docker; echo "Engine: $(engine)"; docker exec "$NAME" thothmemory-mini status ;;
doctor)
  need_docker
  echo "Engine: $(engine)"
  docker inspect -f 'Container: {{.State.Status}} (started {{.State.StartedAt}})' "$NAME"
  docker exec "$NAME" thothmemory-mini healthcheck && echo "Service: ready on 127.0.0.1:$(port)"
  docker exec "$NAME" thothmemory-mini status
  [ -x "$CONF/thothmemory-mini" ] && "$CONF/thothmemory-mini" doctor --config "$CONF/client.json" || echo "Clients not configured; run: thothmini.sh clients"
  ;;
clients)
  need_docker
  umask 077; mkdir -p "$CONF"; chmod 700 "$CONF"
  # The image writes its own static executable; tokens go to private files.
  docker exec "$NAME" thothmemory-mini self-export > "$CONF/thothmemory-mini.new"
  chmod 700 "$CONF/thothmemory-mini.new"; mv "$CONF/thothmemory-mini.new" "$CONF/thothmemory-mini"
  p=$(port)
  for c in agent codex claude opencode read; do
    principal=$c; [ $c = agent ] && principal=local-agent; [ $c = read ] && principal=local-reader
    docker exec "$NAME" thothmemory-mini token "$principal" > "$CONF/${c}_token"
  done
  printf '{"endpoint":"http://127.0.0.1:%s/mcp","token_file":"agent_token"}\n' "$p" > "$CONF/client.json"
  for c in codex claude opencode; do
    printf '{"endpoint":"http://127.0.0.1:%s/mcp","token_file":"%s_token"}\n' "$p" "$c" > "$CONF/client-$c.json"
  done
  "$CONF/thothmemory-mini" doctor --config "$CONF/client.json"
  echo "Register agents (asks before changing their settings):"
  echo "  $CONF/thothmemory-mini connect all --config $CONF/client.json"
  ;;
backup)
  need_docker
  dir=${1:-$CONF/backups}; umask 077; mkdir -p "$dir"
  f="$dir/thothmemory-mini-$(date -u +%Y%m%dT%H%M%SZ).sql"
  docker exec "$NAME" thothmemory-mini backup > "$f" 2> "$f.log" || { cat "$f.log" >&2; rm -f "$f"; die "backup failed"; }
  sed -n 's/^sha256=//p' "$f.log" > "$f.sha256"; rm -f "$f.log"
  [ "$(sha256sum "$f" | cut -d' ' -f1)" = "$(cat "$f.sha256")" ] || die "backup checksum mismatch"
  echo "Backup: $f (sha256 $(cat "$f.sha256"))"
  ;;
restore)
  need_docker
  f=${1:?restore FILE}; [ -f "$f.sha256" ] || die "missing $f.sha256"
  out=$(docker exec -i "$NAME" thothmemory-mini restore --sha256 "$(cat "$f.sha256")" < "$f" 2>&1) || { echo "$out" >&2; die "restore failed; active memory unchanged"; }
  echo "$out"
  db=$(echo "$out" | grep -o 'thoth_restore_[0-9_]*' | head -1)
  echo "To switch to it: docker exec $NAME thothmemory-mini activate $db && thothmini.sh stop && thothmini.sh start"
  ;;
uninstall)
  need_docker
  exists && docker rm -f "$NAME" >/dev/null && echo "Removed container $NAME"
  if [ "${1:-}" = --delete-data ]; then
    printf 'Type the volume name (%s) to delete ALL Mini memory permanently: ' "$VOLUME"
    read -r answer
    [ "$answer" = "$VOLUME" ] || die "not deleted"
    docker volume rm "$VOLUME" >/dev/null && echo "Deleted volume $VOLUME"
  else
    echo "Kept volume $VOLUME (memory and credentials). Reinstall to reuse it."
  fi
  ;;
*) sed -n '2,13p' "$0" ;;
esac
