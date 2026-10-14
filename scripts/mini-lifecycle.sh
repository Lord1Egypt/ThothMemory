#!/usr/bin/env bash
# Lifecycle gates for a DISPOSABLE ThothMemory Mini container: restart,
# SIGKILL recovery, recreation on the same volume, backup and verified
# restore into a new database, activation, clean stop and orphan checks.
#
#   scripts/mini-lifecycle.sh CONTAINER VOLUME IMAGE LISTEN_PORT
#
# Uses the Docker CLI as configured (DOCKER_HOST selects ThothDock). It
# refuses the default production container name and never deletes volumes.
#
# Optional environment:
#   PLATFORM=linux/arm64         run the image for that platform (QEMU is emulation)
#   THOTH_LIFECYCLE_MCP=1        publish the MCP port on 127.0.0.1 and verify the
#                                restored database through real MCP queries (needs python3)
#   THOTH_SKEW_GATE=1            also prove the backup survives a client clock that is
#                                behind the server (builds a throwaway libfaketime image;
#                                needs docker build and network access)
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
platform=(); [[ -z ${PLATFORM:-} ]] || platform=(--platform "$PLATFORM")
mcp=${THOTH_LIFECYCLE_MCP:-0}
run() {
  if [[ $mcp == 1 ]]; then
    docker run -d ${platform[@]+"${platform[@]}"} --name "$name" -v "$volume:/data" -p "127.0.0.1::$port" -e THOTH_LISTEN="0.0.0.0:$port" "$image" >/dev/null
  else
    docker run -d ${platform[@]+"${platform[@]}"} --name "$name" -v "$volume:/data" -e THOTH_LISTEN="127.0.0.1:$port" "$image" >/dev/null
  fi
}
# Owner SQL from stdin, inside the container. The password goes through a private
# option file, never an argument; the socket is private so TLS is off as in the app.
sql() {
  docker exec -i "$name" sh -c 'umask 077; f=$(mktemp /run/thothmini/.gate-XXXXXX)
    printf "[client]\nuser=root\nskip-ssl\nsocket=@thothmemory-mini-%s\npassword=%s\n" "$(cat /data/install-id)" "$(cat /data/secrets/db_owner_password)" > "$f"
    mariadb --defaults-extra-file="$f" -N -B; rc=$?; rm -f "$f"; exit $rc'
}
databases() { sql <<< "SELECT schema_name FROM information_schema.schemata WHERE schema_name LIKE 'thoth%' ORDER BY 1" | tr '\n' ' '; }

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
mcpcall() {
  local hp; hp=$(docker port "$name" "$port/tcp" | head -1)
  docker exec "$name" thothmemory-mini token local-agent > "$work/token"
  python3 "$(dirname "$0")/mini-recovery-mcp.py" "$1" --endpoint "http://127.0.0.1:${hp##*:}/mcp" --token-file "$work/token" --project "$project"
}
if [[ $mcp == 1 ]]; then
  mcpcall seed >/dev/null && ok "seeded evidence, a note and an idea through real MCP" || { bad "MCP seeding failed"; exit 1; }
  mcp_before=$(mcpcall read) || { bad "MCP read failed"; exit 1; }
fi
original=$(fingerprint)
base=$(counts) || { bad "owner status unavailable"; exit 1; }
[[ -n $base ]] || { bad "empty status"; exit 1; }
ok "ready in ${ms} ms; baseline $base"

vm=$(docker exec "$name" sh -c 'for p in /proc/[0-9]*; do [ "$(cat $p/comm 2>/dev/null)" = mariadbd ] && awk "/^VmSize/{print \$2}" $p/status; done; true' | head -1)
# Android kernels with 39-bit user addresses (512 GiB) cannot map MariaDB's default 8 TiB reservation.
if [[ -n $vm && $vm -lt 16777216 ]]; then ok "mariadbd virtual size $((vm/1024)) MiB (< 16 GiB; a 39-bit address space holds 512 GiB)"; else bad "mariadbd virtual size ${vm:-unknown} kB; would not fit a 39-bit address space"; fi
clean_count() { docker exec "$name" sh -c 'grep -c " clean$" /data/shutdown.log 2>/dev/null || true'; }
clean_before=$(clean_count)
docker restart -t 60 "$name" >/dev/null; ms=$(ready) || bad "not ready after restart"
[[ $(counts) == "$base" && $(fingerprint) == "$original" ]] && ok "restart: ready in ${ms} ms, counts and seeded payload hash identical" || bad "restart changed data"
# Graceful shutdown is proven by the supervisor's durable shutdown.log (written when mariadbd exits
# with status 0 after SIGTERM), not by container logs, whose final line can be lost at container exit.
[[ $(clean_count) -gt ${clean_before:-0} ]] && ok "restart shut MariaDB down cleanly (shutdown.log: $(docker exec "$name" tail -n 1 /data/shutdown.log))" || bad "no clean MariaDB shutdown recorded; last: $(docker exec "$name" tail -n 2 /data/shutdown.log 2>&1 | tr '\n' '|')"
repeat=${THOTH_RESTART_REPEAT:-5}
for i in $(seq "$repeat"); do
  before=$(clean_count); docker restart -t 60 "$name" >/dev/null; ready >/dev/null || { bad "not ready after repeated restart $i"; break; }
  [[ $(clean_count) -gt $before ]] || { bad "repeated restart $i recorded no clean shutdown: $(docker exec "$name" tail -n 2 /data/shutdown.log | tr '\n' '|')"; break; }
