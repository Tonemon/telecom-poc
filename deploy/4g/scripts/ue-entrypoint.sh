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
# Note: srsue also never arms a retry timer for several attach-reject causes
# (notably #8, "EPS services and non-EPS services not allowed" -- what an
# auto-reattach gets while still suspended; see parse_attach_reject's "TODO:
# handle other relevant reject causes" in nas.cc), so a UE rejected in that
# state stays dark until something recreates it. That's not fixable from
# here: srsRAN's ZMQ RF driver only reconnects if the eNB (and, in multi-UE
# mode, the broker) restart together with the UE, which a UE-only script has
# no reach to do. Left as documented, existing behavior (see
# assert-live-detach.sh's force-recreate fallback) rather than worked around
# here.
#
# Usage: ue-entrypoint.sh <ue.conf>
set -eu

EPC_SUBNET="172.22.0.0/24"
TUN_DEV="tun_srsue"

iptables -A OUTPUT -o eth0 -d "$EPC_SUBNET" -j ACCEPT
iptables -A OUTPUT -o eth0 -j DROP

srsue "$@" &
SRSUE_PID=$!

has_route=0
while kill -0 "$SRSUE_PID" 2>/dev/null; do
  if ip -4 addr show "$TUN_DEV" 2>/dev/null | grep -q "inet "; then
    if [ "$has_route" -eq 0 ]; then
      ip route replace default dev "$TUN_DEV"
      has_route=1
    fi
  else
    if [ "$has_route" -eq 1 ]; then
      ip route del default dev "$TUN_DEV" 2>/dev/null || true
      has_route=0
    fi
  fi
  sleep 1
done

wait "$SRSUE_PID"
