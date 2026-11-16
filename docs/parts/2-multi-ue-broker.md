# Multi-UE / multi-cell (the shared air)

*Running several UEs on one eNB, and more than one eNB, over the virtual radio.*

This walkthrough explains why the single-UE stack could only ever attach one phone, the GNU Radio **air** we put between all eNBs and UEs to lift that limit, and the steps you can run yourself to reproduce the results. Moving UEs between the cells (handover) builds on this and is in [`3-handover.md`](3-handover.md).

> **History.** The first version used one GNU Radio *broker* per cell (`broker-a`, `broker-b`), each wiring one eNB to its own UEs. Those were replaced by one shared **air** for both cells, so a UE can hear both cells at once, which is what handover needs. The program is still called `broker.py` and its image `broker.Dockerfile`.


## 1. Why one ZMQ link = one UE

The virtual radio between `srsenb` and `srsue` is a ZeroMQ link carrying IQ samples: one socket sends downlink, one receives uplink. It is **point-to-point**, where a `tx` socket binds and a single `rx` socket connects to it. The eNB's `rx` (a REQ socket) connects *out* to one hardcoded peer address (it never binds or listens for incoming connections) so there is no summation and no fan-out, and no way for a second UE to be heard at all without reconfiguring the eNB to point at it instead. That is a property of the transport, not of LTE: the eNB scheduler is perfectly happy to serve many UEs, but the plumbing underneath only carries one.

So to get multi-UE and multi-cell we need something in the middle that behaves like the air does: **combine** every UE's uplink into the stream each eNB expects, and **combine** every eNB's downlink into the stream each UE expects.


## 2. The air

The air is one GNU Radio flowgraph (`deploy/4g/broker/broker.py`, service `air`) between every eNB and every UE:

- **Downlink**: each UE receives the sum of every cell's transmit stream, each multiplied by a gain for that UE/cell pair.
- **Uplink**: each cell receives the sum of every UE's transmit stream, with the same gains.

A gain is a number between 0 ("out of range") and 1 ("full signal"). Summation is what the RF channel does naturally when several transmitters are active at once. On a fixed topology the gains are simply 1 or 0; with handover, the air computes them from the distance between UE and cell (see part 3), and they change while the stack runs.

All endpoints run in one **lockstep**: every receive is a request that waits for its reply, so all eNBs and UEs advance together, at the speed of the slowest one. No sample is ever dropped or inserted, which matters because there are no timestamps on the wire: position in the stream *is* time (see [`../experiments/1-radio-stability.md`](../experiments/1-radio-stability.md) §2).


## 3. How the code is split

The wiring decision is **pure and testable** without GNU Radio installed:

| File | What it does |
|---|---|
| `air.py` | Loads `air.json` (cells, UEs, ZMQ addresses, sample rate) and plans the links: for each UE the list of cells it hears, for each cell the list of UEs |
| `world.py` | Positions, travel and path loss: turns distances into gains (part 3) |
| `control.py` | Glue between the world and the radio: pushes changed gains into the flowgraph |
| `api.py` | The HTTP control API on port 8090 (part 3) |
| `broker.py` | Imports GNU Radio and builds the flowgraph from the plan |

The ZMQ role convention throughout: **a `tx` endpoint binds (REP), an `rx` endpoint connects (REQ)**. So the air *connects* to each eNB's and UE's tx, and *binds* the sockets each eNB's and UE's rx connects to.

The unit tests (`test_air.py`, `test_world.py`, `test_control.py`, `test_api.py`) run during the image build, which fails if one regresses (`deploy/images/broker.Dockerfile`):

```dockerfile
RUN cd /opt/broker && python3 -m unittest discover -p 'test_*.py'
```

Run them directly if you want:

```bash
cd deploy/4g/broker && python3 -m unittest discover -p 'test_*.py' -v
```


## 4. How it is wired (the ZMQ ports)

Every RF endpoint runs at the **same sample rate**, `base_srate = 11.52e6` (25 PRB, 5 MHz). If the eNBs, the air and the UEs disagree, the samples misalign and nothing decodes. The air (172.22.0.41) sits in the middle:

| Direction | Sender binds (tx) | The air | Receiver connects (rx) |
|---|---|---|---|
| Downlink | `enb-a :2000`, `enb-b :2000` | per UE: gain × each cell, summed; served on `:2111`, `:2112`, `:2113` | `ue1` → `:2111`, `ue2` → `:2112`, `ue3` → `:2113` |
| Uplink | `ue1 :2001`, `ue2 :2001`, `ue3 :2001` | per cell: gain × each UE, summed; served on `:2100` (A), `:2200` (B) | `enb-a` → `:2100`, `enb-b` → `:2200` |

The configs only name addresses:

