# Multi-UE / multi-cell (ZMQ broker)

*Running several UEs on one eNB, and more than one eNB, over the virtual radio.*

This walkthrough explains why the single-UE stack could only ever attach one phone, the GNU Radio **broker** we inserted to lift that limit, and the exact steps you can run yourself to reproduce every result we verified while building it.


## 1. Why one ZMQ link = one UE

The virtual radio between `srsenb` and `srsue` is a ZeroMQ link carrying IQ samples: one socket sends downlink, one receives uplink. It is **point-to-point**, where a `tx` socket binds and a single `rx` socket connects to it. The eNB's `rx` (a REQ socket) connects *out* to one hardcoded peer address (it never binds or listens for incoming connections) so there is no summation and no fan-out, and no way for a second UE to be heard at all without reconfiguring the eNB to point at it instead. That is a property of the transport, not of LTE: the eNB scheduler is perfectly happy to serve many UEs, but the plumbing underneath only carries one.

So to get multi-UE (and, later, multi-cell for handover) we need something in the middle that behaves like the air does: **combine** every UE's uplink into the one stream the eNB expects, and **copy** the eNB's downlink out to every UE.


## 2. The fix: a GNU Radio broker per cell

A *cell* becomes `eNB + broker + its UEs`. The broker is a tiny GNU Radio flowgraph (`deploy/4g/broker/broker.py`) that sits between the eNB and its UEs:

- **Downlink**: one ZMQ source pulls the eNB's transmit stream and fans it out to one sink per UE (each UE reads an identical copy).
- **Uplink**: one ZMQ source per UE, all summed with an `add_cc` block, into a single sink the eNB reads. Summation is what the RF channel would do naturally when two handsets transmit at once.

Because there is no real RF, "sum the uplinks" is exactly right: no path loss, no interference model, just add the complex samples.


## 3. The broker, and why it's split in two

The flowgraph is written so the **wiring decision is pure and testable** without GNU Radio installed. `plan_broker()` takes the eNB endpoints and the UE list and returns a `BrokerPlan` (which sockets bind, which connect); `main()` imports GNU Radio lazily and builds the flowgraph from that plan. The ZMQ role convention throughout: **a `tx` endpoint binds (REP), an `rx` endpoint connects (REQ)**, so the broker *connects* to the eNB's tx and to each UE's tx, and *binds* the sinks the eNB rx and UE rx sockets connect to.

`deploy/4g/broker/test_broker.py` unit-tests the planner (one UE and two UEs), and the image build **runs that test and fails if it regresses** (`deploy/images/broker.Dockerfile`):

```dockerfile
RUN cd /opt/broker && python3 -m unittest test_broker
```

Run it directly if you want:

```bash
cd deploy/4g/broker && python3 -m unittest test_broker -v
```

The broker is configured entirely by environment (`ENB_DL`, `ENB_UL`, `SRATE`, `UES`) so one image serves every cell (see the `broker-a` / `broker-b` services in `deploy/4g/docker-compose.multi.yml`).


## 4. How a cell is wired (the ZMQ ports)

Every RF endpoint runs at the **same sample rate**, `base_srate = 23.04e6`. If the eNB, broker and UEs disagree the samples misalign and nothing decodes. Cell A (`enb-a` + `broker-a` + `ue1`, `ue2`) is wired like this:

| Direction | Binds (tx / REP) | Broker in the middle | Connects (rx / REQ) |
|---|---|---|---|
| Downlink | `enb-a` tx `:2000` | source ← eNB, fan-out sinks `:2111`, `:2112` | `ue1` rx → `:2111`, `ue2` rx → `:2112` |
| Uplink | broker sink `:2100` | sources ← `ue1:2001`, `ue2:2001`, summed | `enb-a` rx → broker `:2100` |

Concretely, the config that makes `ue1` and `ue2` land on the **same** eNB is just their ZMQ addresses. The eNB never sees two separate links, only the one merged stream from the broker:

```
# enb-a.conf   device_args = ...,tx_port=tcp://*:2000,rx_port=tcp://172.22.0.41:2100,...
# ue1.conf     device_args = tx_port=tcp://*:2001,rx_port=tcp://172.22.0.41:2111,...
# ue2.conf     device_args = tx_port=tcp://*:2001,rx_port=tcp://172.22.0.41:2112,...
```

A UE's cell membership is therefore an *addressing* choice (which broker port its `rx` points at), not a frequency choice.


## 5. The demo topology: 2 cells, 3 UEs

The shipped topology is **`ue1` + `ue2` on `enb-a`, `ue3` on `enb-b`**, all sharing the one unchanged EPC:

| Cell | eNB (`enb_id`) | PCI / `cell_id` | Broker | UEs |
|---|---|---|---|---|
| A | `enb-a` (0x19B) | 1 / 0x01 | `broker-a` (.41) | `ue1` (.61), `ue2` (.62) |
| B | `enb-b` (0x19C) | 2 / 0x02 | `broker-b` (.51) | `ue3` (.63) |

