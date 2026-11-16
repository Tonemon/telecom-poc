# Radio stability: why UEs sometimes never attached, and what fixed it

This document covers a two-day investigation (November 2026) into the multi-cell 4G stack (`make 4g-multi` and friends: two eNBs, three UEs, one virtual radio). Before it, **1 run in 15** had all three UEs attached and pinging. It explains what was wrong, how we found it, what was tried and did not help, and what the stack does now.

For the stack itself see [`4G.md`](../4G.md); for the multi-UE radio see [`parts/2-multi-ue-broker.md`](../parts/2-multi-ue-broker.md), and for the handover built on top of it [`parts/3-handover.md`](../parts/3-handover.md). Two of the problems are bugs in srsRAN itself.

---

## 1. Short version

| # | Problem | Effect | Fix |
|---|---|---|---|
| 1 | eNB option `fail_on_disconnect=true`: a receive timeout half-way through a subframe throws the samples away without advancing the eNB's clock (srsRAN bug) | That eNB's timeline shifts by a fraction of a subframe for good; its UEs' attach attempts (PRACH) are never detected. **Random per run: the main failure.** | Option removed from both eNBs |
| 2 | srsRAN benchmarks every FFT size at start (`FFTW_MEASURE`), caches it only in `$HOME`, which a fresh container never has | ~3 min "Waiting PHY to initialize" on every start, worse under load | Per-endpoint cache folder `deploy/4g/.fftw/` |
| 3 | UE entrypoint is PID 1 and did not pass `SIGTERM` to srsue | UEs were killed instead of stopped: no clean detach, FFT cache never saved | Signal forwarded (`trap`) |
| 4 | All eNBs/UEs logged the PHY layer at `info` | ~10 MB/min per UE; logs of the attach phase rotated away; extra load | PHY logging at `warning` |
| 5 | The air (radio broker) started after a fixed `sleep 5` | Air could start while a UE was still starting: early stalls | `wait-radio-ready.sh`: start the air only when every radio says it is ready |
| 6 | `make` gave each UE 40 s to attach | Slow-but-fine attaches counted as failures | Wait until attached, limit 3 min (`ATTACH_TIMEOUT`), with progress |
| 7 | 50 PRB (10 MHz) at 23.04 MHz sampling, host load ≈ 19 on 8 CPUs | Overload makes the lockstep stall for seconds at a time | 25 PRB (5 MHz), 11.52 MHz sampling: half the work everywhere |
| 8 | srsRAN's `log_trx_timeout` option could never be switched on (srsRAN bug) | Problem 1 was invisible in the logs | One-line patch, `deploy/patches/srsran-zmq-log-trx-timeout.patch` |
| 9 | The srsRAN image compiled every unit test and example | ~20 min image build | Build only `srsenb`, `srsue` and the ZMQ radio plugin |

Also done along the way: the two per-cell brokers were replaced by **one shared "air"** (one GNU Radio flowgraph for both cells and all UEs, a gain per UE/cell pair), the base for handover. It did not cause or fix the failures, but made the decisive test possible (§4.4).

---

## 2. How the virtual radio works (just enough to follow the rest)

There is no real RF. srsRAN's ZMQ "radio" sends IQ samples over TCP. Each eNB and UE has a transmit socket (answers requests) and a receive socket (sends requests). The air connects to all of them and, per sample, adds up what each receiver should hear.

Two properties matter:

1. **Position in the stream is time.** There are no timestamps on the wire. At 11.52 MHz sampling, the 11 520th sample an eNB receives *is* 1 ms after the first. If an endpoint ever loses or skips samples, its idea of time is shifted from then on, and nothing can correct it.
2. **Lockstep.** Every request waits for its reply, so all endpoints advance together, at the speed of the slowest. On a busy host the whole simulation runs at only ≈ 5 % of real time (LTE timers stretch accordingly: a 15 s NAS timer took 232 s of wall time). A stall is harmless; a slip (lost samples) is fatal.

Why a slip is fatal for attach: a UE takes its timing from the downlink it receives, and sends its first message (the PRACH preamble) in a fixed subframe. The eNB only looks for preambles in that subframe, with ≈ 0.1 ms of tolerance. If the eNB's receive clock is shifted against its own transmit clock, every preamble lands outside that window. The UE keeps trying, the eNB never sees anything, and the transport looks perfectly healthy.

