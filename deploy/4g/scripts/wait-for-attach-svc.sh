#!/usr/bin/env bash
# Wait until UE container <service> has an IP on tun_srsue. Prints the IP.
# The virtual radio runs slower than real time on a busy host, so a UE can take well over a
# minute to attach; only a UE that has not attached after ATTACH_TIMEOUT seconds (default 180)
# counts as failed.
set -euo pipefail
COMPOSE="docker compose -f deploy/4g/docker-compose.yml -f deploy/4g/docker-compose.multi.yml"
SVC="$1"
TIMEOUT="${ATTACH_TIMEOUT:-180}"
start=$(date +%s)
while :; do
  ip=$($COMPOSE exec -T "$SVC" ip -4 addr show tun_srsue 2>/dev/null \
        | awk '/inet /{print $2}') || true
  elapsed=$(( $(date +%s) - start ))
  if [ -n "${ip:-}" ]; then echo "$SVC attached: $ip (${elapsed} s)"; exit 0; fi
  if [ "$elapsed" -ge "$TIMEOUT" ]; then break; fi
  if [ $(( elapsed % 15 )) -eq 0 ] && [ "$elapsed" -gt 0 ]; then
    echo "  $SVC not attached yet (${elapsed} s)"
  fi
  sleep 1
done
echo "$SVC did NOT attach within ${TIMEOUT} s" >&2; exit 1
