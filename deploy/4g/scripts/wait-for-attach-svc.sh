#!/usr/bin/env bash
# Wait until UE container <service> has an IP on tun_srsue. Prints the IP.
set -euo pipefail
COMPOSE="docker compose -f deploy/4g/docker-compose.yml -f deploy/4g/docker-compose.multi.yml"
SVC="$1"
for _ in $(seq 1 40); do
  ip=$($COMPOSE exec -T "$SVC" ip -4 addr show tun_srsue 2>/dev/null \
        | awk '/inet /{print $2}') || true
  if [ -n "${ip:-}" ]; then echo "$SVC attached: $ip"; exit 0; fi
  sleep 1
done
echo "$SVC did NOT attach" >&2; exit 1