---

## 3. Symptoms before the fix

- `make 4g-multi` / `4g-auto` / `test-4g-multi`: usually one UE (most often ue3, sometimes ue1) did not attach within the 40 s the targets allowed. **1 of 15** runs fully passed.
- With a 3 min window, most "failures" were slow attaches (≈ 2 min, from problem 2), but ue3 regularly never attached at all.
- The failing UE: `Found Cell`, SIB1/SIB2 read, `Attach Request`, `Random Access Transmission` ×10, `Maximum number of transmissions reached`; downlink SNR falling to ≈ 1 dB; tens of thousands of `out-of-sync` warnings. The eNB of that cell: **no `RACH:` line at all**.
- `make 4g-device` never worked: it started one UE, but the broker waited for a second UE that was not running (replaced by `4g-devices` in the handover work).

---

## 4. The investigation, in order

All runs from a fully destroyed stack (containers, networks, volumes; images kept), on an 8-CPU, 16 GB host. "3 runs" means 3 separate bring-ups.

### 4.1 Baseline: 1 of 15

| Path | Run 1 | Run 2 | Run 3 |
|---|---|---|---|
| `4g-multi` | ue1 ✗, ue3 ✗ | ue1 ✗ | ue1 ✗, ue3 ✗ |
| `test-4g-multi` | ue3 ✗ | ✓ (make ✗) | ✓ (make ✗) |
| `4g-auto` | ue3 ✗ | ue3 ✗ | ✓ |
| `staged` | ue3 ✗ | ue3 ✗ | ue1 100 % loss, ue3 ✗ |
| `4g-device` | ue1 ✗ | ue1 ✗ | ue1 ✗ |

Host load 16–23 on 8 CPUs. The srsRAN logs of the attach phase had already rotated away.

### 4.2 Experiments with one change each

| | Change | Result |
|---|---|---|
| A | none, but wait 3 min instead of 40 s | Everyone attaches eventually: ue1/ue2 ≈ 25 s, ue3 ≈ 145 s |
| B | PHY logging to `warning` | Load ≈ 22 → ≈ 16; ue3 fast in 2 of 3 runs; logs now usable. **Kept** |
| C | real-time priority + locked memory for srsRAN (`SYS_NICE`, `memlock`) | Warnings gone, no measurable effect. Dropped |
| D | FFT benchmark cache per endpoint | PHY start 175 s → 7 s; ue1/ue2 attach 25 s → 5 s; ue3 still ≈ 130 s. **Kept** (in a cleaner form) |
| E | UE frequency-offset (CFO) tracking off | ue3 unchanged. Dropped |

How D was found: per-thread CPU sampling showed one `PHY` thread at 100 % of one core for ≈ 90 s while the host was nearly idle. That pointed at FFTW planning; a standalone srsue took 175 s without the cache and 7 s with it.

How E came about: ue3's log showed its frequency-offset estimate drifting from 0 to +38 kHz. With tracking off the estimate stayed put, but ue3 failed the same way: the drift was a **symptom** of a broken downlink, not the cause.

### 4.3 The shared air

The two per-cell brokers were replaced by one flowgraph (`deploy/4g/broker/air.py`, `broker.py`, `config/multi/air.json`). ue3 still failed in the same way, so the per-cell broker was never the cause. Cost: the air itself uses ≈ 3 CPU cores; total host load went from ≈ 17 to ≈ 19.5.

### 4.4 Narrowing down, and a wrong turn

With the air, moving ue3 to cell A is a one-line gain change. On cell A ue3 attached normally, so ue3 itself was fine. Changing cell B's PCI from 2 to 1 also made it work, which looked like "PCI 2 is the problem". A sweep then gave PCI 1 ✓, 2 ✗, 3 ✗, 4 ✓, 5 ✓: no pattern. With ue1 failing in the baseline too, the conclusion had to be: **random per run**, and one run per setting proves nothing. (Lesson: with a random failure, compare series, not single runs.)

### 4.5 Measuring where the samples land

A test variant of the air recorded the energy of every uplink and downlink in 0.1 ms bins:

- Cell A's frame started at bin 0 of the air's sample count; **cell B's at bin 4: 0.4 ms off**, not a whole number of subframes.
- ue1's preambles all landed at the same frame position (attached); ue2's first three one subframe off, then right (attached when they moved); **ue3's all ten at a position cell B never looks at**.

