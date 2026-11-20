#!/usr/bin/env bash
# Acceptance: phones and towers switch off and on in a running network (hot-join).
#
# On a running multi stack (make 4g-multi), through the agent's power actions (what the map's
# buttons call), with the phones that are not under test pinging the whole time:
#   1. ue3 switch off: the MME gets its detach; switch on: attached again
#   2. ue1 restarted (crash style; shares tower A with ue2): attached again, ue2 unaffected
#   3. ue2 battery pulled: the MME keeps it registered, then marks it unreachable; switch on
#   4. enb-b shut down: S1 gone at once; power on: S1 back, ue3 on B again
#   5. enb-b power cut: S1 still there at first, gone after the SCTP heartbeats fail; power on
#   6. ue3 sends periodic TAUs and stays registered (assert-periodic-tau.sh)
# Takes ~30 minutes. Docs: docs/parts/5-hot-join.md.
set -euo pipefail
C="docker compose -f deploy/4g/docker-compose.yml -f deploy/4g/docker-compose.multi.yml"
AGENT="http://127.0.0.1:8091"; AGENT_AUTH="Authorization: Bearer ${AGENT_TOKEN:-dev-agent-token}"
TMP=$(mktemp -d); trap 'rm -rf "$TMP"' EXIT

pass() { echo "PASS $*"; }
fail() { echo "FAIL $*" >&2; exit 1; }
imsi_of() { awk -F'= *' '/^imsi/{print $2}' "deploy/4g/config/multi/$1.conf" | tr -d ' '; }
power() {  # power <service> <action>
  local out
  out=$(curl -sf -X POST -H "$AGENT_AUTH" -d "{\"action\": \"$2\"}" "$AGENT/power/$1") || fail "power $1 $2: agent refused"
  echo "  $1 $2: $(python3 -c 'import json,sys; print(json.load(sys.stdin)["command"])' <<<"$out")"
}
metrics() { $C exec -T provisioner wget -qO- "http://172.22.0.5:9090/$1" 2>/dev/null || echo '{}'; }
# ue_field <imsi> <python expression on the /ue-info item u>: prints it, or "gone" when the MME
# does not have the phone registered (after a detach it keeps the context as "deregistered")
ue_field() {
  metrics ue-info | python3 -c '
import json, sys
u = next((u for u in json.load(sys.stdin).get("items", []) if u.get("supi", "").endswith(sys.argv[1])), None)
print("gone" if u is None or u.get("mm_state") != "registered" else eval(sys.argv[2]))' "$1" "$2"
}
enb_listed() { metrics enb-info | python3 -c '
import json, sys
print(any(e.get("enb_id") == int(sys.argv[1]) for e in json.load(sys.stdin).get("items", [])))' "$1"; }
enb_id_of() { curl -sf -H "$AGENT_AUTH" "$AGENT/inventory" | python3 -c '
import json, sys
print(next(c["enb_id"] for c in json.load(sys.stdin)["cells"] if c["service"] == sys.argv[1]))' "$1"; }
until_true() {  # until_true <timeout-s> <description> <command...>
  local t=$1 what=$2; shift 2
  local end=$(( $(date +%s) + t ))
  until "$@"; do [ "$(date +%s)" -lt "$end" ] || fail "$what (after $t s)"; sleep 2; done
}
mme_log_since() { docker logs --since "$1" telecom-poc-4g-mme-1 2>&1; }
# Predicates for until_true.
detach_seen() { mme_log_since "$1" | grep -q "\[$2\] Detach request"; }
ue_gone() { [ "$(ue_field "$1" 1)" = gone ]; }
ue_on_enb() { [ "$(ue_field "$1" 'u.get("enb", {}).get("enb_id")')" = "$2" ]; }
enb_is() { [ "$(enb_listed "$1")" = "$2" ]; }
is_unreachable() { curl -sf -H "$AGENT_AUTH" "$AGENT/core/unreachable" | grep -q "$1"; }
# Pings from the phones not under test, in the background (40 each). The virtual radio itself
# loses about one ping in a thousand (docs/parts/3-handover.md §7), so one lost ping in a window
# is noise; two or more means the event disturbed a phone it should not have.
start_pings() { for ue in "$@"; do $C exec -T "$ue" ping -I tun_srsue -c 40 -W 15 10.45.0.1 >"$TMP/ping-$ue" 2>&1 & done; }
check_pings() {
  wait
  for f in "$TMP"/ping-*; do
    local ue sent got rtt
    ue=$(basename "$f" | cut -d- -f2-)
    sent=$(awk '/packets transmitted/{print $1}' "$f"); got=$(awk '/packets transmitted/{print $4}' "$f")
    rtt=$(awk -F'/' '/^rtt/{print $(NF-1)}' "$f")
    echo "  $ue pinged meanwhile: $got/$sent replies, slowest ${rtt:-?} ms"
    [ -n "$sent" ] && [ $(( sent - got )) -le 1 ] || fail "$ue lost pings meanwhile: $(grep transmitted "$f")"
    rm -f "$f"
  done
}
attached() { ./deploy/4g/scripts/wait-for-attach-svc.sh "$1" >/dev/null || fail "$1 did not attach again"; }

for ue in ue1 ue2 ue3; do attached "$ue"; done
ENB_B=$(enb_id_of enb-b)

echo "== 1. ue3: switch off, switch on"
start_pings ue1 ue2
t=$(date -u +%Y-%m-%dT%H:%M:%SZ)
power ue3 switch_off
I3=$(imsi_of ue3)
until_true 30 "no detach from ue3 at the MME" detach_seen "$t" "$I3"
until_true 30 "the MME still lists ue3 after its detach" ue_gone "$I3"
pass "switch off: ue3 sent a detach, the MME deregistered it"
power ue3 switch_on
attached ue3
check_pings
pass "switch on: ue3 attached again; ue1 and ue2 kept pinging"

echo "== 2. ue1: crash-style restart (shares tower A with ue2)"
start_pings ue2 ue3
$C restart ue1 >/dev/null 2>&1
attached ue1
check_pings
pass "ue1 restarted alone and attached again; ue2 and ue3 kept pinging"

echo "== 3. ue2: pull battery"
I2=$(imsi_of ue2)
start_pings ue1 ue3
power ue2 pull_battery
sleep 30
[ "$(ue_field "$I2" 1)" != gone ] || fail "the MME deregistered ue2 at once; a pulled battery sends nothing"
pass "battery pulled: 30 s later the MME still has ue2 ($(ue_field "$I2" 'u["cm_state"]'))"
# The tower first has to notice the radio link is gone (its timers run in radio time, slower than
# real time), then the MME's mobile reachable timer (T3412 + 4 min = 270 s) runs out: ~7 min.
until_true 600 "ue2 never became unreachable" is_unreachable "$I2"
pass "the core marks ue2 unreachable (mobile reachable timer)"
check_pings
power ue2 switch_on
attached ue2
pass "switch on: ue2 attached again"

echo "== 4. enb-b: shut down (maintenance), power on"
start_pings ue1 ue2
power enb-b shut_down
until_true 15 "the MME still has enb-b after its shutdown" enb_is "$ENB_B" False
pass "shut down: S1 gone at once"
power enb-b power_on
until_true 90 "enb-b never set up S1 again" enb_is "$ENB_B" True
pass "power on: S1 back"
until_true 180 "ue3 not back on enb-b" ue_on_enb "$I3" "$ENB_B"
check_pings
pass "ue3 is back on enb-b; ue1 and ue2 kept pinging"

echo "== 5. enb-b: power cut (outage), power on"
start_pings ue1 ue2
power enb-b power_cut
sleep 3
[ "$(enb_listed "$ENB_B")" = True ] || fail "the MME dropped enb-b at once; a power cut sends nothing"
pass "power cut: 3 s later the MME still has enb-b"
until_true 90 "the MME never noticed enb-b was gone" enb_is "$ENB_B" False
pass "the MME noticed through its SCTP heartbeats"
power enb-b power_on
until_true 90 "enb-b never set up S1 again" enb_is "$ENB_B" True
until_true 180 "ue3 not back on enb-b" ue_on_enb "$I3" "$ENB_B"
check_pings
pass "power on: S1 back, ue3 on enb-b; ue1 and ue2 kept pinging"

echo "== 6. periodic TAU"
./deploy/4g/scripts/assert-periodic-tau.sh ue3
echo "PASS: hot-join (phones and towers off and on in a running network)"