done
[[ $(counts) == "$base" && $(fingerprint) == "$original" ]] && ok "$repeat further consecutive restarts each recorded a clean shutdown; data identical" || bad "data changed across repeated restarts"

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
dbs_before=$(databases)

# A failed backup must not look like a valid artifact: non-zero exit and no checksum line.
if docker exec -e THOTH_DB_SOCKET=@thothmemory-mini-does-not-exist "$name" thothmemory-mini backup > "$work/failed.sql" 2> "$work/failed.err"; then
  bad "backup against an unreachable database exited 0"
elif grep -q '^sha256=' "$work/failed.err"; then bad "failed backup printed a checksum"
else ok "failed backup exits non-zero and prints no checksum"; fi
if docker exec -i "$name" thothmemory-mini restore --sha256 "$(sha256sum "$work/failed.sql" | cut -d' ' -f1)" < "$work/failed.sql" >/dev/null 2>&1; then
  bad "restore accepted the output of a failed backup"
else ok "output of a failed backup cannot be restored"; fi

# Corrupt and malformed backups are refused before activation and leave nothing behind.
bad_sum=$(printf '0%.0s' $(seq 64))
if docker exec -i "$name" thothmemory-mini restore --sha256 "$bad_sum" < "$work/backup.sql" >/dev/null 2>&1; then bad "restore accepted a wrong checksum"; else ok "restore refuses a wrong checksum before any change"; fi
python3 - "$work/backup.sql" "$work/corrupt.sql" <<'PY'
import sys
b = bytearray(open(sys.argv[1], "rb").read())
b[len(b) // 2] ^= 0x01
open(sys.argv[2], "wb").write(b)
PY
if docker exec -i "$name" thothmemory-mini restore --sha256 "$sum" < "$work/corrupt.sql" >/dev/null 2>&1; then bad "restore accepted a one-bit-corrupted backup"; else ok "one-bit-corrupted backup refused by checksum"; fi
printf 'THIS IS NOT SQL;\n' > "$work/malformed.sql"
if docker exec -i "$name" thothmemory-mini restore --sha256 "$(sha256sum "$work/malformed.sql" | cut -d' ' -f1)" < "$work/malformed.sql" >/dev/null 2>&1; then bad "restore accepted malformed SQL"; else ok "malformed backup with a matching checksum refused"; fi
[[ $(databases) == "$dbs_before" ]] && ok "refused restores left no database behind" || bad "databases changed after refused restores: $(databases)"

if [[ ${THOTH_SKEW_GATE:-0} == 1 ]]; then
  # Regression for "TLS/SSL error: certificate is not yet valid": the server's per-start
  # certificate is valid from the second it started; a client clock behind that fails TLS.
  printf 'FROM %s\nUSER 0\nRUN if command -v apt-get >/dev/null; then apt-get update -qq && apt-get install -y -qq --no-install-recommends faketime >/dev/null && rm -rf /var/lib/apt/lists/*; else apk add --no-cache libfaketime >/dev/null; fi\nUSER 65532:65532\n' "$image" \
    | docker build -q ${platform[@]+"${platform[@]}"} -t "thothmini-skew-$name" - >/dev/null || { bad "could not build the clock-skew fixture image"; }
  if docker run --rm ${platform[@]+"${platform[@]}"} --network "container:$name" -v "$volume:/data" --entrypoint faketime "thothmini-skew-$name" -f -5s thothmemory-mini backup > "$work/skew.sql" 2> "$work/skew.err" && grep -q '^sha256=' "$work/skew.err" && [[ -s $work/skew.sql ]]; then
    ok "backup succeeds with the client clock 5 s behind the server"
  else bad "backup failed with a skewed client clock: $(tail -2 "$work/skew.err")"; fi
  docker rmi -f "thothmini-skew-$name" >/dev/null 2>&1 || true
fi

docker exec -i "$name" thothmemory-mini restore --sha256 "$sum" < "$work/backup.sql" 2> "$work/restore.err" >/dev/null || bad "restore failed: $(cat "$work/restore.err")"
restored=$(grep -o 'thoth_restore_[0-9_]*' "$work/restore.err" | head -1)
[[ -n $restored ]] && ok "restored into new database $restored; active database untouched" || bad "restore did not report a database"
[[ $(counts) == "$base" ]] || bad "restore modified the active database"

# Compare every table (row content checksum), the applied migrations and the foreign keys.
tables=$(sql <<< "SELECT table_name FROM information_schema.tables WHERE table_schema='thoth' AND table_type='BASE TABLE' ORDER BY 1")
mismatch=""; n=0
for t in $tables; do
  n=$((n+1))
  a=$(sql <<< "CHECKSUM TABLE thoth.\`$t\`" | cut -f2); b=$(sql <<< "CHECKSUM TABLE \`$restored\`.\`$t\`" | cut -f2)
  [[ -n $a && $a == "$b" ]] || mismatch="$mismatch $t"
done
[[ $n -gt 0 && -z $mismatch ]] && ok "restored copy matches the original in all $n tables (row checksums)" || bad "restored tables differ from the original:${mismatch:- none listed ($n tables)}"
mig_a=$(sql <<< "SELECT GROUP_CONCAT(version,':',status ORDER BY version) FROM thoth.schema_migrations")
mig_b=$(sql <<< "SELECT GROUP_CONCAT(version,':',status ORDER BY version) FROM \`$restored\`.schema_migrations")
[[ -n $mig_a && $mig_a == "$mig_b" ]] && ok "restored schema_migrations identical ($mig_a)" || bad "schema_migrations differ: '$mig_a' vs '$mig_b'"
orphan_queries() {
  sql <<< "SELECT CONCAT('SELECT \"', k.table_name, '.', k.constraint_name, '\", COUNT(*) FROM \`$1\`.\`', k.table_name, '\` c LEFT JOIN \`$1\`.\`', k.referenced_table_name, '\` p ON ', GROUP_CONCAT(CONCAT('c.\`', k.column_name, '\`=p.\`', k.referenced_column_name, '\`') SEPARATOR ' AND '), ' WHERE p.\`', MIN(k.referenced_column_name), '\` IS NULL AND ', GROUP_CONCAT(CONCAT('c.\`', k.column_name, '\` IS NOT NULL') SEPARATOR ' AND '), ';') FROM information_schema.key_column_usage k WHERE k.table_schema='$1' AND k.referenced_table_name IS NOT NULL GROUP BY k.table_name, k.constraint_name, k.referenced_table_name"
}
fk_total=$(orphan_queries "$restored" | wc -l); fk_orig=$(orphan_queries thoth | wc -l)
orphans_found=$(orphan_queries "$restored" | sql | awk '$NF != 0')
[[ $fk_total -gt 0 && $fk_total == "$fk_orig" && -z $orphans_found ]] && ok "restored copy keeps all $fk_total foreign keys with no orphan rows" || bad "foreign key check failed: restored=$fk_total original=$fk_orig orphans='$orphans_found'"

docker exec "$name" thothmemory-mini activate "$restored" && docker restart -t 60 "$name" >/dev/null && ready >/dev/null
after=$(counts)
[[ ${after#* } == "${base#* }" && $after == "$restored "* && $(fingerprint) == "$original" ]] && ok "activated restore serves identical seeded payload" || bad "activated restore differs: $after"
if [[ $mcp == 1 ]]; then
  [[ $(mcpcall read) == "$mcp_before" ]] && ok "real MCP queries on the restored copy return identical context, tasks, handoff and search results" || bad "MCP queries differ on the restored copy"
fi
docker exec "$name" thothmemory-mini activate thoth && docker restart -t 60 "$name" >/dev/null && ready >/dev/null
[[ $(counts) == "$base" ]] && ok "re-activated the original database" || bad "original database not restored as active"

docker stop -t 60 "$name" >/dev/null
[[ $(docker inspect -f '{{.State.ExitCode}}' "$name") == 0 ]] && ok "clean stop exits 0" || bad "stop exit code $(docker inspect -f '{{.State.ExitCode}}' "$name")"
[[ -z $(orphans) ]] && ok "no orphaned MariaDB process after stop" || bad "orphan after stop: $(orphans)"
docker start "$name" >/dev/null; ready >/dev/null || true
echo "$pass passed, $fail failed"
[[ $fail == 0 ]]
