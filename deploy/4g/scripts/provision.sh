#!/usr/bin/env bash
# Provision the test subscriber via the Go provisioner (telcoctl -> REST -> Mongo).
# Uses the SAME key material as deploy/4g/config/ue.conf so the soft-UE still attaches.
set -euo pipefail

COMPOSE="docker compose -f deploy/4g/docker-compose.yml"
IMSI="999700000000001"
KI="465B5CE8B199B49FAA5F0A2EE238A6BC"
OPC="E8ED289DEBA952E4283B54E88E6183CA"
TOKEN="dev-operator-token"

# Wait for the provisioner REST API to be ready.
echo "Waiting for provisioner to be ready..."
for i in $(seq 1 30); do
  if $COMPOSE exec -T provisioner wget -qO- http://127.0.0.1:8080/healthz >/dev/null 2>&1; then
    break
  fi
  sleep 1
done

# telcoctl runs inside the provisioner container; server is on localhost there.
$COMPOSE exec -T -e TELCOCTL_TOKEN="$TOKEN" -e TELCOCTL_SERVER="http://127.0.0.1:8080" \
  provisioner telcoctl add --imsi "$IMSI" --ki "$KI" --opc "$OPC" --apn internet \
  --reason NEW_ACTIVATION --note "make test-4g"

echo "Provisioned $IMSI via telcoctl"
