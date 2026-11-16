#!/usr/bin/env bash
# Acceptance: ue2 travels from cell A to cell B and back with a ping running; both handovers must
# be real S1 handovers (same IP, few pings lost); ue1 and ue3 must not notice. Then ue3 goes into a
# Faraday cage (no service) and must come back by itself.
set -euo pipefail
C="docker compose -f deploy/4g/docker-compose.yml -f deploy/4g/docker-compose.multi.yml"
TRAVEL="python3 ./deploy/4g/scripts/travel.py"
AIR="http://127.0.0.1:8090"; AIR_AUTH="Authorization: Bearer ${AIR_TOKEN:-dev-air-token}"
PROV="http://127.0.0.1:8080"; PROV_AUTH="Authorization: Bearer ${TELCOCTL_TOKEN:-dev-operator-token}"
TRIP_TIMEOUT="${TRIP_TIMEOUT:-420}"   # a 6 km trip at 1000 km/h takes ~3.5 min of real time
IMSI_UE2=999700000000002

fail() { echo "FAIL: $*" >&2; exit 1; }
ip_of() { $C exec -T "$1" ip -4 addr show tun_srsue 2>/dev/null | awk '/inet /{print $2}'; }
enb_of() {  # eNB id the MME has ue2 on, or empty
  curl -s -H "$PROV_AUTH" "$PROV/subscribers/$1" | python3 -c \
    'import json,sys; n=(json.load(sys.stdin).get("network") or {}); print(n.get("enb_id", ""))'
}
travelling() {
  curl -s -H "$AIR_AUTH" "$AIR/world" | python3 -c \
    "import json,sys; print(any(u['travel'] for u in json.load(sys.stdin)['ues'] if u['id']=='$1'))"
}
hos() { $C logs --no-color ue2 2>/dev/null | grep -c 'HO to PCI=.* successful' || true; }
loss() { $C exec -T "$1" sh -c 'cat /tmp/ho-ping.log' | grep -oE '[0-9.]+% packet loss' | cut -d% -f1; }

trip() {  # trip <x> <expected eNB id> <cell name>
  local before t0; before=$(hos); t0=$(date +%s)
  $TRAVEL UE=ue2 TO="$1" >/dev/null
  while :; do
    if [ "$(travelling ue2)" = False ] && [ "$(hos)" -gt "$before" ] && [ "$(enb_of $IMSI_UE2)" = "$2" ]; then
      echo "PASS: ue2 handed over to $3 ($(( $(date +%s) - t0 )) s)"; return
    fi
    [ $(( $(date +%s) - t0 )) -gt "$TRIP_TIMEOUT" ] && \
      fail "ue2 not handed over to $3 within ${TRIP_TIMEOUT} s (MME has it on eNB '$(enb_of $IMSI_UE2)')"
    sleep 5
  done
}

$C up -d --build mongo provisioner nrf scp hss pcrf sgwc sgwu upf smf mme
./deploy/4g/scripts/provision-multi.sh
$C build air
$C up -d enb-a enb-b ue1 ue2 ue3
./deploy/4g/scripts/wait-radio-ready.sh enb-a enb-b ue1 ue2 ue3
$C up -d --no-deps air
for u in ue1 ue2 ue3; do ./deploy/4g/scripts/wait-for-attach-svc.sh "$u"; done

ip_start=$(ip_of ue2)
for u in ue1 ue2 ue3; do
  $C exec -d "$u" sh -c 'ping -I tun_srsue -i 0.2 8.8.8.8 > /tmp/ho-ping.log 2>&1'
done
sleep 5

trip 8 412 "cell B"   # enb_id 0x19C
trip 2 411 "cell A"   # enb_id 0x19B

for u in ue1 ue2 ue3; do $C exec -T "$u" pkill -INT ping || true; done
sleep 2
[ "$(ip_of ue2)" = "$ip_start" ] || fail "ue2's IP changed: $ip_start -> $(ip_of ue2) (a reattach, not a handover)"
echo "PASS: ue2 kept its IP ($ip_start)"
for u in ue1 ue2 ue3; do
  l=$(loss "$u"); max=1; [ "$u" = ue2 ] && max=5
  python3 -c "import sys; sys.exit(0 if float('$l') <= $max else 1)" \
    || { $C exec -T "$u" tail -3 /tmp/ho-ping.log >&2; fail "$u lost $l% of its pings (max $max%)"; }
  echo "PASS: $u lost $l% of its pings during both handovers (max $max%)"
done

$TRAVEL UE=ue3 SHIELD=on >/dev/null
sleep 30
if $C exec -T ue3 ping -I tun_srsue -c 2 -W 2 8.8.8.8 >/dev/null 2>&1; then
  fail "ue3 still reaches the internet inside the Faraday cage"
fi
echo "PASS: ue3 has no service while shielded"
$TRAVEL UE=ue3 SHIELD=off >/dev/null
t0=$(date +%s)
until $C exec -T ue3 ping -I tun_srsue -c 1 -W 2 8.8.8.8 >/dev/null 2>&1; do
  [ $(( $(date +%s) - t0 )) -gt 60 ] && fail "ue3 did not get service back within 60 s after leaving the cage"
  sleep 2
done
echo "PASS: ue3 back in service $(( $(date +%s) - t0 )) s after leaving the Faraday cage"
echo "PASS: real S1 handover A -> B -> A with the ping surviving; no-coverage case recovers"
