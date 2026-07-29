#!/bin/sh
# Rewrites the in-image freeDiameter config for container networking, then
# execs the Open5GS network-function daemon.
#
# The stock freeDiameter configs bind ListenOn to 127.0.0.x and ConnectTo a
# 127.0.0.x peer (loopback assumptions). On our static-IP compose network we
# rewrite both to the real addresses. TLS is already disabled (No_TLS) in the
# stock peer definitions, so no certificates are involved.
#
# Usage: diameter-entrypoint.sh <fd_conf> <listen_ip> <peer_ip> -- <daemon> [args...]
set -e
FD_CONF="$1"; LISTEN_IP="$2"; PEER_IP="$3"; shift 3
[ "$1" = "--" ] && shift

sed -i "s#^ListenOn = .*#ListenOn = \"${LISTEN_IP}\";#" "$FD_CONF"
sed -i "s#ConnectTo = \"127\.0\.0\.[0-9]\+\"#ConnectTo = \"${PEER_IP}\"#" "$FD_CONF"

echo "[diameter-entrypoint] $FD_CONF -> ListenOn=${LISTEN_IP} ConnectTo=${PEER_IP}"
exec "$@"
