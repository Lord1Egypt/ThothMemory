#!/usr/bin/env bash
# Lifecycle gates for a DISPOSABLE ThothMemory Mini container: restart,
# SIGKILL recovery, recreation on the same volume, backup and verified
# restore into a new database, activation, clean stop and orphan checks.
#
#   scripts/mini-lifecycle.sh CONTAINER VOLUME IMAGE LISTEN_PORT
#
# Uses the Docker CLI as configured (DOCKER_HOST selects ThothDock). It
# refuses the default production container name and never deletes volumes.
set -euo pipefail
name=${1:?container}; volume=${2:?volume}; image=${3:?image}; port=${4:?listen port}
case "$name" in thothmemory-mini|thothmini-prod) echo "refusing a production container name" >&2; exit 2 ;; esac
work=$(mktemp -d); trap 'rm -rf "$work"' EXIT; chmod 700 "$work"
pass=0; fail=0
ok() { echo "PASS  $*"; pass=$((pass+1)); }
bad() { echo "FAIL  $*"; fail=$((fail+1)); }
ready() {
  local start; start=$(date +%s%N)
  for _ in $(seq 600); do
    if docker exec "$name" thothmemory-mini healthcheck >/dev/null 2>&1; then echo $(( ($(date +%s%N) - start) / 1000000 )); return 0; fi
    sleep 0.1
  done
  return 1
}
counts() { docker exec "$name" thothmemory-mini status | python3 -c 'import json,sys; d=json.load(sys.stdin); print(d["database"], json.dumps(d["counts"], sort_keys=True))'; }
# Pure /proc scan (no pgrep dependency, which a minimal guest may lack and which would
# then make this check pass vacuously): this install's mariadbd, by its config path.
orphans() { for p in /proc/[0-9]*; do tr '\0' ' ' < "$p/cmdline" 2>/dev/null | grep -q -- '--defaults-file=/etc/thothmini/my.cnf' && echo "${p#/proc/}"; done; true; }
run() { docker run -d --name "$name" -v "$volume:/data" -e THOTH_LISTEN="127.0.0.1:$port" "$image" >/dev/null; }

docker inspect "$name" >/dev/null 2>&1 || run
ms=$(ready) || { bad "service did not become ready"; exit 1; }
# Seed through the real MCP client. Empty schemas do not establish persistence.
docker exec "$name" sh -c '
  umask 077
  thothmemory-mini token local-agent > /run/thothmini/lifecycle-token
  printf "{\"endpoint\":\"http://127.0.0.1:%s/mcp\",\"token_file\":\"/run/thothmini/lifecycle-token\"}" "$1" > /run/thothmini/lifecycle-client.json
  thothmemory-mini demo --config /run/thothmini/lifecycle-client.json
  rm /run/thothmini/lifecycle-token /run/thothmini/lifecycle-client.json
' sh "$port" > "$work/demo.log"
project=$(sed -n 's/^Demo project: //p' "$work/demo.log")
[[ -n $project ]] || { bad "MCP seed did not return a project"; exit 1; }
fingerprint() { docker exec "$name" thothmemory-mini export "$project" | python3 -c 'import json,sys; d=json.load(sys.stdin); assert d["counts"]["events"] > 0; print(d["payload_sha256"])'; }
original=$(fingerprint)
base=$(counts) || { bad "owner status unavailable"; exit 1; }
[[ -n $base ]] || { bad "empty status"; exit 1; }
ok "ready in ${ms} ms; baseline $base"

vm=$(docker exec "$name" sh -c 'for p in /proc/[0-9]*; do [ "$(cat $p/comm 2>/dev/null)" = mariadbd ] && awk "/^VmSize/{print \$2}" $p/status; done; true' | head -1)
# Android kernels with 39-bit user addresses (512 GiB) cannot map MariaDB's default 8 TiB reservation.
if [[ -n $vm && $vm -lt 16777216 ]]; then ok "mariadbd virtual size $((vm/1024)) MiB (< 16 GiB; a 39-bit address space holds 512 GiB)"; else bad "mariadbd virtual size ${vm:-unknown} kB; would not fit a 39-bit address space"; fi
docker restart -t 60 "$name" >/dev/null; ms=$(ready) || bad "not ready after restart"
[[ $(counts) == "$base" && $(fingerprint) == "$original" ]] && ok "restart: ready in ${ms} ms, counts and seeded payload hash identical" || bad "restart changed data"
docker logs "$name" 2>&1 | grep -q "MariaDB stopped cleanly" && ok "restart shut MariaDB down cleanly" || bad "no clean MariaDB shutdown logged"

