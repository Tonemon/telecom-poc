#!/usr/bin/env bash
# Register a few demo subscribers via the Go provisioner (telcoctl -> REST -> Mongo),
# to exercise the client after 'make 4g-infra'. Subscriber #1 uses the SAME key material
# as deploy/4g/config/ue.conf, so the soft-UE ('make 4g-device') can attach. #2 and #3
# are batch-issued with server-generated keys and never attach (DB/demo entries only).
#
# Idempotent: safe to re-run. Already-present subscribers are left untouched.
set -euo pipefail

COMPOSE="docker compose -f deploy/4g/docker-compose.yml"
TOKEN="dev-operator-token"

# The subscriber the soft-UE actually uses (keys must match ue.conf).
UE_IMSI="999700000000001"
UE_KI="465B5CE8B199B49FAA5F0A2EE238A6BC"
UE_OPC="E8ED289DEBA952E4283B54E88E6183CA"
# How many demo subscribers we want in total (UE SIM + batch-issued extras).
TARGET=3

# quiet telcoctl inside the provisioner container.
telctl() {
  $COMPOSE exec -T -e TELCOCTL_TOKEN="$TOKEN" -e TELCOCTL_SERVER="http://127.0.0.1:8080" \
    provisioner telcoctl "$@"
}
# visible telcoctl: echo the call, then run it.
run() { echo "+ telcoctl $*"; telctl "$@"; }
# is this IMSI already provisioned? (telcoctl get exits non-zero when not found)
exists() { telctl get "$1" >/dev/null 2>&1; }

echo "Waiting for provisioner to be ready..."
for i in $(seq 1 30); do
  if $COMPOSE exec -T provisioner wget -qO- http://127.0.0.1:8080/healthz >/dev/null 2>&1; then
    break
  fi
  sleep 1
done

echo
echo "== Subscriber 1/3: the soft-UE's own SIM (matches ue.conf, this one can attach) =="
if exists "$UE_IMSI"; then
  echo "  $UE_IMSI already present — skipping."
else
  run add --imsi "$UE_IMSI" --ki "$UE_KI" --opc "$UE_OPC" --apn internet \
    --reason NEW_ACTIVATION --note "demo: soft-UE SIM"
fi

echo
echo "== Batch-issued demo SIMs (server-generated Ki/OPc), up to $TARGET total =="
# Count current subscribers (one "imsi" per record) and issue only the shortfall,
# so re-runs — even after a partial run — converge on exactly $TARGET.
have=$(telctl list | grep -c '"imsi"' || true)
need=$(( TARGET - have ))
if [ "$need" -gt 0 ]; then
  run issue-sim --count "$need" --apn internet --reason BATCH --note "demo batch"
else
  echo "  already have $have subscriber(s) — nothing to issue."
fi

echo
echo "Demo subscribers registered. Current roster:"
run list
