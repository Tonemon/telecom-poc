# Hot-join: phones and towers off and on

*Switch a phone or a tower off and on while the rest of the network keeps running, and watch what the network notices, and when.*

This builds on the shared air from [`2-multi-ue-broker.md`](2-multi-ue-broker.md) and the world map from [`4-world-map.md`](4-world-map.md). Before hot-join, one restarted phone or tower meant restarting the whole radio side (`make 4g-devices`); now a single phone or tower can leave and come back, and a crashed one rejoins by itself.


## 1. The buttons

On the map, select a phone or a tower; its card has a **Power** block. Every button has a terminal icon (`>_`) with the command that does the same.

| Button | What it does to the device | What the network sees |
|---|---|---|
| Phone: **Switch off** | Stops the phone's container. srsue shuts down like a phone does: it sends a NAS *Detach Request* marked "switch off" and waits up to 5 s for it to go out. | The MME gets the detach over the radio, through the tower, and deregisters the phone at once. |
| Phone: **Pull battery** | Kills the container. Nothing is sent. | The tower notices the radio link is gone and releases the phone (UE Context Release). The MME still has the phone registered (idle). After its *mobile reachable* timer it marks it unreachable, and after the *implicit detach* timer it removes it. |
| Phone: **Switch on** | Starts the container. | A cold start: cell search, random access, authentication, a new IP. With a capture running, that is the textbook attach. |
| Tower: **Shut down** | Stops the eNB. srsenb closes its S1 link (SCTP) to the core cleanly. It sends no S1 message: S1 has no "eNB leaving" procedure, real eNBs don't either. | The MME drops the eNB at once. Its phones lose the signal and look for another cell. |
| Tower: **Power cut** | Cuts the eNB's network first, then kills it, so nothing at all goes out. (Killing alone would not be realistic: Linux would close the SCTP link politely on the dead process's behalf.) | Silence. The MME notices only when its SCTP heartbeats go unanswered, about half a minute later. |
| Tower: **Power on** | Puts the eNB back on the network with its fixed address, then starts it. | S1 Setup, the cell broadcasts again, phones in range find it. |

The map never tells the core anything. Everything the core learns, it learns the way it would in real life: from the phone over the radio, from the tower over S1, or by noticing on its own. What the cards show (on or off, the S1 link, "registered", "unreachable") is read from the containers and the core's own metrics, the way an operator's dashboard does.

From a terminal, the same through the agent (what the buttons call):

```bash
curl -X POST -H "Authorization: Bearer $AGENT_TOKEN" -d '{"action": "pull_battery"}' http://127.0.0.1:8091/power/ue2
curl -H "Authorization: Bearer $AGENT_TOKEN" http://127.0.0.1:8091/core/unreachable
```

Phones: `switch_off`, `pull_battery`, `switch_on`. Towers: `shut_down`, `power_cut`, `power_on`. `AGENT_TOKEN` defaults to `dev-agent-token`.


## 2. The timers behind "pull battery"

A real idle phone reports in every **T3412** with a *periodic tracking area update* (TAU). That is how the core knows a silent phone is still there. If the reports stop, the MME counts down:

1. **Mobile reachable timer** (T3412 + 4 minutes): when it runs out, the phone is *unreachable*.
2. **Implicit detach timer** (another T3412 + 4 minutes): when that runs out, the core detaches the phone without hearing from it.

The default T3412 is 9 minutes, so a pulled battery would take 26 minutes to be detached. This network hands out T3412 = 30 s (`deploy/4g/config/mme.yaml`), so it takes about 9 minutes after the tower lets the phone go: short enough for a class. The tower itself needs a few minutes to notice, because its radio timers run in radio time, which is slower than real time.

Stock srsue ignores T3412 and never sends a periodic TAU at all, so an idle phone would be detached even while switched on. A build-time patch (`deploy/patches/srsran-nas-periodic-tau.patch`) adds it. The patch's T3412 counts wall time like the core's timers, not radio time like srsue's other timers: it is a contract between the phone and the core.


## 3. How the air makes room

The air is the only clock: every phone and tower waits on it for each slice of radio, so a radio's own sample count is exactly how much it has exchanged with the air since its process started. When a new process asks for its first slice, the air notes where in radio time that is, and from then on it knows where each of that radio's samples belongs.

- **Leaving** is noticed only when the connection closes (or, after a power cut, when ZMQ's own link checks go unanswered), never because a radio went quiet: while the air waits for one radio, all the others are quiet too.
- **A booting phone never holds the others up**: until it transmits, the air does not wait for it.
- **When it first transmits**, the air waits for it to catch up with the downlink it still has waiting. While booting, a phone reads slower than the air sends; stock srsRAN keeps up to 0.27 s of that old radio and works through all of it first, which slowed the other phones' pings to 1.5 s. A build-time patch (`deploy/patches/srsran-zmq-rx-max-backlog.patch`, `rx_max_backlog_ms=20` in `deploy/4g/config/multi/*.conf`) makes a radio that fell behind drop its oldest samples instead, like a real radio's overflow, so only a short wait remains: the others' pings rise to about half a second once, and nothing is lost.

`AIR_DEBUG=1 make 4g-multi` logs the air's view of every radio every 2 s. The code is in `deploy/4g/broker/engine.py` (the logic, unit-tested without sockets) and `zmqair.py` (the sockets).


## 4. Try it yourself (exercises)

All on a running `make 4g-multi` with `make map`.

1. **The textbook attach, on demand.** Start a capture on cell A, switch ue1 off, then on. Stop the capture and open it in Wireshark (filter `s1ap`): the Detach Request (switch off), then the full attach as the core sees it: Initial UE Message with the Attach Request, Authentication, Security Mode, Attach Accept with an IP. (A tower capture sees S1; the radio's RRC messages never leave the tower.)
2. **Switch off vs pull battery.** Switch ue3 off and watch its card: the core deregisters it at once. Switch it on, then pull its battery: the card says *phone off · core still has it registered*. Wait for "unreachable", then for the core to drop it. How long did each step take, and which timer was it?
3. **Periodic TAU in a pcap.** Capture on cell A while ue1 is idle (no ping running) for two minutes. Find the Tracking Area Update Requests: how often do they come, and why does the network release the connection right after each one?
4. **Maintenance vs outage.** Shut cell B down and look at the tower card: S1 goes down at once. Power it on, then cut its power: the card still says S1 up. How long until the core notices? Where does ue3 go meanwhile, and why can't it hand over to cell A?
5. **A crash.** `docker compose -f deploy/4g/docker-compose.yml -f deploy/4g/docker-compose.multi.yml restart ue1`: it rejoins by itself, and ue2 on the same tower keeps working.


## 5. Check it

```bash
make test-hot-join   # every button, on a running make 4g-multi (~30 minutes)
```

It switches ue3 off and on, restarts ue1, pulls ue2's battery until the core marks it unreachable, shuts enb-b down and powers it on, cuts its power and powers it on, and checks the periodic TAU, while the phones not under test keep pinging without losing a packet.