```
# enb-a.conf   device_args = ...,tx_port=tcp://*:2000,rx_port=tcp://172.22.0.41:2100,...
# ue1.conf     device_args = tx_port=tcp://*:2001,rx_port=tcp://172.22.0.41:2111,...
# air.json     cells: A dl tcp://172.22.0.40:2000, ul tcp://*:2100; ue1 dl tcp://*:2111, ...
```

Which cell a UE uses is no longer an addressing choice: every UE is wired to every cell, and the gains decide what it hears.


## 5. The demo topology: 2 cells, 3 UEs

`ue1` and `ue2` start near cell A, `ue3` near cell B, all sharing the one unchanged EPC:

| Cell | eNB (`enb_id`) | PCI / `cell_id` | Position | UEs at start |
|---|---|---|---|---|
| A | `enb-a` (.40, 0x19B) | 1 / 0x01 | (0, 0) | `ue1` (.61) at 1 km, `ue2` (.62) at 2 km |
| B | `enb-b` (.50, 0x19C) | 2 / 0x02 | (10, 0) | `ue3` (.63) at 9 km |

The two cells share `dl_earfcn = 3350` but carry **distinct PCI / cell_id / enb_id**, so the MME sees them as two genuine cells. Because the air mixes both cells into each UE, a UE between them really does hear both, on the same frequency (part 3). Each UE has its own IMSI/keys in `config/multi/ue*.conf`, provisioned as three fixed subscribers by `scripts/provision-multi.sh` (keys match the configs, so they actually authenticate).


## 6. Start order: the air **last**, and restart together

The air *connects* to the eNB and UE tx sockets, and the lockstep starts the moment it does. Two consequences:

1. **The air starts last**, after every eNB and UE says its radio is ready (`scripts/wait-radio-ready.sh`; the `make` targets do this for you).
2. **The radio side restarts as a whole.** There are no timestamps on the wire, so an endpoint that restarts cannot line up with the others again. `make 4g-devices` therefore recreates the eNBs, the UEs and the air together (the core and the subscribers stay). Letting a single UE join or leave a running air is the "hot-join" step on the roadmap.

This doesn't come up for `telcoctl suspend`/`resume`: that only tears down the S1-U bearer and NAS session, never the radio, so a suspended-then-resumed UE reattaches on its own (see `docs/4G.md` §2) without any container restarting.


## 7. The `make` surface

```bash
# one-shot (choose one)
make 4g-auto      # 2 eNBs + 3 UEs, provision, attach, ping all three
make 4g-multi     # same, without the closing ping
make 4g-single    # 1 eNB / 1 UE, direct link (no air)

# in stages
make 4g-infra             # EPC core + both eNBs, no subscribers, no UEs
make 4g-demo-subscribers  # the 3 fixed subscribers
make 4g-devices           # ue1 + ue2 + ue3 + the air; attach + ping all three
make 4g-devices-down      # remove the 3 UEs + the air; core, eNBs and subscribers stay

make 4g-travel            # move UEs between the cells (part 3)
make 4g-auto-down         # tear it all down
```


## 8. Validation

**8.1 The planner and world (no Docker needed):**

```bash
cd deploy/4g/broker && python3 -m unittest discover -p 'test_*.py' -v
```

**8.2 Two UEs on one eNB (the point of all this).** Bring the whole thing up and confirm two UEs are concurrently attached to `enb-a`:

```bash
make 4g-multi
make status-4g                       # ue1, ue2, ue3 and air all Up
docker compose -f deploy/4g/docker-compose.yml -f deploy/4g/docker-compose.multi.yml exec -T ue2 ping -I tun_srsue -c 3 8.8.8.8
```

**8.3 The acceptance test.** One command that does everything automatically: provisions, starts everything in the right order, waits for all three to attach, pings **concurrently**, and asserts two UEs really are on `enb-a`:

```bash
make test-4g-multi
# ... PASS: 3 UEs attached across 2 cells; 2 concurrent on enb-a.
```

**8.4 How the "two on one eNB" assert avoids lying to itself.** Counting *any* RNTI in the eNB log over-counts: transient RA-RNTIs from the random-access preamble would inflate it. So `scripts/assert-two-on-one-enb.sh` counts **distinct C-RNTIs on GTP-U (data-plane) log lines only**, which appear solely for UEs actually passing user traffic. That's why 8.3 pings every UE *before* asserting. You can see the raw evidence:

```bash
docker compose -f deploy/4g/docker-compose.yml -f deploy/4g/docker-compose.multi.yml logs enb-a | grep GTPU | grep -oiE "rnti=0x[0-9a-f]+" | sort -u
# two distinct RNTIs = two UEs carrying data on one cell
```


## 9. What this sets up

With every UE wired to every cell, the air is the radio world: change a gain and a UE moves. Part 3 uses that for handover, dead zones and a Faraday cage. Next on the roadmap: UEs that join and leave a running air (hot-join), and later a map that shows and drives it all.
