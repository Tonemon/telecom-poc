#!/usr/bin/env bash
# Insert the test subscriber via the vendored open5gs-dbctl helper.
# dbctl uses `mongosh`, which lives in the mongo container, so we copy the
# script in and run it there against the local DB. Idempotent: removes any
# existing entry for this IMSI first.
# Replaced by the Go `provisioner` in Plan 2 — kept as the documented baseline.
set -euo pipefail

IMSI="999700000000001"
KI="465B5CE8B199B49FAA5F0A2EE238A6BC"
OPC="E8ED289DEBA952E4283B54E88E6183CA"
APN="internet"

COMPOSE="docker compose -f deploy/4g/docker-compose.yml"
DBCTL="vendor/open5gs/misc/db/open5gs-dbctl"
DB_URI="mongodb://localhost/open5gs"

$COMPOSE cp "$DBCTL" mongo:/tmp/open5gs-dbctl
$COMPOSE exec -T mongo chmod +x /tmp/open5gs-dbctl

$COMPOSE exec -T mongo /tmp/open5gs-dbctl --db_uri="$DB_URI" remove "$IMSI" >/dev/null 2>&1 || true
$COMPOSE exec -T mongo /tmp/open5gs-dbctl --db_uri="$DB_URI" add_ue_with_apn "$IMSI" "$KI" "$OPC" "$APN"

echo "Provisioned subscriber $IMSI (APN=$APN)"
