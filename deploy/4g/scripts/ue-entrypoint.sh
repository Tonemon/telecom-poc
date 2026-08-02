#!/bin/sh
# Runs srsue, then locks eth0 out of the internet and keeps a default route
# via tun_srsue in sync with the PDU session's lifecycle.
#
# This container's eth0 exists only to carry the simulated ZMQ RF link to
# the eNB/broker (a real phone has no such side-channel), but it sits on a
# normal Docker bridge with real internet access. Left alone, that gives the
# UE "free" internet unrelated to its cellular attach state, so suspending a
# subscriber (which tears down the S1-U/GTP bearer) would have no visible
# effect. Two rules fix that: allow eth0 only to the epc subnet (the RF
# link), drop everything else on it. tun_srsue gets no rules -- it's the
# only interface that can ever reach the internet, and only while attached.
#
# srsue itself never installs a default route via tun_srsue (only an on-link
# route for its assigned /24), so ordinary (non interface-bound) traffic has
# no way to reach the tunnel at all without this loop maintaining one.
#
# The loop re-asserts the route every second unconditionally (rather than
# only reacting to an observed no-IP -> has-IP edge) because `ip route
# replace`/`ip route del ... || true` are idempotent and cheap, and an
# edge-triggered version can miss a transition -- confirmed live: after the
# nas.cc T3402 patch, a suspend/resume cycle can complete a full detach +
# automatic reattach (new tun_srsue address) without this container ever
# restarting, and an edge-triggered check running at 1s granularity missed
# that the address had changed, leaving the route stale even though the
# tunnel itself was working fine.
#
# Usage: ue-entrypoint.sh <ue.conf>
set -eu

EPC_SUBNET="172.22.0.0/24"
TUN_DEV="tun_srsue"

iptables -A OUTPUT -o eth0 -d "$EPC_SUBNET" -j ACCEPT
iptables -A OUTPUT -o eth0 -j DROP

srsue "$@" &
SRSUE_PID=$!

while kill -0 "$SRSUE_PID" 2>/dev/null; do
  if ip -4 addr show "$TUN_DEV" 2>/dev/null | grep -q "inet "; then
    ip route replace default dev "$TUN_DEV"
  else
    ip route del default dev "$TUN_DEV" 2>/dev/null || true
  fi
  sleep 1
done

wait "$SRSUE_PID"