docker kill "$name" >/dev/null
gone=no; for i in $(seq 50); do [[ -z $(orphans) ]] && { gone=yes; break; }; sleep 0.1; done
[[ $gone == yes ]] && ok "SIGKILL: no MariaDB process left after $((i*100)) ms" || bad "MariaDB survived SIGKILL: $(orphans | tr '\n' ' ')"
echo "      exit state: $(docker inspect -f '{{.State.Status}} {{.State.ExitCode}}' "$name")"
docker start "$name" >/dev/null; ms=$(ready) || bad "not ready after SIGKILL"
[[ $(counts) == "$base" && $(fingerprint) == "$original" ]] && ok "SIGKILL recovery: ready in ${ms} ms, counts and seeded payload hash identical" || bad "data changed after SIGKILL"

docker rm -f "$name" >/dev/null; run; ms=$(ready) || bad "not ready after recreation"
[[ $(counts) == "$base" && $(fingerprint) == "$original" ]] && ok "recreated container on the same volume: ready in ${ms} ms, counts and seeded payload hash identical" || bad "recreation lost data"

docker exec "$name" thothmemory-mini backup > "$work/backup.sql" 2> "$work/backup.err" || { bad "backup failed: $(cat "$work/backup.err")"; }
sum=$(sed -n 's/^sha256=//p' "$work/backup.err"); actual=$(sha256sum "$work/backup.sql" | cut -d' ' -f1)
[[ -n $sum && $sum == "$actual" ]] && ok "backup: $(wc -c < "$work/backup.sql") bytes, sha256 matches" || bad "backup checksum mismatch"
bad_sum=$(printf '0%.0s' $(seq 64))
if docker exec -i "$name" thothmemory-mini restore --sha256 "$bad_sum" < "$work/backup.sql" >/dev/null 2>&1; then bad "restore accepted a wrong checksum"; else ok "restore refuses a wrong checksum before any change"; fi
docker exec -i "$name" thothmemory-mini restore --sha256 "$sum" < "$work/backup.sql" 2> "$work/restore.err" >/dev/null || bad "restore failed: $(cat "$work/restore.err")"
restored=$(grep -o 'thoth_restore_[0-9_]*' "$work/restore.err" | head -1)
[[ -n $restored ]] && ok "restored into new database $restored; active database untouched" || bad "restore did not report a database"
[[ $(counts) == "$base" ]] || bad "restore modified the active database"
docker exec "$name" thothmemory-mini activate "$restored" && docker restart -t 60 "$name" >/dev/null && ready >/dev/null
after=$(counts)
[[ ${after#* } == "${base#* }" && $after == "$restored "* && $(fingerprint) == "$original" ]] && ok "activated restore serves identical seeded payload" || bad "activated restore differs: $after"
docker exec "$name" thothmemory-mini activate thoth && docker restart -t 60 "$name" >/dev/null && ready >/dev/null
[[ $(counts) == "$base" ]] && ok "re-activated the original database" || bad "original database not restored as active"

docker stop -t 60 "$name" >/dev/null
[[ $(docker inspect -f '{{.State.ExitCode}}' "$name") == 0 ]] && ok "clean stop exits 0" || bad "stop exit code $(docker inspect -f '{{.State.ExitCode}}' "$name")"
[[ -z $(orphans) ]] && ok "no orphaned MariaDB process after stop" || bad "orphan after stop: $(orphans)"
docker start "$name" >/dev/null; ready >/dev/null || true
echo "$pass passed, $fail failed"
[[ $fail == 0 ]]
