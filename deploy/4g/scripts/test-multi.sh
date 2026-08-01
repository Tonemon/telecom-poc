#!/usr/bin/env bash
# Bring up the 2-cell/3-UE topology and assert concurrent multi-UE + multi-cell works.
set -euo pipefail
C="docker compose -f deploy/4g/docker-compose.yml -f deploy/4g/docker-compose.multi.yml"

$C up -d --build mongo provisioner nrf scp hss pcrf sgwc sgwu upf smf mme
./deploy/4g/scripts/provision-multi.sh
$C up -d enb-a enb-b ue1 ue2 ue3
sleep 5
$C up -d --no-deps broker-a broker-b   # brokers start LAST

for u in ue1 ue2 ue3; do ./deploy/4g/scripts/wait-for-attach-svc.sh "$u"; done

# Concurrent pings so all three UEs generate data-plane traffic at once.
for u in ue1 ue2 ue3; do
  $C exec -T "$u" ping -I tun_srsue -c 3 8.8.8.8 >"/tmp/test-multi-$u.log" 2>&1 &
done
wait
for u in ue1 ue2 ue3; do
  grep -q "0% packet loss" "/tmp/test-multi-$u.log" || { echo "$u lost packets" >&2; exit 1; }
  echo "$u: 0% packet loss"
done

./deploy/4g/scripts/assert-two-on-one-enb.sh
echo "PASS: 3 UEs attached across 2 cells; 2 concurrent on enb-a."
