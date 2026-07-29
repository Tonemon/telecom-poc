#!/usr/bin/env bash
# Poll the UE container until the tun_srsue interface has an IP from 10.45.0.0/16.
set -euo pipefail
COMPOSE="docker compose -f deploy/4g/docker-compose.yml"
for i in $(seq 1 60); do
  if $COMPOSE exec -T ue ip -4 addr show tun_srsue 2>/dev/null | grep -q "inet 10.45."; then
    echo "UE attached: $($COMPOSE exec -T ue ip -4 addr show tun_srsue | grep inet | tr -s ' ')"
    exit 0
  fi
  sleep 2
done
echo "UE failed to attach within timeout" >&2
$COMPOSE logs --tail 50 ue enb >&2
exit 1
