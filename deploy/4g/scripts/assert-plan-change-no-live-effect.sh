#!/usr/bin/env bash
# Prove telcoctl set-plan reaches the MME live (a real S6a
# Insert-Subscriber-Data-Request, over the same HSS change-stream that powers
# suspend's live detach) but has NO live effect on an already-attached UE's
# bearer. Open5GS's mme_s6a_handle_idr (src/mme/mme-s6a-handler.c) updates
# mme_ue->ambr in memory but never calls s1ap_build_ue_context_modification_
# request -- the new AMBR is only ever sent to the eNB inside
# s1ap_build_initial_context_setup_request, which only runs on attach. So the
# UE keeps running at its OLD rate limit until it next attaches.
#
# Topology-agnostic: set COMPOSE_FILES/UE_SVC/IMSI to target the single-UE
# stack (the defaults) or any UE on the multi-UE stack, e.g.:
#   COMPOSE_FILES="-f deploy/4g/docker-compose.yml -f deploy/4g/docker-compose.multi.yml" \
#   UE_SVC=ue3 IMSI=999700000000003 ./deploy/4g/scripts/assert-plan-change-no-live-effect.sh
set -euo pipefail

COMPOSE_FILES="${COMPOSE_FILES:--f deploy/4g/docker-compose.yml}"
COMPOSE="docker compose $COMPOSE_FILES"
UE_SVC="${UE_SVC:-ue}"
IMSI="${IMSI:-999700000000001}"
TOKEN="dev-operator-token"
tctl() { $COMPOSE exec -T -e TELCOCTL_TOKEN="$TOKEN" -e TELCOCTL_SERVER="http://127.0.0.1:8080" provisioner telcoctl "$@"; }
has_ip() { $COMPOSE exec -T "$UE_SVC" ip -4 addr show tun_srsue 2>/dev/null | grep -q 'inet 10.45.'; }
attached() { has_ip && $COMPOSE exec -T "$UE_SVC" ping -c 1 -W 2 8.8.8.8 >/dev/null 2>&1; }

# The MME exposes its live in-memory UE state (including the AMBR it would
# use to build the next S1AP message) on its metrics server -- this is the
# same value mme_s6a_handle_idr just wrote to, so reading it here is reading
# real MME state, not the subscriber DB.
ambr_for() {
  $COMPOSE exec -T provisioner wget -qO- "http://172.22.0.5:9090/ue-info" \
    | grep -oP "\"supi\":\"$1\".*?\"ambr\":\{\"downlink\":\d+,\"uplink\":\d+\}" \
    | grep -oP '"ambr":\{"downlink":\d+,"uplink":\d+\}'
}

trap 'tctl set-plan "$IMSI" --dl 1G --ul 1G --reason OTHER --note "plan-change test cleanup" >/dev/null 2>&1 || true' EXIT

if ! attached; then
  echo "FAIL: precondition - $UE_SVC must already be attached before this test" >&2
  exit 1
fi

BEFORE="$(ambr_for "$IMSI")"
echo "MME's live AMBR for $IMSI before the change: $BEFORE"

tctl set-plan "$IMSI" --dl 5M --ul 2M --reason DOWNGRADE
sleep 2   # HSS change-stream poll interval

AFTER="$(ambr_for "$IMSI")"
echo "MME's live AMBR for $IMSI after the change:  $AFTER"
if [ "$AFTER" = "$BEFORE" ] || [ -z "$AFTER" ]; then
  echo "FAIL: MME's live UE state never picked up the plan change" >&2
  exit 1
fi
echo "PASS: plan change reached the MME live (a real Insert-Subscriber-Data-Request)."

# The bearer itself must be completely untouched: still attached, session
# still up, since nothing pushes the new AMBR toward the eNB/UE without a
# fresh attach.
if ! attached; then
  echo "FAIL: $UE_SVC lost its session as a side effect of the plan change (should be untouched)" >&2
  exit 1
fi
$COMPOSE exec -T "$UE_SVC" ping -c 2 8.8.8.8
echo "PASS: $UE_SVC's live session is untouched by the plan change -- signaled to the MME, not enforced, until its next attach."

# Force a fresh attach with pure NAS signaling: suspend, wait for the live
# CLR-based detach to actually land, then resume so the rejected
# auto-reattach falls back to T3402 (nas.cc's parse_attach_reject) into a
# genuine reattach -- no container ever restarts, so this works identically
# regardless of topology and never disturbs any other UE sharing a
# cell/broker (unlike recreating containers -- see docs/4G.md §7.1 and
# docs/parts/2-multi-ue-broker.md §6).
#
# suspend and resume must NOT be called back-to-back: the HSS's
# use_mongodb_change_stream watcher polls every 100ms, but that's not a
# guarantee -- calling resume before it has had a chance to observe the
# suspend's request_cancel_location=true update risks it only ever seeing
# the net final (already-cleared) document and never firing the CLR at all.
# Confirmed live: without this wait, the detach silently didn't happen on
# some runs. Waiting for a real ping failure (not just a fixed sleep) proves
# the detach actually landed before moving on.
echo "Forcing a fresh attach via suspend+resume to prove the new plan applies there..."
tctl suspend "$IMSI" --reason MAINTENANCE --note "plan-change test: force reattach"
detached=false
for i in $(seq 1 15); do
  if ! $COMPOSE exec -T "$UE_SVC" ping -c 1 -W 1 8.8.8.8 >/dev/null 2>&1; then
    detached=true
    break
  fi
  sleep 1
done
if [ "$detached" = false ]; then
  echo "FAIL: $UE_SVC was never actually detached by suspend" >&2
  exit 1
fi
tctl resume "$IMSI" --reason CLEARED --note "plan-change test: force reattach"
for i in $(seq 1 60); do
  attached && break
  sleep 1
done
if ! attached; then
  echo "FAIL: $UE_SVC did not reattach within 60s" >&2
  exit 1
fi

REATTACHED="$(ambr_for "$IMSI")"
echo "MME's live AMBR for $IMSI after reattach:     $REATTACHED"
if [ "$REATTACHED" != "$AFTER" ]; then
  echo "FAIL: fresh attach did not pick up the new plan" >&2
  exit 1
fi
$COMPOSE exec -T "$UE_SVC" ping -c 2 8.8.8.8
echo "PASS: the new plan is exactly what the fresh attach initialized $UE_SVC's live session with."
