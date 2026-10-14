#!/usr/bin/env bash
# Regression for the Compose production-volume collision, upgrade compatibility and backup safety.
# Runs ONLY against a disposable daemon (scripts/compose-isolation-dind.sh sets THOTH_DISPOSABLE_DAEMON=1).
#
# Fixture: a REAL historical installation made with the published legacy compose file and image
# (project `thothmemory`, volume `thothmemory_memory`, seeded through MCP), then stopped, exactly like
# the owner's production. Fresh installs, repeats and every direct-Compose misuse must leave its volume
# byte-identical and its containers unchanged. Afterwards it is upgraded deliberately (legacy -> next
# published -> candidate through `adopt-volume`), with backup/restore and a downgrade attempt.
set -uo pipefail
repo=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
: "${THOTH_NEW_IMAGE:?local candidate Full image}" "${THOTH_LEGACY_IMAGE:?published legacy Full image}" "${THOTH_MID_IMAGE:?next published Full image}"
OLD=${THOTH_OLDEST_IMAGE:-}
[[ ${THOTH_DISPOSABLE_DAEMON:-0} == 1 ]] || { echo "refusing: run through scripts/compose-isolation-dind.sh (disposable daemon only)" >&2; exit 2; }
PG=$(sed -n 's/^const GuardImage = "\(.*\)"/\1/p' "$repo/internal/cli/volume.go")
docker volume inspect thothmemory_memory >/dev/null 2>&1 && { echo "refusing: thothmemory_memory already exists on this daemon (not a disposable daemon?)" >&2; exit 2; }
[[ -z $(docker ps -aq) ]] || { echo "refusing: containers already exist on this daemon" >&2; exit 2; }
work=$(mktemp -d -p "${THOTH_WORK_ROOT:-/work}"); trap 'rm -rf "$work"' EXIT
pass=0; fail=0
ok() { echo "PASS  $*"; pass=$((pass+1)); }
bad() { echo "FAIL  $*"; fail=$((fail+1)); }
cp "${THOTH_CLI_BIN:?candidate CLI}" "$work/thothmemory"; cp "${THOTH_LEGACY_CLI:?legacy CLI}" "$work/legacy-thothmemory"; chmod +x "$work"/*thothmemory
vol_hash() { docker run --rm --network none -v "$1:/v:ro" --entrypoint sh "$PG" -c 'cd /v && find . -type f | sort | xargs sha256sum | sha256sum'; }
cstate() { docker inspect -f '{{.Id}} {{.State.Status}} {{.State.StartedAt}}' "$1" 2>/dev/null || echo missing; }
newdir() { local d="$work/$1"; mkdir -p "$d/bin"; cp "$repo/compose.yaml" "$d/"; cp "$work/thothmemory" "$d/bin/"; echo "$d"; }
sig() { (cd "$1" && docker compose exec -T db psql -U postgres -d "${2:-thoth}" -Atc "SELECT (SELECT count(*) FROM projects)||','||(SELECT count(*) FROM tasks)||','||(SELECT count(*) FROM sessions)||','||(SELECT count(*) FROM events)||','||(SELECT count(*) FROM records)||','||(SELECT count(*) FROM evidence)||','||md5(coalesce((SELECT string_agg(id::text||body,'|' ORDER BY id) FROM events),''))||','||md5(coalesce((SELECT string_agg(id::text||title||state,'|' ORDER BY id) FROM tasks),''))" 2>&1); }
ready() { (cd "$1" && docker compose up -d --wait --wait-timeout 240 >"$work/up.out" 2>&1); }

# ---------------------------------------------------------------- historical fixture (real legacy install)
L="$work/legacy"; mkdir -p "$L/bin"; cp "$work/legacy-thothmemory" "$L/bin/thothmemory"; cp "${THOTH_LEGACY_COMPOSE:?legacy compose.yaml}" "$L/compose.yaml"
(cd "$L" && THOTH_IMAGE=$THOTH_LEGACY_IMAGE THOTH_PORT=17400 ./bin/thothmemory init-config >/dev/null 2>&1; chmod 700 .secrets; chmod 644 .secrets/db_owner_password .secrets/db_app_password .secrets/tokens.json)
ready "$L" && ok "legacy installation (published image, legacy compose, project thothmemory, volume thothmemory_memory) is up" || { bad "legacy install failed: $(tail -5 "$work/up.out")"; exit 1; }
printf '{"endpoint":"http://127.0.0.1:7319/mcp","token_file":"/w/.secrets/agent_token"}' > "$L/dc.json"
docker run --rm --network container:thothmemory-app-1 -v "$L:/w:ro" --entrypoint /w/bin/thothmemory "$PG" demo --config /w/dc.json >"$work/demo.out" 2>&1 && ok "seeded the legacy installation through real MCP ($(grep -c OK "$work/demo.out") agent sessions)" || bad "seeding failed: $(tail -3 "$work/demo.out")"
(cd "$L" && docker compose exec -T db psql -U postgres -d thoth -Atc "INSERT INTO evidence(project_id,source_kind,locator,git_revision,outcome,summary,reported_by) SELECT id,'test','test://fixture',repeat('a',40),'pass','fixture evidence',(SELECT id FROM principals ORDER BY id LIMIT 1) FROM projects LIMIT 1" >/dev/null 2>&1) && ok "added an evidence record" || bad "evidence insert failed"
S0=$(sig "$L"); echo "      legacy signature: $S0"
[[ $S0 == 1,1,4,* ]] && ok "legacy data present (projects,tasks,sessions,events,records,evidence,hashes)" || bad "unexpected legacy signature: $S0"
(cd "$L" && docker compose stop >/dev/null 2>&1)
H0=$(vol_hash thothmemory_memory); C0DB=$(cstate thothmemory-db-1); C0APPSTART=$(docker inspect -f '{{.State.StartedAt}}' thothmemory-app-1); [[ $(docker ps -q | wc -l) == 0 ]] && ok "legacy installation stopped, like the owner's production"
# Data is the requirement: byte-identical volume, nothing from the historical project running or ever
# started again, and the database container untouched. Compose may replace a stopped historical `app`
# container with a never-started one when the project name is deliberately forced to `thothmemory`
# (documented limitation; stateless, never started, no data touched).
unchanged() {
  local h dbs running appstart; h=$(vol_hash thothmemory_memory); dbs=$(cstate thothmemory-db-1)
  running=$(docker ps -q --filter label=com.docker.compose.project=thothmemory | wc -l)
  appstart=$(docker inspect -f '{{.State.StartedAt}}' thothmemory-app-1 2>/dev/null || echo missing)
  if [[ $h == "$H0" && $dbs == "$(cstate thothmemory-db-1 | cat)" && $dbs == "$C0DB" && $running == 0 && ( $appstart == "$C0APPSTART" || $appstart == 0001-01-01T00:00:00Z ) ]]; then return 0; fi
  echo "      state diff: volume $([[ $h == "$H0" ]] && echo same || echo CHANGED); db: $dbs (was $C0DB); running historical containers: $running; app start: $appstart (was $C0APPSTART)"; return 1
}
volumes_before=$(docker volume ls -q | sort)

# ---------------------------------------------------------------- 1. direct Compose with no configuration
d=$(newdir bare); (cd "$d" && docker compose up -d >"$work/bare.out" 2>&1); rc=$?
[[ $rc != 0 ]] && grep -q 'required variable' "$work/bare.out" && [[ $(docker volume ls -q | sort) == "$volumes_before" ]] && ok "bare 'docker compose up' fails before creating anything ($(grep -o 'required variable [A-Z_]*' "$work/bare.out" | head -1))" || bad "bare compose up: rc=$rc $(head -c 300 "$work/bare.out")"
unchanged && ok "historical volume byte-identical and containers unchanged after the bare invocation" || bad "bare invocation changed historical state"

# ---------------------------------------------------------------- 2. fresh installation
d=$(newdir fresh)
(cd "$d" && THOTH_IMAGE=$THOTH_NEW_IMAGE THOTH_PORT=17401 ./bin/thothmemory init-config >/dev/null 2>"$work/init.err") || bad "init-config: $(cat "$work/init.err")"
chmod 700 "$d/.secrets"; chmod 644 "$d"/.secrets/db_owner_password "$d"/.secrets/db_app_password "$d"/.secrets/tokens.json
project=$(sed -n 's/^COMPOSE_PROJECT_NAME=//p' "$d/.env"); volume=$(sed -n 's/^THOTH_VOLUME=//p' "$d/.env")
[[ -n $project && $project != thothmemory && -n $volume && $volume != thothmemory_memory ]] && ok "init-config selected an isolated identity ($project / $volume)" || bad "identity: '$project' '$volume'"
ready "$d" && ok "fresh install became healthy" || bad "fresh install failed: $(tail -5 "$work/up.out")"
unchanged && ok "historical volume byte-identical; historical containers neither started nor recreated" || bad "fresh install changed historical state"
[[ -n $(docker volume ls -q --filter "name=^$volume$") ]] && ok "fresh install created its own volume" || bad "own volume missing"
[[ $(docker run --rm --network none -v "$volume:/v:ro" --entrypoint cat "$PG" /v/.thothmemory-volume) == "thothmemory-volume-v1 $volume $project" ]] && ok "own volume carries its ownership marker (volume + project)" || bad "marker missing or wrong"
(cd "$d" && docker compose exec -T db psql -U postgres -d thoth -Atc "CREATE TABLE iso_sentinel(v text); INSERT INTO iso_sentinel VALUES ('fresh-data')" >/dev/null 2>&1) && ok "wrote data to the fresh install" || bad "could not write to fresh DB"
envsum=$(sha256sum "$d/.env" | cut -c1-64)

# ---------------------------------------------------------------- 3. repeated install, restart
(cd "$d" && THOTH_IMAGE=$THOTH_NEW_IMAGE ./bin/thothmemory init-config >/dev/null 2>&1 && docker compose up -d --wait --wait-timeout 240 >/dev/null 2>&1 && docker compose restart >/dev/null 2>&1) && ready "$d" && [[ $(sha256sum "$d/.env" | cut -c1-64) == "$envsum" ]] && ok "repeated install and restart are idempotent; .env unchanged" || bad "repeat install failed or changed .env"
[[ $(cd "$d" && docker compose exec -T db psql -U postgres -d thoth -Atc 'SELECT v FROM iso_sentinel') == fresh-data ]] && ok "data survived restart" || bad "data lost after restart"
unchanged && ok "historical state still untouched after repeat" || bad "repeat changed historical state"

# ---------------------------------------------------------------- 4. conflicting configurations fail closed
conflict() { # label, dir, project whose guard should refuse, compose args, env assignments...
  local label=$1 dir=$2 proj=$3 args=$4; shift 4
  (cd "$dir" && env "$@" docker compose $args up -d >"$work/c.out" 2>&1); local rc=$?
  local g; g=$(docker logs "$proj-guard-1" 2>&1 | grep -c REFUSING)
  local started=0 c
  for c in migrate app; do docker inspect -f '{{.State.StartedAt}}' "$proj-$c-1" 2>/dev/null | grep -qv '^0001-01-01' && [[ $proj != thothmemory && $proj != "$project" ]] && started=$((started+1)); done
  if [[ $rc != 0 && $g -ge 1 && $started == 0 ]] && unchanged; then ok "$label: guard refused; migrate/app never started; historical state untouched"; else bad "$label: rc=$rc guard_refusals=$g started=$started $(tail -c 700 "$work/c.out")"; fi
  docker rm -f -v "$proj-guard-1" >/dev/null 2>&1
}
bad_env() { local d2; d2=$(newdir "$1"); cp -r "$d/.secrets" "$d2/"; printf 'THOTH_IMAGE=%s\nTHOTH_PORT=17402\nTHOTH_DATABASE=thoth\nCOMPOSE_PROJECT_NAME=%s\nTHOTH_VOLUME=%s\n' "$THOTH_NEW_IMAGE" "$2" "$3" > "$d2/.env"; echo "$d2"; }
conflict "the exact historical configuration (project thothmemory + volume thothmemory_memory)" "$(bad_env hist thothmemory thothmemory_memory)" thothmemory "" X=1
conflict "a new project name with the historical volume" "$(bad_env hist2 thothmemory-other thothmemory_memory)" thothmemory-other "" X=1
conflict "-p thothmemory overriding .env" "$(bad_env hist3 thothmemory-x thothmemory_memory)" thothmemory "-p thothmemory" X=1
conflict "COMPOSE_PROJECT_NAME environment override" "$(bad_env hist4 thothmemory-y thothmemory_memory)" thothmemory "" COMPOSE_PROJECT_NAME=thothmemory
conflict "THOTH_VOLUME environment override on a healthy install" "$d" "$project" "" THOTH_VOLUME=thothmemory_memory
ready "$d"
[[ $(cd "$d" && docker compose exec -T db psql -U postgres -d thoth -Atc 'SELECT v FROM iso_sentinel') == fresh-data ]] && ok "fresh install's data intact after the conflicting attempts (a rejected override may stop its containers; setup restores them)" || bad "fresh install data affected"
unchanged && ok "historical state still untouched after all conflicting attempts" || bad "historical state changed"
d3=$(bad_env other thothmemory-z "$volume"); (cd "$d3" && docker compose up -d >"$work/o.out" 2>&1); rc=$?
[[ $rc != 0 ]] && docker logs thothmemory-z-guard-1 2>&1 | grep -q 'different installation' && ok "a volume owned by another installation/project is refused" || bad "foreign-marked volume accepted (rc=$rc)"

# ---------------------------------------------------------------- 5. deliberate adoption is guarded
d4=$(newdir adopt); printf 'THOTH_IMAGE=%s\nTHOTH_PORT=17403\nTHOTH_DATABASE=thoth\n' "$THOTH_NEW_IMAGE" > "$d4/.env"; cp -r "$L/.secrets" "$d4/"
(cd "$d4" && THOTH_IMAGE=$THOTH_NEW_IMAGE ./bin/thothmemory init-config >/dev/null 2>"$work/legacy.err"); grep -q adopt-volume "$work/legacy.err" && ok "init-config refuses to choose a volume for an older .env and names the adoption command" || bad "legacy .env not refused: $(cat "$work/legacy.err")"
(cd "$d4" && ./bin/thothmemory adopt-volume thothmemory_memory >"$work/a1.out" 2>&1); [[ $? != 0 ]] && unchanged && ! grep -q THOTH_VOLUME "$d4/.env" && ok "adopt-volume without --confirm refuses and changes nothing" || bad "adopt without confirm"
(cd "$d4" && ./bin/thothmemory adopt-volume thothmemory_memory --confirm wrong >"$work/a2.out" 2>&1); [[ $? != 0 ]] && unchanged && ok "adopt-volume with a wrong confirmation refuses" || bad "adopt wrong confirm"
docker volume create iso_empty >/dev/null; (cd "$d4" && ./bin/thothmemory adopt-volume iso_empty --confirm iso_empty >"$work/a3.out" 2>&1); rc=$?; [[ $rc != 0 ]] && grep -q 'not adopted' "$work/a3.out" && ok "adopt-volume refuses a volume that is not a PostgreSQL data volume" || bad "empty volume: rc=$rc $(cat "$work/a3.out")"

# ---------------------------------------------------------------- 6. upgrade 1: legacy compose, legacy volume, next published image (v0.2.1 -> v0.2.2 style)
printf 'THOTH_IMAGE=%s\nTHOTH_PORT=17400\nTHOTH_DATABASE=thoth\n' "$THOTH_MID_IMAGE" > "$L/.env.next"; cp "$L/.env" "$L/.env.orig"; sed -i "s#^THOTH_IMAGE=.*#THOTH_IMAGE=$THOTH_MID_IMAGE#" "$L/.env"
ready "$L" && ok "legacy compose + legacy volume upgraded to the next published image (migrations applied)" || bad "upgrade 1 failed: $(tail -5 "$work/up.out")"
S1=$(sig "$L"); [[ $S1 == "$S0" ]] && ok "upgrade 1: projects, tasks, sessions, events, records, evidence and content hashes identical" || bad "upgrade 1 data differs: $S0 vs $S1"
(cd "$L" && ./bin/thothmemory backup "$work/pre.dump" >/dev/null 2>&1) && ok "upgrade 1: backup works on the upgraded installation" || bad "backup after upgrade 1 failed"
(cd "$L" && docker compose stop >/dev/null 2>&1)

# ---------------------------------------------------------------- 7. upgrade 2: deliberate adoption into the isolated compose, candidate image
d5=$(newdir upgrade); cp -r "$L/.secrets" "$d5/"; cp "$L/.env.orig" "$d5/.env"; sed -i "s#^THOTH_IMAGE=.*#THOTH_IMAGE=$THOTH_NEW_IMAGE#" "$d5/.env"
(cd "$d5" && ./bin/thothmemory adopt-volume thothmemory_memory --confirm thothmemory_memory >"$work/ad.out" 2>&1) && ok "explicit adoption marks the volume and records project + volume in .env (no migration run)" || bad "adoption failed: $(cat "$work/ad.out")"
grep -q '^COMPOSE_PROJECT_NAME=thothmemory$' "$d5/.env" && grep -q '^THOTH_VOLUME=thothmemory_memory$' "$d5/.env" && ok ".env now names the legacy project and volume explicitly" || bad ".env lacks explicit identity"
ready "$d5" && ok "adopted installation started on the candidate image; legacy containers upgraded in place" || bad "upgrade 2 failed: $(tail -8 "$work/up.out")"
S2=$(sig "$d5"); [[ $S2 == "$S0" ]] && ok "upgrade 2: all historical data identical after adoption and migration" || bad "upgrade 2 data differs: $S0 vs $S2"
(cd "$d5" && ./bin/thothmemory backup "$work/post.dump" >/dev/null 2>&1) && ok "backup after upgrade" || bad "backup after upgrade failed"
(cd "$d5" && ./bin/thothmemory restore "$work/post.dump" --confirm >"$work/restore.out" 2>&1); restored=$(sed -n 's/^Restored into //p' "$work/restore.out")
[[ -n $restored && $(sig "$d5" "$restored") == "$S0" ]] && ok "restore into an isolated database ($restored) reproduces the original signature" || bad "restore mismatch: $(cat "$work/restore.out" | tail -3)"
[[ $(sig "$d5") == "$S0" ]] && ok "active database unchanged by the restore" || bad "restore touched the active database"
priv=$(cd "$d5" && docker compose exec -T db psql -U postgres -d thoth -Atc "SELECT NOT has_table_privilege('thoth_app','attestations','INSERT') AND NOT has_schema_privilege('thoth_app','public','CREATE')"); [[ $priv == t ]] && ok "runtime credential remains least-privilege after upgrade" || bad "runtime privileges wrong: $priv"
perm=$(stat -c %a "$d5/.secrets"); [[ $perm == 700 ]] && ok "credential directory still private (700)" || bad ".secrets mode $perm"
S2b=$(sig "$d5")
# a database written by a newer release is refused by this build, and nothing changes
(cd "$d5" && docker compose exec -T db psql -U postgres -d thoth -Atc "INSERT INTO schema_migrations(version,checksum) VALUES('999_from_a_future_release.sql','x')" >/dev/null 2>&1)
(cd "$d5" && docker compose run --rm migrate >"$work/down.out" 2>&1); rc=$?
[[ $rc != 0 ]] && grep -q 'newer than this build' "$work/down.out" && ok "this build refuses to migrate a database written by a newer release (rc=$rc)" || bad "newer-schema database accepted (rc=$rc): $(tail -2 "$work/down.out")"
(cd "$d5" && docker compose exec -T db psql -U postgres -d thoth -Atc "DELETE FROM schema_migrations WHERE version='999_from_a_future_release.sql'" >/dev/null 2>&1)
[[ $(sig "$d5") == "$S2b" ]] && ok "the refused attempt left the data intact" || bad "refused attempt changed data"
if [[ -n $OLD ]]; then
  (cd "$d5" && THOTH_IMAGE=$OLD docker compose run --rm migrate >"$work/old.out" 2>&1); echo "      informational: the already-released oldest image's migrate returned rc=$? against the upgraded database (released builds cannot be changed)"
  [[ $(sig "$d5") == "$S2b" ]] && ok "data intact after the oldest image's attempt" || bad "oldest-image attempt changed data"
fi
(cd "$d5" && docker compose down >/dev/null 2>&1)
[[ $(docker run --rm --network none -v thothmemory_memory:/v:ro --entrypoint cat "$PG" /v/.thothmemory-volume) == "thothmemory-volume-v1 thothmemory_memory thothmemory" ]] && ok "adopted volume carries the explicit marker" || bad "adopted marker missing"

echo "$pass passed, $fail failed"
(cd "$d" && docker compose down -v >/dev/null 2>&1); docker volume rm "$volume" >/dev/null 2>&1
[[ $fail == 0 ]]
