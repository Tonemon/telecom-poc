# The world map

*A web page that shows everything running in the 4G network on a map, and lets you drive it: move phones and towers, start packet captures, suspend a subscriber, run a speedtest.*

This builds on the shared air and the handover from [`2-multi-ue-broker.md`](2-multi-ue-broker.md) and [`3-handover.md`](3-handover.md). Everything the map does, you can also do from a terminal: every action has a terminal icon (`>_`) that shows the exact command.


## 1. Start it

```bash
make 4g-multi        # the multi-cell network (2 towers, 3 phones)
make map             # the map, next to it
```

Open `http://127.0.0.1:8088`. `make map-down` stops the map; the network keeps running. `make 4g-auto-down` stops everything.

The map is a "remote control with a window". The air stays in charge of the radio world (positions, travel, signal strength), the core of who is attached where. The map only shows and asks, so closing the page changes nothing in the network, and a reload starts with an empty history.


## 2. How it fits together

```
browser ──> map server (127.0.0.1:8088): serves the page, passes calls on with the tokens
              ├──> air          (8090)  positions, travel, towers, activity
              ├──> agent        (8091)  health, inventory, pcaps, ping, speedtest
              └──> provisioner  (8080)  subscribers, serving cell, suspend / resume / plan
```

| Part | Where | What it does |
|---|---|---|
| Air | `deploy/4g/broker/` | The radio world. New for the map: move a tower (with a dry-run), stop a phone, random and realtime activity |
| Agent | `tools/agent/` | One per network. Reads the configs (`deploy/4g/config/multi/instance.json` adds operator name and phone models), asks the MME which towers have an S1 link, and runs a fixed set of actions in the containers through the Docker socket |
| Map server | `tools/map/` | Serves the page (plain HTML and JavaScript, built into the binary, nothing from the internet) and passes on only a fixed list of calls, adding the tokens. The browser never sees a token |

The map server's list of networks is `deploy/map/instances.json`. There is one network today; the list is there for more networks later (roaming).


## 3. Reading the map

| You see | It means |
|---|---|
| Shaded areas | Each tower's coverage, in its colour, stronger towards the tower. Unshaded: no signal at all |
| Dotted lines | Handover lines: where one tower beats the other by the handover margin (3 dB). Between the two lines is the hysteresis zone, where a phone stays with the tower it has |
| Filled dot | A phone with a radio connection right now (connected), in its serving tower's colour |
| Hollow dot | A phone that is attached but idle (no radio connection; the first packet wakes it up) |
| Grey dot | No service. With "down": the phone's container is not running |
| Orange `!` | The radio is gone (Faraday cage, dead zone) but the core still thinks the phone is connected. It catches up when its timers run out |
| Solid / dashed line | From a phone to its serving tower / to another tower it also hears; brighter = stronger |
| Flag | A handover happened here: "A→B", with the number of pings lost if the live ping was on |

The grid is in 1 km squares, rows A–T and columns 1–30 (like Battleship); the box in the corner shows the square and the km under the mouse.

Click a tower or a phone for its card. Terms with a dotted underline (PCI, EARFCN, A3, AMBR, ...) explain themselves on hover.

The cards show what the operator's core knows, the way a real network operations centre sees it: a tower's "Link to core" is its S1 link as the MME reports it (S1 Setup done, and how many connected phones the MME counts on it), and a phone's IP is the address the SMF gave its session. A tower whose container runs but that has lost the core shows "S1 down".


## 4. Moving things

