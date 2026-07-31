#!/usr/bin/env bash
# Assert enb-a has >= 2 distinct UE RNTIs in its logs (proves multi-UE on one cell).
set -euo pipefail
COMPOSE="docker compose -f deploy/4g/docker-compose.yml -f deploy/4g/docker-compose.multi.yml"
rntis=$($COMPOSE logs enb-a 2>/dev/null | grep -oiE "rnti=0x[0-9a-f]+" | sort -u | wc -l)
echo "enb-a distinct RNTIs: $rntis"
[ "$rntis" -ge 2 ] || { echo "expected >=2 UEs on enb-a" >&2; exit 1; }
echo "PASS: two UEs on one eNB"