So an eNB *can* end up shifted against the sample stream, despite the lockstep.

### 4.6 What the documentation and other users say

- The srsRAN 4G ZMQ app note: *"We currently only support a single eNB and a single UE"*; the broker must start last and be restarted with the network. The multi-UE GNU Radio tutorial calls itself *"not intended to be an optimized, performant, or scalable solution"* and slows its flowgraph down on purpose.
- The handover app note configures cells with different `root_seq_idx` and correct neighbour EARFCNs (relevant for the handover work, not for this bug).
- Someone else saw a 22 min "Waiting PHY to initialize" in Docker, unanswered (our problem 2).
- No report of problem 1 found.

### 4.7 The cause

Reading srsRAN's ZMQ receive code (`lib/src/phy/rf/rf_zmq_imp.c`, `rf_zmq_recv_with_time_multi`) explained the 0.4 ms:

1. The eNBs had `fail_on_disconnect=true` in `device_args` (copied from the official app note).
2. If a receive gets no samples for 2 s, that option makes the function bail out **before** it advances its clock (`next_rx_ts`). Samples of the current subframe that had already arrived are thrown away.
3. srsenb ignores the error and carries on, now with its receive clock behind the stream by that partial amount, and its transmit stream padded to the same wrong clock.
4. UEs follow the shifted downlink; their preambles land outside the eNB's window: problem 1.

UEs do not use that option, so on a timeout they just keep waiting and lose nothing.

### 4.8 Proof

- A first check with timeout logging showed **zero** timeouts while ue3 failed, which seemed to refute the idea, until it turned out the logging option itself was broken in srsRAN (problem 8). Patched.
- **Without `fail_on_disconnect`:** 3 of 3 runs, all UEs attached (on the shared air, ue3 had failed in about half of the runs before). With the logging patch, 3 more runs: all UEs attached, and **each eNB logged one or two 2 s timeouts per run**, both eNBs at the same moment, ≈ 10 s after the air started, during the attach. Those stalls happen every run; with the old option each could shift an eNB, now they only cost time.

---

## 5. What the stack does now

- **eNB `device_args`** (`deploy/4g/config/multi/enb-*.conf`): no `fail_on_disconnect`, `log_trx_timeout=true`, `base_srate=11.52e6`; `n_prb = 25`.
- **UE `device_args`**: `base_srate=11.52e6` (the UE reads the bandwidth from the cell).
- **FFT cache**: each eNB/UE runs with `HOME=/fftw` on its own folder under `deploy/4g/.fftw/` (git-ignored; survives `make 4g-auto-down`). The first boot on a host benchmarks once (several minutes, with progress shown); later boots take seconds. Delete the folder to re-benchmark, e.g. after moving to another machine. The files are owned by root (written from the containers): `sudo rm -rf deploy/4g/.fftw`.
- **Start order** (`make 4g-multi`, `4g-devices`, `test-4g-multi`, `test-4g-handover`): core, eNBs and UEs start; `wait-radio-ready.sh` waits until both eNBs log `eNodeB started` and every UE logs `Attaching UE...`, which srsue prints right after its PHY is initialised (limit `RADIO_READY_TIMEOUT`, default 600 s); only then does the air start.
- **Attach waits** (`wait-for-attach-svc.sh`): up to `ATTACH_TIMEOUT` (default 180 s) per UE, with progress every 15 s.
- **UE shutdown**: the entrypoint passes `SIGTERM` on to srsue (clean detach, cache saved).
- **srsRAN image**: built with the T3402 patch and the `log_trx_timeout` patch; only the programs the stack runs are compiled.

Expect the first `make 4g-auto` on a fresh host to take several minutes longer than later ones.

### Results on the final stack

3 runs per path, each from a fully destroyed stack, FFT cache warm. "UEs" = every UE had an IP and `ping -c 10 8.8.8.8` through `tun_srsue` lost nothing, checked after `make` returned.

