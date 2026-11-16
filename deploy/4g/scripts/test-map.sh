#!/usr/bin/env bash
# Acceptance: the world map and everything it drives, through the map server's API only (the
# same calls the page makes): health, inventory, tower moves, packet captures, speedtest, the
# operator's suspend/resume, the allow-list, and random/realtime activity.
set -euo pipefail
C="docker compose -f deploy/4g/docker-compose.yml -f deploy/4g/docker-compose.multi.yml"
M="http://127.0.0.1:8088/api/instances/telecom-poc"
IMSI_UE3=999700000000003

fail() { echo "FAIL: $*" >&2; exit 1; }
py() { python3 -c "import json,sys; d=json.load(sys.stdin); $1"; }
code() { curl -s -o /dev/null -w '%{http_code}' -H 'X-Map: 1' "$@"; }
curl() { command curl -H 'X-Map: 1' "$@"; }  # the map server only takes changes that carry X-Map

make 4g-multi
for u in ue1 ue2 ue3; do ./deploy/4g/scripts/wait-for-attach-svc.sh "$u"; done
make map
for _ in $(seq 1 30); do curl -sf "$M/air/world" >/dev/null && break; sleep 1; done

# 1. Every tower, device and API container runs.
curl -s "$M/agent/components" | py '
want = {"enb-a", "enb-b", "ue1", "ue2", "ue3", "air", "agent", "provisioner", "mme"}
bad = [c["service"] for c in d if c["service"] in want and c["state"] != "running"]
missing = want - {c["service"] for c in d}
sys.exit(f"not running: {bad}, missing: {sorted(missing)}" if bad or missing else 0)' \
  || fail "components"
echo "PASS: all components running"

# 2. Inventory from the configs.
curl -s "$M/agent/inventory" | py '
a = d["cells"][0]
ok = a["band"] == 7 and a["dl_mhz"] == 2680 and a["eci"] == 0x19B01 and len(d["devices"]) == 3
sys.exit(0 if ok else f"unexpected inventory: {a}")' || fail "inventory"
echo "PASS: inventory (cell A band 7, 2680 MHz; 3 devices)"

# 3. Tower move: the dry-run says ue3 would lose coverage and moves nothing; a real move and back.
curl -s -X PUT -d '{"x": 30, "y": 0}' "$M/air/cells/B/position?dry_run=1" \
  | py 'sys.exit(0 if {u["id"]: u["effect"] for u in d["ues"]}["ue3"] == "loses" else d)' || fail "dry-run"
curl -s "$M/air/world" | py 'sys.exit(0 if [c for c in d["cells"] if c["id"] == "B"][0]["x"] == 10 else "B moved")' \
  || fail "dry-run moved the cell"
for x in 11 10; do
  [ "$(code -X PUT -d "{\"x\": $x, \"y\": 0}" "$M/air/cells/B/position")" = 200 ] || fail "move cell B to $x"
done
echo "PASS: tower move (dry-run reports ue3 losing coverage; move and back)"

# 4. Packet captures on a tower and a device.
for c in enb-a ue2; do
  [ "$(code -X POST -d "{\"component\": \"$c\"}" "$M/agent/pcaps")" = 200 ] || fail "start capture on $c"
done
$C exec -T ue2 ping -I tun_srsue -c 3 -W 5 8.8.8.8 >/dev/null || true
for c in enb-a ue2; do
  f=$(curl -s -X DELETE "$M/agent/pcaps/$c" | py 'print(d["file"])') || fail "stop capture on $c"
  n=$(curl -s "$M/agent/pcaps/$f" | wc -c)
  [ "$n" -gt 24 ] || fail "$f has $n bytes"
  echo "PASS: capture on $c ($f, $n bytes)"
done

# 5. Speedtest (to the UPF).
curl -s -X POST "$M/agent/ues/ue1/speedtest" \
  | py 'print("%.2f down, %.2f up (Mbit/s)" % (d["down_mbps"], d["up_mbps"])); sys.exit(0 if d["down_mbps"] > 0 else 1)' \
  || fail "speedtest"
echo "PASS: speedtest on ue1"

# 6. The operator suspends ue3 (detached: no service) and resumes it (back by itself).
hdr=$(curl -s -D - -o /dev/null -X POST -d '{"reason": "MAINTENANCE"}' "$M/provisioner/subscribers/$IMSI_UE3/suspend" \
  | tr -d '\r' | awk -F': ' 'tolower($1) == "x-command" {print $2}')
[[ "$hdr" == "./bin/telcoctl suspend $IMSI_UE3"* ]] || fail "X-Command header: '$hdr'"
t0=$(date +%s)
while $C exec -T ue3 ping -I tun_srsue -c 1 -W 2 8.8.8.8 >/dev/null 2>&1; do
  [ $(( $(date +%s) - t0 )) -gt 60 ] && fail "ue3 still online 60 s after suspend"
  sleep 2
done
[ "$(code -X POST -d '{"reason": "CLEARED"}' "$M/provisioner/subscribers/$IMSI_UE3/resume")" = 200 ] || fail "resume"
t0=$(date +%s)
until $C exec -T ue3 ping -I tun_srsue -c 1 -W 2 8.8.8.8 >/dev/null 2>&1; do
  [ $(( $(date +%s) - t0 )) -gt 240 ] && fail "ue3 not back 240 s after resume"
  sleep 3
done
echo "PASS: suspend detached ue3, resume brought it back ($(( $(date +%s) - t0 )) s); X-Command: $hdr"

# 7. The proxy only passes its allow-list.
[ "$(code -X PUT -d '{"A": 1}' "$M/air/ues/ue1/gains")" = 403 ] || fail "gains route not blocked"
[ "$(code -X DELETE "$M/provisioner/subscribers/$IMSI_UE3")" = 403 ] || fail "delete subscriber not blocked"
echo "PASS: routes outside the allow-list get 403"

# 8. Random activity, then realtime on for a minute and off: every automated UE stops.
curl -s -X POST "$M/air/activity/random" | py 'sys.exit(0 if len(d["plans"]) == 3 else d)' || fail "random activity"
[ "$(code -X PUT -d '{"on": true}' "$M/air/activity/realtime")" = 200 ] || fail "realtime on"
sleep 60
curl -s -X PUT -d '{"on": false}' "$M/air/activity/realtime" >/dev/null
curl -s "$M/air/world" | py '
moving = [u["id"] for u in d["ues"] if u["travel"]]
sys.exit(f"still travelling: {moving}" if moving or d["activity"]["realtime"] else 0)' || fail "realtime off"
echo "PASS: random activity; realtime off stops every automated UE where it is"
echo "PASS: the world map drives the instance end to end"
