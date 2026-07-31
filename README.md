# telecom-poc

A proof-of-concept, fully-virtual 4G/5G mobile network built from open-source components (Open5GS + srsRAN_4G + UERANSIM), for learning telecom internals. No RF hardware, no over-the-air transmission. See `docs/` for `THEORY.md` background, `4G.md` walkthrough, and more.


## Quickstart (4G)

```bash
make submodules      # fetch Open5GS / srsRAN_4G / UERANSIM / PacketRusher

# Completely automatic flow
make 4g-auto         # full deploy: build + start, provision, attach, ping the internet
make status-4g       # check health
make 4g-auto-down    # tear down


# Or bring it up in stages (mirrors the manual steps in docs/4G.md §5.3):
make 4g-infra             # operator network only: core + eNB (no subscribers, no UE)
make 4g-demo-subscribers  # register 3 demo subscribers via the telcoctl client
make 4g-device            # attach the UE and verify the data plane

make 4g-device-down       # remove ONLY the UE (infra keeps running)
make 4g-infra-down        # tear the whole stack down (infra is the foundation, so the UE goes too)
```

## Layout

- `deploy/` - Docker Compose stacks and configs
- `tools/` - custom Go tools (provisioner, loadgen)
- `vendor/` - upstream projects as git submodules
- `docs/` - theory, walkthroughs, and more
