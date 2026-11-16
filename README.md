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
make 4g-devices           # then the 3 devices: ue1+ue2+ue3 + the air, attach + ping all

make 4g-devices-down      # remove the 3 devices + the air (core, eNBs and subscribers keep running)
make 4g-auto-down         # tear the whole stack down (infra is the foundation, so the UEs go too)

# Move the devices around (on a running multi-cell stack):
make 4g-travel                  # help + where everyone is
make 4g-travel UE=ue2 TO=8      # ue2 drives to cell B: a real handover, ping keeps running
make 4g-travel UE=ue3 SHIELD=on # Faraday cage: no coverage (SHIELD=off to come back)

# Acceptance tests (each starts its own stack):
make test-4g-multi        # 3 UEs attach across 2 cells, 2 share one eNB
make test-4g-handover     # ue2 travels A -> B -> A with a ping running; Faraday cage recovers
```

## Docs

- [`docs/THEORY.md`](docs/THEORY.md) - background theory
- [`docs/4G.md`](docs/4G.md) - architecture & validation walkthrough

Parts (deep dives):
- [`docs/parts/1-provisioning.md`](docs/parts/1-provisioning.md) - provisioning, crypto, suspend/resume enforcement
- [`docs/parts/2-multi-ue-broker.md`](docs/parts/2-multi-ue-broker.md) - multi-UE / multi-cell (the shared air)
- [`docs/parts/3-handover.md`](docs/parts/3-handover.md) - handover, travel and no coverage

Experiments and findings:
- [`docs/experiments/RADIO_STABILITY.md`](docs/experiments/RADIO_STABILITY.md) - why UEs sometimes never attached, and what fixed it
Scenarios (runnable walkthroughs):
- [`docs/scenarios/1-suspend-resume-subscriber.md`](docs/scenarios/1-suspend-resume-subscriber.md) - provision, suspend, and resume a subscriber
- [`docs/scenarios/2-change-subscriber-plan.md`](docs/scenarios/2-change-subscriber-plan.md) - change a subscriber's plan while attached (signaled live, enforced on next attach)

## Layout

- `deploy/` - Docker Compose stacks and configs
- `tools/` - custom Go tools (provisioner, loadgen)
- `vendor/` - upstream projects as git submodules
- `docs/` - theory, walkthroughs, and more