The two cells share `dl_earfcn = 3350` (harmless with no real RF, the ZMQ links are electrically isolated) but carry **distinct PCI / cell_id / enb_id**, so the MME sees them as two genuine cells. Each UE has its own IMSI/keys in `config/multi/ue*.conf`, provisioned as three fixed subscribers by `scripts/provision-multi.sh` (keys match the configs, so they actually authenticate).


## 6. Start order: brokers **last**, and restart-together

The broker *connects* to the eNB and UE tx sockets, so those must be listening first, and the eNB side (`enb-a.conf`/`enb-b.conf`, not the UE configs) runs with `fail_on_disconnect=true`. If its peer drops, the eNB's RF worker exits. Two consequences:

1. **Bring brokers up last**, after their eNB and UEs (the `4g-multi` / `4g-demo-devices` targets `sleep` then start brokers with `--no-deps`).
2. **Recovering one endpoint means recreating the whole cell, not just that endpoint.** The broker holds one ZMQ socket per UE plus one to the eNB; restarting any single member (the eNB, or any one UE) stales the broker's *other* sockets too, since a fresh peer connection needs a fresh socket on both ends. So reconnecting `ue1` on Cell A means recreating `enb-a` + `broker-a` + `ue1` **and** `ue2` (even though nobody touched `ue2`), not just the one endpoint that actually changed.

Even the full, correctly-ordered coordinated recreate isn't guaranteed to re-pair cleanly. The only recovery verified to work consistently once a cell gets stuck is a full teardown and redeploy: `make 4g-auto-down && make 4g-multi`.

This doesn't come up for `telcoctl suspend`/`resume`: that only tears down the S1-U bearer and NAS session, never the ZMQ radio link, so a suspended-then-resumed UE reattaches on its own (see `docs/4G.md` §2) without any container ever restarting.


## 7. The `make` surface

The same topology is reachable one-shot or in stages (full table in `README.md`):

```bash
# multiple possible one-shot deployments (choose one)
make 4g-auto      # 2 eNBs + 3 UEs, provision, attach, ping all three
make 4g-multi     # same, without the closing ping
make 4g-single    # 1 eNB / 1 UE (the pre-broker single-cell stack)

# in stages
make 4g-infra             # EPC core + both eNBs, no subscribers, no UEs
make 4g-demo-subscribers  # The 3 fixed subscribers
make 4g-demo-devices      # all three: ue1+ue2+ue3 + brokers
make 4g-device            # OR only ONE device: ue1 -> sub …001 -> enb-a


make 4g-auto-down   # tear it all down
```


## 8. Validation

The stack was built up in layers, each independently checked.

**8.1 The broker planner (no Docker needed).** Proves the wiring logic before any radio is involved:

```bash
cd deploy/4g/broker && python3 -m unittest test_broker -v
```

**8.2 Single UE through the broker (parity).** The first milestone was one UE attaching *through* the broker exactly as it did over the direct link, proving the broker is transparent. That is now `make 4g-device` (ue1 alone through broker-a):

```bash
make 4g-infra && make 4g-demo-subscribers && make 4g-device
```

Expect the `4g-device` run to end in `0% packet loss` pinging `8.8.8.8` from `ue1`.

**8.3 Two UEs on one eNB (the real point).** Bring the whole thing up and confirm two UEs are concurrently attached to `enb-a`:

```bash
make 4g-multi
make status-4g                       # ue1, ue2, ue3 all Up
docker compose -f deploy/4g/docker-compose.yml -f deploy/4g/docker-compose.multi.yml exec -T ue2 ping -I tun_srsue -c 3 8.8.8.8
```

**8.4 The full acceptance test.** One command that does everything automatically: provisions, starts everything in the right order, waits for all three to attach, pings **concurrently**, and asserts two UEs really are on `enb-a`:

```bash
make test-4g-multi
# ... PASS: 3 UEs attached across 2 cells; 2 concurrent on enb-a.
```

**8.5 How the "two on one eNB" assert avoids lying to itself.** Counting *any* RNTI in the eNB log over-counts, transient RA-RNTIs from the random-access preamble would inflate it. So `scripts/assert-two-on-one-enb.sh` counts **distinct C-RNTIs on GTP-U (data-plane) log lines only**, which appear solely for UEs actually passing user traffic. That's why 8.4 pings every UE *before* asserting. You can see the raw evidence:

```bash
docker compose -f deploy/4g/docker-compose.yml -f deploy/4g/docker-compose.multi.yml logs enb-a | grep GTPU | grep -oiE "rnti=0x[0-9a-f]+" | sort -u
# two distinct RNTIs = two UEs carrying data on one cell
```


## 9. What this sets up

The broker is the RAN-plumbing prerequisite for the rest of the roadmap. A UE that can hear **two** cells is the setup for **X2 handover**; several UEs on a cell is the setup for **multi-UE / roaming** scenarios. And because cell membership is now just an addressing choice, it is the foundation for a device-provisioning UX (naming a handset and choosing which subscription and which eNB it uses) which is exactly the shape `make 4g-device` (and a future `make 5g-device`) already takes.
