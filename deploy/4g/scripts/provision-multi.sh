#!/usr/bin/env bash
# Provision the 3 fixed subscribers for the multi-UE topology (keys match config/multi/ue*.conf).
set -euo pipefail
COMPOSE="docker compose -f deploy/4g/docker-compose.yml -f deploy/4g/docker-compose.multi.yml"
TOKEN="dev-operator-token"

telctl() { $COMPOSE exec -T -e TELCOCTL_TOKEN="$TOKEN" \
  -e TELCOCTL_SERVER="http://127.0.0.1:8080" provisioner telcoctl "$@"; }
exists() { telctl get "$1" >/dev/null 2>&1; }
add() { # imsi k opc
  if exists "$1"; then echo "  $1 already present — skipping";
  else telctl add --imsi "$1" --ki "$2" --opc "$3" --apn internet \
    --reason NEW_ACTIVATION --note "multi-ue demo"; fi
}

echo "Waiting for provisioner..."
for _ in $(seq 1 30); do
  $COMPOSE exec -T provisioner wget -qO- http://127.0.0.1:8080/healthz >/dev/null 2>&1 && break
  sleep 1
done
add 999700000000001 465B5CE8B199B49FAA5F0A2EE238A6BC E8ED289DEBA952E4283B54E88E6183CA
add 999700000000002 00112233445566778899AABBCCDDEEFF 000102030405060708090A0B0C0D0E0F
add 999700000000003 8899AABBCCDDEEFF0011223344556677 0F0E0D0C0B0A09080706050403020100
echo "Multi-UE subscribers provisioned."