| Path | UEs attached + ping | `make` exit 0 | Attach after air start | Run time (destroyed → pinging) |
|---|---|---|---|---|
| `4g-multi` | 3/3 | 3/3 | 2–5 s | 107–111 s |
| `4g-auto` | 3/3 | 3/3 | ≤ 1 s* | 120–125 s |
| `test-4g-multi` | 3/3 | 2/3 | ≤ 1 s* | 99–110 s |
| staged (`4g-infra` → `4g-demo-subscribers` → `4g-demo-devices`, now `4g-devices`) | 3/3 | 3/3 | ≤ 1 s* | 111–116 s |
| `4g-single` (regression check) | ✓ | ✓ | – | 130 s |

\* these targets wait for attach themselves, so by the time of the check the UEs were already up.

**12 of 12** multi-cell runs had all three UEs attached and pinging (baseline: 1 of 15). The first boot with an empty FFT cache took several minutes in the radio-ready step, as expected; all five endpoints then saved their cache on teardown, including the UEs (which never did before the entrypoint fix).

**Re-run after the handover work** (world model, travel API, handover config, `4g-devices`), same method: again **12 of 12**, every `make` exit 0, all pings answered. `4g-multi` 98–105 s, `4g-auto` 114–121 s, `test-4g-multi` 95–103 s, staged (now ending in `4g-devices`) 136–139 s; host load 15.6–18.3. `make test-4g-handover` passed 3 of 3 as well (see [`parts/3-handover.md`](../parts/3-handover.md) §7).

The one `make` failure: in `test-4g-multi` run 2, ue2 lost one of the 3 pings that `test-multi.sh` sends right after attach (the 10-ping check straight after had 0 % loss). The ping output of that run was overwritten before it could be inspected; `test-multi.sh` now prints it on failure. Open point, see §6.

Host load (1-minute average at the end of each run): 13.7–18.1, mean 16.6. Not directly comparable with the earlier 16–23 (the runs are now much shorter, so the average is still rising when read), so this does not prove what 25 PRB saves.

Along the way, `test-multi.sh` turned out to accept 100 % packet loss (its check `grep "0% packet loss"` also matches `100% packet loss`); fixed.

---

## 6. Still open

- **A lost ping right after attach** (once in 12 runs, `test-4g-multi`): not yet understood. The next occurrence will print the ping output.
- **What 25 PRB saves**, measured properly (average load over a fixed window, 25 vs 50 PRB).

- **What holds the lockstep up for 2 s+ once per run.** Harmless now, but it is where time goes. Likely one endpoint briefly starved of CPU; the timeout log line pins the moment.
- **Host load.** The radio is CPU-bound; 25 PRB halves it. More cells or UEs grow the air's work with cells × UEs.
- **No timestamps on the wire** remains the fundamental limit: a UE that restarts cannot rejoin a running air (see the hot-join step on the roadmap), and any future sample loss would be fatal again. The 5G phase starts with a decision about the radio stack for this reason.

---

## 7. Try it yourself (exercises)

1. **Logs before and after.** Set `all_level = info` without the `phy_level`/`phy_lib_level` lines in a UE config, bring up `make 4g-multi`, and watch `docker stats` and `uptime`. Compare with the current config.
2. **The FFT cache.** `sudo rm -rf deploy/4g/.fftw`, then `make 4g-multi` and time the `wait-radio-ready.sh` step. Tear down, bring up again, time it again. `docker compose -f deploy/4g/docker-compose.yml -f deploy/4g/docker-compose.multi.yml logs -t ue1 | grep "Waiting PHY"` shows the moment PHY init finished.
3. **Does the eNB see the UE?** `... logs ue3 | grep -E "Found Cell|Random Access|Attach"` and `... logs enb-b | grep RACH`. A UE sending preambles that the eNB never logs is the signature of a shifted timeline.
4. **The timeouts.** `... logs enb-a enb-b | grep "timeout receiving"`: the 2 s stalls, now visible thanks to the patch.
5. **Bring the bug back.** Add `fail_on_disconnect=true,` back to the start of `device_args` in both `enb-*.conf` files and run `make 4g-multi` a few times. Count how often a UE never attaches, and check the eNB's `RACH` lines for that UE.
6. **Move a UE between cells.** With the stack up, `make 4g-travel UE=ue3 TO=1` drives ue3 to cell A (a handover; see [`parts/3-handover.md`](../parts/3-handover.md)). For an attach on the other cell instead, change ue3's start position in `deploy/4g/config/multi/air.json` to `"x": 1`, bring the stack up, and check which eNB logs ue3's RACH.
