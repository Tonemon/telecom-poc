#!/usr/bin/env bash
# Wait until every given eNB/UE service has finished starting its radio, so the air (which
# puts all of them in one sample lockstep) can start without waiting on a half-started peer.
#   eNB ready: "==== eNodeB started ==="
#   UE  ready: "Attaching UE..." (printed right after PHY init; srsue writes "Waiting PHY to
#              initialize ... " and "done!" separately, so other log lines can split them)
# The first boot on a host can take several minutes (srsRAN benchmarks its FFTs once and caches
# the result in deploy/4g/.fftw/); later boots take seconds.
# Usage: wait-radio-ready.sh <service>...   (timeout: RADIO_READY_TIMEOUT seconds, default 600)
set -euo pipefail
COMPOSE="docker compose -f deploy/4g/docker-compose.yml -f deploy/4g/docker-compose.multi.yml"
TIMEOUT="${RADIO_READY_TIMEOUT:-600}"

marker() {
  case "$1" in
    enb*) echo "==== eNodeB started ===" ;;
    *)    echo "Attaching UE..." ;;
  esac
}

start=$(date +%s)
pending=("$@")
while [ "${#pending[@]}" -gt 0 ]; do
  still=()
  for svc in "${pending[@]}"; do
    if $COMPOSE logs --no-color "$svc" 2>/dev/null | grep -qF "$(marker "$svc")"; then
      echo "  $svc radio ready ($(( $(date +%s) - start )) s)"
    else
      still+=("$svc")
    fi
  done
  pending=("${still[@]+"${still[@]}"}")
  [ "${#pending[@]}" -eq 0 ] && break
  elapsed=$(( $(date +%s) - start ))
  if [ "$elapsed" -ge "$TIMEOUT" ]; then
    echo "radio NOT ready after ${TIMEOUT} s: ${pending[*]}" >&2
    exit 1
  fi
  if [ $(( elapsed % 15 )) -lt 2 ] && [ "$elapsed" -ge 15 ]; then
    echo "  waiting for ${pending[*]} (${elapsed} s; the first boot benchmarks FFTs, later boots are fast)"
  fi
  sleep 2
done
