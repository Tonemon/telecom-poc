# Handover, travel and no coverage (the air as a world)

*A UE drives from cell A to cell B with a ping running, and the network hands it over without dropping the connection. A UE in a Faraday cage loses service and gets it back by itself.*

This builds on the shared air from [`2-multi-ue-broker.md`](2-multi-ue-broker.md). Read that first if "the air" is new to you. Why the radio needed hardening before any of this could work is in [`../experiments/RADIO_STABILITY.md`](../experiments/RADIO_STABILITY.md).


## 1. What a handover is, and which kind this is

A UE that is connected (has an RRC connection and a data bearer) keeps measuring the cells around it. When a neighbour gets clearly better than the serving cell, the serving eNB moves the UE to the neighbour while the session continues: same IP, same bearer, a few packets lost at most. That is a **handover**. An idle UE does not hand over; it just camps on a different cell (cell reselection).

There are two ways two eNBs can coordinate a handover:

- **X2 handover**: the eNBs talk to each other directly over X2, and only tell the MME afterwards.
- **S1 handover**: the eNBs have no direct link; everything goes through the MME over S1AP.

srsRAN_4G's eNB has no X2 interface, so this stack does **S1 handovers**. The flow is the same idea with the MME in the middle (§5 shows it in a capture).


## 2. The world: positions, distance and signal

The air is not only a mixer any more, it is a small world. `deploy/4g/broker/world.py` keeps a 2D position in kilometres for every cell and UE (`deploy/4g/config/multi/air.json`):

```
        ue1   ue2                                          ue3
  A------1-----2-------------------------------------------3-----B
  0 km                                                         10 km
```

| | Position | Starts on |
|---|---|---|
| cell A (`enb-a`) | (0, 0) | |
| cell B (`enb-b`) | (10, 0) | |
| `ue1` | (1, 0) | cell A |
| `ue2` | (2, 0) | cell A |
| `ue3` | (9, 0) | cell B |

