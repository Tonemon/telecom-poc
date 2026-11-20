#!/usr/bin/env bash
# Assert that an idle UE sends periodic tracking area updates and stays registered.
#
# A real idle phone reports in every T3412 (30 s here, deploy/4g/config/mme.yaml) with a periodic
# TAU; that is what keeps the MME's mobile-reachable timer (T3412 + 4 min) and implicit-detach
# timer (another T3412 + 4 min) from detaching it. Stock srsue ignores T3412; the patch
# deploy/patches/srsran-nas-periodic-tau.patch adds the periodic TAU.
#
# Usage: assert-periodic-tau.sh <ue-service>   (on a running multi stack; takes ~11 minutes)
set -euo pipefail
COMPOSE="docker compose -f deploy/4g/docker-compose.yml -f deploy/4g/docker-compose.multi.yml"
UE="$1"
IMSI=$(awk -F'= *' '/^imsi/{print $2}' "deploy/4g/config/multi/$UE.conf" | tr -d ' ')
STAY_S="${STAY_S:-600}"  # past the ~9-minute implicit-detach mark for T3412 = 30 s

# cm_state of the UE as the MME sees it: connected, idle, or "gone" (not listed).
cm_state() {
  $COMPOSE exec -T provisioner wget -qO- http://172.22.0.5:9090/ue-info 2>/dev/null | python3 -c '
import json, sys
imsi = sys.argv[1]
items = json.load(sys.stdin).get("items", [])
print(next((u.get("cm_state", "?") for u in items if u.get("supi", "").endswith(imsi)), "gone"))' "$IMSI"
}

echo "Waiting for $UE ($IMSI) to go idle..."
for _ in $(seq 1 120); do
  [ "$(cm_state)" = idle ] && break
  sleep 2
done
[ "$(cm_state)" = idle ] || { echo "FAIL: $UE never went idle" >&2; exit 1; }
idle_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)
echo "$UE idle since $idle_at; expecting a periodic TAU within 90 s..."

tau_seen=""
for _ in $(seq 1 45); do
  if docker logs --since "$idle_at" telecom-poc-4g-mme-1 2>&1 | grep -q "\[$IMSI\] Tracking area update request"; then
    tau_seen=yes; break
  fi
  sleep 2
done
[ -n "$tau_seen" ] || { echo "FAIL: no Tracking area update request from $UE within 90 s of going idle" >&2; exit 1; }
echo "  periodic TAU seen"

echo "Checking $UE is still registered after $STAY_S s..."
sleep "$STAY_S"
state=$(cm_state)
[ "$state" != gone ] || { echo "FAIL: the MME no longer lists $UE (implicitly detached)" >&2; exit 1; }
count=$(docker logs --since "$idle_at" telecom-poc-4g-mme-1 2>&1 | grep -c "\[$IMSI\] Tracking area update request" || true)
if docker logs --since "$idle_at" telecom-poc-4g-mme-1 2>&1 | grep -q "\[$IMSI\] \(Mobile Reachable\|Implicit Detach\)"; then
  echo "FAIL: the MME's reachability timers ran out for $UE" >&2; exit 1
fi
echo "PASS: $UE sends periodic TAUs ($count in $STAY_S s) and stays registered"
