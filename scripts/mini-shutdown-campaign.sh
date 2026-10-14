#!/usr/bin/env bash
# Bounded reproducibility campaign for the Mini shutdown record (issue #15). DISPOSABLE container only.
#   scripts/mini-shutdown-campaign.sh IMAGE RESTARTS STOPS KILLS [PLATFORM]  > result.jsonl
# Per iteration it prints one JSON line; the last line is a summary. Outcomes of /data/shutdown.log:
#   clean | exited-before-stop | error | killed-after-60s | missing (no new line) | expected-missing (after SIGKILL)
set -euo pipefail
image=${1:?image}; restarts=${2:?}; stops=${3:?}; kills=${4:?}; platform=${5:-}
name=mini-campaign-$$; vol=$name-vol
plat=(); [[ -z $platform ]] || plat=(--platform "$platform")
cleanup() { docker rm -f -v "$name" >/dev/null 2>&1 || true; docker volume rm "$vol" >/dev/null 2>&1 || true; }
trap cleanup EXIT
docker run -d ${plat[@]+"${plat[@]}"} --name "$name" -v "$vol:/data" -e THOTH_LISTEN=127.0.0.1:7319 "$image" >/dev/null
ready() { for _ in $(seq 600); do docker exec "$name" thothmemory-mini healthcheck >/dev/null 2>&1 && return 0; sleep 0.1; done; return 1; }
# The log is capped at 50 lines, so line counts stop growing; compare the newest full line (timestamp + outcome).
lastfull() { docker exec "$name" sh -c 'tail -n 1 /data/shutdown.log 2>/dev/null || true'; }
last() { lastfull | cut -d' ' -f2-; }
orphan() { for p in /proc/[0-9]*; do tr '\0' ' ' < "$p/cmdline" 2>/dev/null | grep -q -- '--defaults-file=/etc/thothmini/my.cnf' && echo "${p#/proc/}"; done | wc -l; }
declare -A tally
iter=0
run() { # kind
  local kind=$1 before after outcome exitcode="" t0; iter=$((iter+1)); before=$(lastfull); t0=$(date +%s%N)
  case $kind in
    restart) docker restart -t 60 "$name" >/dev/null ;;
    stop) docker stop -t 60 "$name" >/dev/null; exitcode=$(docker inspect -f '{{.State.ExitCode}}' "$name"); docker start "$name" >/dev/null ;;
    kill) docker kill "$name" >/dev/null; exitcode=$(docker inspect -f '{{.State.ExitCode}}' "$name"); docker start "$name" >/dev/null ;;
  esac
  ready || { echo "{\"i\":$iter,\"kind\":\"$kind\",\"outcome\":\"not-ready\"}"; tally[not-ready]=$(( ${tally[not-ready]:-0} + 1 )); return; }
  after=$(lastfull)
  if [[ $kind == kill ]]; then outcome=$([[ $after == "$before" ]] && echo expected-missing || echo "unexpected-line:$(last)")
  elif [[ $after != "$before" ]]; then outcome=$(last | sed 's/:.*//; s/ .*//'); else outcome=missing; fi
  tally[$outcome]=$(( ${tally[$outcome]:-0} + 1 ))
  echo "{\"i\":$iter,\"kind\":\"$kind\",\"outcome\":\"$outcome\",\"container_exit_code\":\"$exitcode\",\"ms\":$(( ($(date +%s%N)-t0)/1000000 ))}"
}
ready || { echo '{"error":"first start not ready"}'; exit 1; }
for _ in $(seq "$restarts"); do run restart; done
for _ in $(seq "$stops"); do run stop; done
for _ in $(seq "$kills"); do run kill; done
sum="{\"summary\":true,\"image\":\"$image\",\"platform\":\"${platform:-native}\",\"restarts\":$restarts,\"stops\":$stops,\"kills\":$kills,\"iterations\":$iter,\"outcomes\":{"
first=1; for k in "${!tally[@]}"; do [[ $first == 1 ]] || sum+=","; first=0; sum+="\"$k\":${tally[$k]}"; done
echo "$sum},\"host_orphan_mariadbd_after\":$(docker stop -t 60 "$name" >/dev/null; orphan)}"
