# telecom-poc

A proof-of-concept, fully-virtual 4G/5G mobile network built from open-source components (Open5GS + srsRAN_4G + UERANSIM), for learning telecom internals. No RF hardware, no over-the-air transmission. See `docs/` for `THEORY.md` background, `4G.md` walkthrough, and more.


## Quickstart (4G)

```bash
make submodules      # fetch Open5GS / srsRAN_4G / UERANSIM / PacketRusher

# Completely automatic flow
make 4g-auto         # MULTI: 2 eNBs + 3 UEs, provision, attach, ping all 3 (= 4g-multi + ping)
make 4g-single       # SINGLE: 1 eNB + 1 UE + 1 subscriber, attach + ping (the pre-multi behaviour)
make status-4g       # check health
make 4g-auto-down    # tear down (or 4g-single-down for the single-cell stack)


# Or bring it up in stages (mirrors the manual steps in docs/4G.md §5.3):
make 4g-infra             # operator network only: EPC core + both eNBs (no subscribers, no UEs)
make 4g-demo-subscribers  # provision the 3 fixed subscribers via the telcoctl client
make 4g-device            # then ONE device: ue1 → sub …001 → enb-a, attach + ping
make 4g-demo-devices      # or ALL THREE: ue1+ue2+ue3 + brokers, attach all

make 4g-device-down       # remove ONLY device ue1 + broker-a (infra keeps running)
make 4g-auto-down         # tear the whole stack down (infra is the foundation, so the UEs go too)
```

## Layout

- `deploy/` - Docker Compose stacks and configs
- `tools/` - custom Go tools (provisioner, loadgen)
- `vendor/` - upstream projects as git submodules
- `docs/` - theory, walkthroughs, and more
