#!/usr/bin/env bash
# Suspend the subscriber, recreate the UE (pristine NAS state), and assert it
# does NOT get an IP within the timeout (attach rejected by HSS via
# subscriber_status=barred). Then resume and recreate the UE so a following
# wait-for-attach can succeed. We force-recreate (not restart) so the UE never
# reuses a stored .ctxt/GUTI from a prior rejected attempt.
set -euo pipefail

COMPOSE="docker compose -f deploy/4g/docker-compose.yml"
IMSI="999700000000001"
TOKEN="dev-operator-token"
tctl() { $COMPOSE exec -T -e TELCOCTL_TOKEN="$TOKEN" -e TELCOCTL_SERVER="http://127.0.0.1:8080" provisioner telcoctl "$@"; }

tctl suspend "$IMSI" --reason NON_PAYMENT --note "lifecycle test"

$COMPOSE up -d --force-recreate enb ue >/dev/null 2>&1
echo "UE recreated while suspended; watching for attach (should FAIL)..."

for i in $(seq 1 30); do
  if $COMPOSE exec -T ue ip -4 addr show tun_srsue 2>/dev/null | grep -q 'inet 10.45.'; then
    echo "FAIL: UE got an IP while suspended — barring not enforced"
    tctl resume "$IMSI" --reason CLEARED || true
    exit 1
  fi
  sleep 1
done
echo "PASS: suspended UE did not attach."

tctl resume "$IMSI" --reason PAYMENT_RECEIVED --note "lifecycle test"
$COMPOSE up -d --force-recreate enb ue >/dev/null 2>&1
echo "Resumed $IMSI and recreated UE."