For every UE/cell pair the air turns the distance into a **gain** (the amplitude it multiplies that link's samples with), with a simple log-distance path loss:

- up to 1 km: gain 1.0 (full signal);
- beyond that the power falls with distance³ (`exponent: 3`), so the amplitude is `(1/d)^1.5`: 0.35 at 2 km, 0.19 at 3 km, 0.09 at 5 km;
- from 7 km on (`edge_km: 7`): gain 0, no signal at all. The virtual radio has no noise floor, so without a hard edge a cell would be decodable at any distance.

So both cells are heard between x = 3 and x = 7 km (the overlap), and nothing is heard from x = 17 km on. The same gain is used for the downlink (UE hears the cell) and the uplink (cell hears the UE).

Both cells use the same frequency (EARFCN 3350), like most real LTE networks. That makes the neighbour cell **interference** as well as a candidate: once the neighbour is about 8 dB stronger than the serving cell, the UE can no longer decode the serving cell at all. A handover must happen well before that point.


## 3. Radio time

The virtual radio runs slower than the clock: on the test host about 0.1× real time (one second of LTE takes about ten seconds). LTE timers run in radio time, so a 640 ms trigger delay takes about 6 s of wall time.

Travel therefore also runs in **radio time**. The air counts the samples that have passed through it (`radio_samples()` in `broker.py`); every 0.2 s of wall time it tells the world how many radio seconds passed, and moving UEs advance by `speed × radio seconds`. A UE at 1000 km/h moves at 1000 km/h as far as the LTE procedures can tell, however slowly the host runs. That is why the speeds look absurd (1000 km/h) while a 6 km trip still takes a few minutes of real time.

The speed limit is the handover itself: a handover takes a few hundred milliseconds of radio time after the trigger delay. Above about 1500 km/h the UE crosses from "neighbour slightly better" to "serving cell lost" faster than that, and drops the connection instead (radio link failure, then a new attach with a new IP).


## 4. The handover configuration

Each eNB's `rr-*.conf` (`deploy/4g/config/multi/rr-a.conf`, `rr-b.conf`) has:

| Setting | Value | Why |
|---|---|---|
| `ho_active` | `true` | Allow handovers from this cell |
| `meas_cell_list` | the other cell: ECI `0x19C02` (in `rr-a`) / `0x19B01` (in `rr-b`), PCI, EARFCN 3350 | Tells the UE which neighbour to measure and the eNB where to send it |
| `eventA = 3` | A3 | "Neighbour becomes better than serving by an offset" |
| `a3_offset` | 4 (= 2 dB) | Units of 0.5 dB |
| `hysteresis` | 2 (= 1 dB) | Together 3 dB: a UE in the overlap does not bounce back and forth |
| `time_to_trigger` | 640 ms | The condition must hold this long (radio time) |
| `root_seq_idx` | 204 (A) / 264 (B) | Different PRACH sequences per cell: in the overlap a UE's random access reaches both eNBs |

With these settings, a UE travelling from A to B hands over around x = 5.9 km, and one travelling from B to A around x = 4.1 km (the 3 dB margin works in both directions).

The ECI is `enb_id << 8 | cell_id`: cell A is eNB `0x19B` (411) cell `0x01`, cell B eNB `0x19C` (412) cell `0x02`.


## 5. Try it: make a UE travel

Bring the multi-cell stack up and start a ping on `ue2`:

```bash
make 4g-multi
C="docker compose -f deploy/4g/docker-compose.yml -f deploy/4g/docker-compose.multi.yml"
$C exec -d ue2 sh -c 'ping -I tun_srsue -i 0.2 8.8.8.8 > /tmp/ping.log 2>&1'
```

`make 4g-travel` without options shows the help and where everyone is:

```bash
make 4g-travel
```

```
        1     2                                         3
  A-----------------------------------------------------------B
  0 km                                                    10 km

  UE   position     travel                               gain A/B    serving
  ue1  (1.0, 0.0)   -                                    1.00/0.00   cell A (enb-a)
  ue2  (2.0, 0.0)   -                                    0.35/0.00   cell A (enb-a)
  ue3  (9.0, 0.0)   -                                    0.00/1.00   cell B (enb-b)

  radio runs at 0.10x real time (1 radio-s so far)
```

The serving cell comes from the MME (through the provisioner), not from the air: it shows what the network thinks, not what the radio does.

Send `ue2` to cell B and follow it:

```bash
make 4g-travel UE=ue2 TO=8
make 4g-travel WATCH=1        # refreshes every 2 s, Ctrl-C to stop
```

Around x = 5.9 the serving cell switches from `cell A` to `cell B`. Then stop the ping and look:

```bash
$C exec ue2 pkill -INT ping; $C exec ue2 tail -3 /tmp/ping.log   # a few lost pings at most
$C logs ue2 | grep -E "HO to PCI|Received HO command"
$C logs enb-a enb-b | grep -iE "handover|HO "
```

All options:

| Command | What it does |
|---|---|
| `make 4g-travel UE=ue2 TO=8` | Travel to x = 8 km (y = 0) at 1000 km/h |
| `make 4g-travel UE=ue2 TO=8,3 SPEED=600` | Travel to (8, 3) at 600 km/h |
| `make 4g-travel UE=ue2 AT=5` | Jump there right away (cancels a travel) |
| `make 4g-travel UE=ue3 SHIELD=on` | Faraday cage: no radio at all (`SHIELD=off`: back) |
| `make 4g-travel WATCH=1` | Live view |

`make 4g-travel` is a client of the air's HTTP API on `127.0.0.1:8090` (Bearer token `AIR_TOKEN`, default `dev-air-token`): `GET /world`, `POST /ues/{id}/travel`, `PUT /ues/{id}/position`, `PUT /ues/{id}/shielded`, and `PUT /ues/{id}/gains` to set raw gains by hand (kept until the UE moves again). For example:

```bash
curl -s -H "Authorization: Bearer dev-air-token" http://127.0.0.1:8090/world | python3 -m json.tool
```

### What happens on the wire

An S1 handover follows this sequence (3GPP TS 36.413 and 23.401). The packet capture targets only cover the single-cell stack for now, so on this stack you see it in the logs (`$C logs mme | grep -i handover`, plus the eNB and UE lines above). In order:

| Step | Message | Between |
|---|---|---|
| 1 | Measurement Report (A3) | UE → source eNB (RRC, not on S1) |
| 2 | `HandoverRequired` | source eNB → MME |
| 3 | `HandoverRequest` / `HandoverRequestAcknowledge` | MME ↔ target eNB |
| 4 | `HandoverCommand` | MME → source eNB, which sends the RRC reconfiguration to the UE |
| 5 | `eNBStatusTransfer` / `MMEStatusTransfer` | source eNB → MME → target eNB (PDCP sequence numbers) |
| 6 | random access on the target cell, then `HandoverNotify` | UE → target eNB → MME |
| 7 | `Modify Bearer Request` / `Response` (GTPv2) | MME ↔ SGW: downlink now goes to the target eNB |
| 8 | `UEContextReleaseCommand` / `Complete` | MME ↔ source eNB |

There is no new authentication, no new session and no new IP: only the radio side and the S1-U tunnel move.


## 6. No coverage: dead zone and Faraday cage

Two ways to take a UE out of coverage:

- **Dead zone**: travel beyond 7 km from both cells, e.g. `make 4g-travel UE=ue3 TO=30`. The gains fade to 0 on the way.
- **Faraday cage**: `make 4g-travel UE=ue3 SHIELD=on`. All its gains go to 0 at once; the position stays.

The network does not notice right away. The UE loses the cell (radio link failure); the eNB only releases it once its own timers run out, and until then the MME still lists it as connected. `make 4g-travel` shows that as `no radio (MME still says cell B (enb-b))`.

Back in coverage (`SHIELD=off`, or travel back), the UE finds the cell again and reconnects by itself. No container restarts.


## 7. The acceptance test

```bash
make test-4g-handover
```

From a stopped stack, it brings everything up, starts a ping (every 0.2 s) on all three UEs, and then:

1. sends `ue2` to x = 8 (cell B) and waits until the travel is done, `ue2` logged a successful handover, and the MME has it on eNB 412 (cell B);
2. sends it back to x = 2 and waits for the same towards cell A (eNB 411);
3. checks that `ue2` kept its IP (a reattach would give it a new one), and that `ue2` lost at most 5 % of its pings and `ue1`/`ue3` at most 1 %;
4. shields `ue3`: after 30 s it must have no service; unshields it: it must be back within 60 s.

A run takes about 10 minutes, mostly the two trips (each ≈ 3.5 min of real time at 1000 km/h). `TRIP_TIMEOUT` (default 420 s) is the limit per trip.

Results, 3 runs each from a fully destroyed stack (FFT cache warm except in run 1, which was the first boot after a host restart):

| Run | A → B | B → A | ue2 kept IP | Pings lost ue2 / ue1 / ue3 | Back after Faraday cage | Run time |
|---|---|---|---|---|---|---|
| 1 | 208 s | 209 s | ✓ | 0.28 % / 0.19 % / 0.09 % | 6 s | 28 min |
| 2 | 203 s | 221 s | ✓ | 0.14 % / 0.09 % / 0.05 % | 7 s | 9.5 min |
| 3 | 208 s | 215 s | ✓ | 0.05 % / 0.14 % / 0.19 % | 7 s | 9.5 min |

With a ping every 0.2 s, 0.1 % is about one ping in a thousand: each handover cost `ue2` at most a ping or two. The other UEs lose about as many, so part of that is the virtual radio itself, not the handover.


## 8. Known limits

- **Speed**: above about 1500 km/h radio speed, a UE can lose its connection instead of handing over (§3).
- **A UE container that restarts cannot rejoin the running air.** The radio has no timestamps, so a restarted endpoint cannot line up with the others again; restart the radio side with `make 4g-devices`. This is the "hot-join" step on the roadmap.
- **New IPs after a restart**: each UE gets a new IP from the network every time the radio side is restarted.
- **One frequency**: both cells share an EARFCN, so all handovers are intra-frequency; no measurement gaps are needed.


## 9. Try it yourself (exercises)

1. **Too fast.** `make 4g-travel UE=ue2 TO=8 SPEED=5000`. Does `ue2` hand over, or lose the connection and attach again (new IP)? Check `$C logs ue2 | grep -E "HO to|RLF|Attach"`.
2. **No margin.** Set `a3_offset = 0` and `hysteresis = 0` in both `rr-*.conf`, restart, park `ue2` at x = 5 (`AT=5`) and count the handovers in its log over a few minutes (ping-pong).
3. **Too much margin.** Set `a3_offset = 16` (8 dB). Where does the UE lose cell A, and does it still hand over?
4. **Follow the signalling.** While `ue2` travels, follow `$C logs -f mme enb-a enb-b` and match the lines to the steps in the table in §5. Which step does the MME log, and which only the eNBs?
5. **The dead zone.** Send `ue3` to x = 30 and back to x = 9, timing how long the MME keeps saying "connected" after the radio is gone (`make 4g-travel WATCH=1`).
6. **Move the towers.** Put cell B at (6, 0) in `air.json`, rebuild with `make 4g-multi`, and find the new handover points.
