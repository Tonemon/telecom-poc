# Scenario: change a subscriber's plan while attached

*A realistic (and slightly surprising) telco behavior: changing a customer's plan reaches the network immediately, but the device they're actively using doesn't feel it until it reattaches.*

This runs on the multi-UE demo topology (`docs/parts/2-multi-ue-broker.md`) using `telcoctl`, the operator-side provisioning client (`docs/parts/1-provisioning.md`). It uses `ue3` specifically, which is the only UE on `broker-b`, so forcing a reattach later in this walkthrough can't disturb any other UE the way it would on `ue1`/`ue2`'s shared cell.

If you already have the multi-UE stack up from another scenario (e.g. `docs/scenarios/1-suspend-resume-subscriber.md`), you can skip straight to [Check the plan, then change it](#check-the-plan-then-change-it-while-attached).

## Why this is interesting

`telcoctl set-plan` writes the subscriber's new AMBR (bandwidth cap) to Mongo. The same HSS `use_mongodb_change_stream` watcher that powers live `suspend` (`docs/parts/1-provisioning.md` §6) reacts to *any* subscriber field change, not just the ones suspend cares about, so this fires a real Diameter S6a **Insert-Subscriber-Data-Request** to the MME (live, while the subscriber is attached and using data).

But Open5GS's handler for that message (`mme_s6a_handle_idr`, `vendor/open5gs/src/mme/mme-s6a-handler.c`) only updates `mme_ue->ambr` in memory. It never sends a `UE Context Modification Request` to push the new rate to the eNB. The *only* place that value is ever transmitted over S1AP is `s1ap_build_initial_context_setup_request`, which is built once, at attach. So the change is real and immediate from the network's point of view, and completely invisible to the device until it reattaches.

`telcoctl get`/`list` surface exactly this gap: the `Status` line always shows the provisioned plan (what's in Mongo, updated the instant `set-plan` runs), while the `Network` line shows the MME's *live* AMBR. The two can disagree for a moment right after a change, and `telcoctl` calls that out explicitly.

## Bring the topology up and provision the demo subscribers

```bash
make 4g-infra
make telcoctl                # Make the provisioning tool if not yet done

make 4g-demo-subscribers
make 4g-demo-devices
```

## Check the plan, then change it while attached

Running the following `telcoctl` command...
```bash
TELCOCTL_TOKEN=dev-operator-token ./bin/telcoctl get 999700000000003
```

... will output:
```
IMSI     999700000000003
Status   active (1G/1G)
Network  connected · registered
         cell 105474 (enb 412) · TAC 1 · PDN internet · QCI 9 · active
Last     add / NEW_ACTIVATION
```

Now downgrade the plan, with the UE still attached and passing traffic, and check again **immediately** (no delay, the HSS's change-stream watcher usually catches up within a second or two, so you have to be quick to see this):

```bash
TELCOCTL_TOKEN=dev-operator-token ./bin/telcoctl set-plan 999700000000003 --dl 5M --ul 2M --reason DOWNGRADE
TELCOCTL_TOKEN=dev-operator-token ./bin/telcoctl get 999700000000003
```

Which shows:
```
IMSI     999700000000003
Status   active (5M/2M)
Network  connected · registered
         cell 105474 (enb 412) · TAC 1 · PDN internet · QCI 9 · active · AMBR 1G/1G (plan not yet applied, reattach pending)
Last     plan / DOWNGRADE
```

`Status` already shows the new plan (that's just a database write), but `Network` still shows the *old* AMBR with an explicit "plan not yet applied" note, which is the MME's actual live state, not yet caught up. Check again a couple of seconds later and the note is gone and the MME has it now too:

```bash
TELCOCTL_TOKEN=dev-operator-token ./bin/telcoctl get 999700000000003
```
```
IMSI     999700000000003
Status   active (5M/2M)
Network  connected · registered
         cell 105474 (enb 412) · TAC 1 · PDN internet · QCI 9 · active
Last     plan / DOWNGRADE
```

That's a real S6a IDR that just happened, live. But `ue3` itself is completely unaffected. This is the same session, same bearer, no interruption:

```bash
docker exec -it telecom-poc-4g-ue3-1 ping -c 2 8.8.8.8
```
```
PING 8.8.8.8 (8.8.8.8) 56(84) bytes of data.
64 bytes from 8.8.8.8: icmp_seq=1 ttl=126 time=61.3 ms
64 bytes from 8.8.8.8: icmp_seq=2 ttl=126 time=45.3 ms

--- 8.8.8.8 ping statistics ---
2 packets transmitted, 2 received, 0% packet loss, time 1001ms
```

## Force a reattach, and the new plan takes hold

No container restart is needed for this as `suspend` immediately triggers a real CLR-based live detach, and `resume` lets the UE's own rejected-auto-reattach-then-T3402-retry cycle (the same self-healing mechanism `docs/scenarios/1-suspend-resume-subscriber.md` walks through) bring it back with a fresh attach. This is exactly what an operator toggling a stuck line looks like in real life, and it's what a support agent telling a customer "give it a minute, it should catch up" is actually relying on.

**suspend and resume must not be fired back-to-back**. The HSS's change-stream watcher polls every 100ms but isn't instant, and calling resume immediately risks it never observing the transient suspended state at all (confirmed live: without a wait in between, the detach sometimes silently doesn't happen). Wait for a ping to actually fail before resuming:

```bash
TELCOCTL_TOKEN=dev-operator-token ./bin/telcoctl suspend 999700000000003 --reason MAINTENANCE --note "force resync onto new plan"

# wait until a ping genuinely fails, proving the live detach landed, THEN:
TELCOCTL_TOKEN=dev-operator-token ./bin/telcoctl resume 999700000000003 --reason CLEARED --note "force resync onto new plan"
```

Give it up to a minute to reattach on its own, then check again. No more "plan not yet applied" note, because this time it's what the fresh attach actually initialized the session with, not a stale in-memory leftover:

```bash
TELCOCTL_TOKEN=dev-operator-token ./bin/telcoctl get 999700000000003
```
```
IMSI     999700000000003
Status   active (5M/2M)
Network  connected · registered
         cell 105474 (enb 412) · TAC 1 · PDN internet · QCI 9 · active
Last     resume / CLEARED
```

## Run it as an automated assertion

The full before/after/reattach sequence above is scripted end-to-end in `deploy/4g/scripts/assert-plan-change-no-live-effect.sh`, which is topology-agnostic:

```bash
COMPOSE_FILES="-f deploy/4g/docker-compose.yml -f deploy/4g/docker-compose.multi.yml" UE_SVC=ue3 IMSI=999700000000003 ./deploy/4g/scripts/assert-plan-change-no-live-effect.sh
```

It's also wired into `make test-provisioner-lifecycle` (against the single-UE stack, its defaults).
