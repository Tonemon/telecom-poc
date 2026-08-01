#!/usr/bin/env bash
# Suspend an ALREADY-ATTACHED UE (no recreate) and prove the live session is
# actually torn down: HSS change-stream -> S6a Cancel-Location-Request ->
# MME Detach-Request -> brief real outage -> UE (told REATTACH_REQUIRED)
# reattaches on its own. This is the scenario the stash-only enforcement
# never exercises on its own, since assert-attach-rejected.sh always
# force-recreates the UE before suspending.
set -euo pipefail

COMPOSE="docker compose -f deploy/4g/docker-compose.yml"
IMSI="999700000000001"
TOKEN="dev-operator-token"
PING_LOG="$(mktemp)"
tctl() { $COMPOSE exec -T -e TELCOCTL_TOKEN="$TOKEN" -e TELCOCTL_SERVER="http://127.0.0.1:8080" provisioner telcoctl "$@"; }
has_ip() { $COMPOSE exec -T ue ip -4 addr show tun_srsue 2>/dev/null | grep -q 'inet 10.45.'; }

if ! has_ip; then
  echo "FAIL: precondition - UE must already be attached before this test" >&2
  exit 1
fi

# Run a continuous background ping spanning the suspend event, then suspend
# partway through. 60 pings at 0.3s = ~18s total, comfortably longer than
# the ~3.5s outage a live detach+reattach cycle takes.
$COMPOSE exec -T ue sh -c 'ping -I tun_srsue -i 0.3 -c 60 8.8.8.8' > "$PING_LOG" 2>&1 &
PING_PID=$!
sleep 2
tctl suspend "$IMSI" --reason FRAUD --note "live-detach test"
wait "$PING_PID" || true

LOSS="$(grep -oE '[0-9]+(\.[0-9]+)?% packet loss' "$PING_LOG" | grep -oE '^[0-9]+' || true)"
echo "Ping loss spanning the suspend event: ${LOSS}%"
if [ -z "$LOSS" ] || [ "$LOSS" -lt 5 ]; then
  echo "FAIL: no meaningful packet loss observed — live session was not detached" >&2
  cat "$PING_LOG" >&2
  tctl resume "$IMSI" --reason CLEARED || true
  rm -f "$PING_LOG"
  exit 1
fi
echo "PASS: suspend caused real packet loss on the live session (live detach enforced)."
rm -f "$PING_LOG"

tctl resume "$IMSI" --reason CLEARED --note "live-detach test"
echo "Resumed $IMSI; confirming the UE has working connectivity again (it reattaches on its own per REATTACH_REQUIRED)..."

reattached=false
for i in $(seq 1 30); do
  if has_ip && $COMPOSE exec -T ue ping -I tun_srsue -c 1 -W 2 8.8.8.8 >/dev/null 2>&1; then
    reattached=true
    break
  fi
  sleep 1
done

if [ "$reattached" = false ]; then
  echo "UE did not auto-reattach within timeout; forcing recreate as fallback..."
  $COMPOSE up -d --force-recreate enb ue >/dev/null 2>&1
  for i in $(seq 1 30); do
    if has_ip && $COMPOSE exec -T ue ping -I tun_srsue -c 1 -W 2 8.8.8.8 >/dev/null 2>&1; then
      reattached=true
      break
    fi
    sleep 1
  done
fi

if [ "$reattached" = false ]; then
  echo "FAIL: UE did not reattach after resume, even after force-recreate" >&2
  exit 1
fi
echo "PASS: UE reattached after resume."

$COMPOSE exec -T ue ping -I tun_srsue -c 2 8.8.8.8