- **Send a phone somewhere**: "Send somewhere…" on its card, click a destination, pick a speed. Speeds are time-lapse: the virtual radio runs at about a tenth of real time, so the slider stops at 1500 km/h, the fastest a handover can keep up with. It shows how long the trip takes in real time.
- **Jump**: "Jump to…" puts a phone somewhere right away.
- **Move a tower**: drag it. A ghost shows the new coverage; on drop, a confirmation lists which phones would gain or lose coverage. Towers cannot be added or removed yet (that needs "hot-join").
- **Random activity**: every phone either stays (about one in three) or drives to a random spot at a random speed, mostly within coverage and sometimes not (a tunnel).
- **Realtime activity**: the same, but it keeps going: a phone arrives, waits a little and picks again. Switch it off and every phone it was moving stops where it is. A phone you send by hand leaves the automation until it arrives.
- **Presets and map files**: "Presets…" has a highway, a tunnel and a crowded cell. "Download map" saves the terrain and all positions, "Upload map" puts them back. "New terrain" draws a different (fictional) landscape; it never changes the radio.

Positions reset to `air.json` when the network restarts (`make 4g-multi`); save a map file to keep a layout.


## 5. Looking deeper

- **Packet captures**: "Start capture" / "Stop capture" on a tower records its S1 traffic (signalling to the core, and the GTP-U tunnels with the phones' data); on a phone, what goes in and out of `tun_srsue`. Download the file from the card and open it in Wireshark.
- **Live ping**: a strip of the last minute of pings (every 0.2 s) on the phone's card. Off by default, because a phone that pings all the time never goes idle.
- **Speedtest**: `iperf3` from the phone to the network's gateway (the UPF, `10.45.0.1`), down and up. It measures the radio and the core, not the internet. Expect about 1 Mbit/s: the radio is slow, not the cell.
- **Operator**: suspend, resume and change the plan of a phone's subscriber, the same as `telcoctl` (see the scenarios).
- **Signal chart**: each tower's signal over the last five minutes, the band the other tower must beat, and a marker at every handover.


## 6. The acceptance test

```bash
make test-map
```

The page's own logic (map-file checks, pin labels) has unit tests that need no network, only Docker: `make test-map-js` runs them in a Node container.

`make test-map`, from a stopped network, brings everything up and, only through the map server (the same calls the page makes), checks: every container runs; the inventory; a tower move (dry-run and real); captures on a tower and a phone; a speedtest; suspend and resume of a subscriber (the phone loses service and comes back by itself); that calls outside the allowed list are refused; and random and realtime activity.


## 7. Known limits

- One network on the map; several networks (roaming) come later.
- No history: a reload starts with no pins and no charts.
- Towers can be moved, not added or removed, and a phone's container cannot rejoin the radio after a restart (hot-join is the next step on the roadmap).


## 8. Try it yourself (exercises)

1. **Find the hysteresis zone.** Turn on the live ping for `ue2`, send it to cell B and back a few times. Where do the flags land going A→B, and where going B→A? How wide is the gap, and why is it there? (`3-handover.md` §4.)
2. **Push a tower away.** Drag cell B away from `ue3` until the confirmation says `ue3` will lose coverage, and confirm. How long does the orange `!` stay before the core notices `ue3` is gone?
3. **Capture a handover.** Start a capture on cell A, send `ue2` to cell B, stop the capture after the flag appears, and open the file in Wireshark. Filter on `s1ap` and find `HandoverRequired`, `HandoverCommand` and `UEContextReleaseCommand`. Which of them does cell A send, and which does it receive?
4. **Near and far.** Run a speedtest on `ue1` (1 km from cell A), then jump it to 6 km from cell A, inside cell B's reach, and run it again. Does the weaker signal, with cell B as interference on the same frequency, cost speed? (The gateway's `iperf3` server runs one test at a time, so run them one after the other.)
5. **The tunnel.** Apply the Tunnel preset and send `ue2` from cell A to cell B. What happens in the gap, and what is different from a handover (IP address, flag, ping strip)?
6. **Make your own preset.** Arrange the towers and phones, download the map, change a position in the file by hand, and upload it again.
7. **Rush hour.** Turn on Realtime activity with the live ping on every phone and leave it for ten minutes. Count the flags, and look for phones that dropped out instead of handing over: which speeds and routes do that?
