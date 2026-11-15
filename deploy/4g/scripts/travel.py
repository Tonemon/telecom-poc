#!/usr/bin/env python3
"""Move UEs around the multi-cell world: `make 4g-travel ...` (see help()).

Talks to the air's API (positions, travel, shielding) and the provisioner (which cell serves each
UE, from the MME's live state). Standard library only.
"""
import json
import math
import os
import re
import sys
import time
import urllib.error
import urllib.request

HERE = os.path.dirname(os.path.abspath(__file__))
CONF = os.path.join(HERE, "..", "config", "multi")
AIR = os.environ.get("AIR_URL", "http://127.0.0.1:8090")
AIR_TOKEN = os.environ.get("AIR_TOKEN", "dev-air-token")
PROV = os.environ.get("TELCOCTL_SERVER", "http://127.0.0.1:8080")
PROV_TOKEN = os.environ.get("TELCOCTL_TOKEN", "dev-operator-token")
DEFAULT_SPEED = 1000.0  # km/h: tested; crosses the handover zone slowly enough in radio time
SAFE_MAX_SPEED = 1500.0

HELP = """\
make 4g-travel: move the multi-cell UEs around the world (needs a running stack: make 4g-multi)

  make 4g-travel                             this help + where everyone is now
  make 4g-travel UE=ue2 TO=8                 travel to x = 8 km (y = 0) at {d:g} km/h
  make 4g-travel UE=ue2 TO=8,3 SPEED=600     travel to (8, 3) at 600 km/h
  make 4g-travel UE=ue2 AT=5                 jump there right away (cancels a travel)
  make 4g-travel UE=ue3 SHIELD=on            Faraday cage: no radio at all (SHIELD=off: back)
  make 4g-travel WATCH=1                     live view, refreshes every 2 s until Ctrl-C

The world is in km: cell A at (0, 0), cell B at (10, 0). Both cells are heard between
x = 3 and 7 km; a UE travelling across hands over around x = 5.9 (A -> B) or 4.1 (B -> A).
Beyond 7 km from both cells (e.g. TO=30) there is no coverage at all.

Speed is in radio time: the virtual radio runs slower than the clock (the view shows how
much), so a trip takes longer in real time than its km/h suggest. Above {m:g} km/h a UE can
cross the handover zone faster than a handover takes, and lose its connection instead.
""".format(d=DEFAULT_SPEED, m=SAFE_MAX_SPEED)


class Fail(Exception):
    pass


def call(base, token, method, path, body=None):
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(base + path, data=data, method=method)
    req.add_header("Authorization", f"Bearer {token}")
    if data is not None:
        req.add_header("Content-Type", "application/json")
    try:
        with urllib.request.urlopen(req, timeout=5) as r:
            return json.loads(r.read() or b"null")
    except urllib.error.HTTPError as e:
        try:
            msg = json.loads(e.read()).get("error", "")
        except ValueError:
            msg = ""
        raise Fail(f"{e.code}: {msg or e.reason}")
    except (urllib.error.URLError, OSError) as e:
        raise Fail(f"cannot reach {base} ({getattr(e, 'reason', e)}): is the stack up? (make 4g-multi)")


def conf_value(path, key):
    with open(path) as f:
        m = re.search(rf"^\s*{key}\s*=\s*(\S+)", f.read(), re.M)
    return m.group(1).rstrip(";") if m else None


def ue_imsis():
    out = {}
    for name in sorted(os.listdir(CONF)):
        m = re.fullmatch(r"(ue\d+)\.conf", name)
        if m:
            out[m.group(1)] = conf_value(os.path.join(CONF, name), "imsi")
    return out


def enb_names():
    out = {}
    for name in sorted(os.listdir(CONF)):
        m = re.fullmatch(r"enb-(\w+)\.conf", name)
        if m:
            enb = conf_value(os.path.join(CONF, name), "enb_id")
            if enb:
                out[int(enb, 0)] = f"cell {m.group(1).upper()} (enb-{m.group(1)})"
    return out


def serving(imsi, subs, enbs):
    s = subs.get(imsi)
    net = (s or {}).get("network")
    if not net or net.get("mm_state") != "registered":
        return "no service"
    cell = enbs.get(net.get("enb_id"), f"enb {net.get('enb_id')}")
    return cell + ("" if net.get("cm_state") == "connected" else " (idle)")


def fmt_xy(x, y):
    return f"({x:.1f}, {y:.1f})"


def strip(world):
    """A 0..10 km road (wider if a UE is further out) with cells (A, B) and UEs (1, 2, 3)."""
    xs = [c["x"] for c in world["cells"]] + [u["x"] for u in world["ues"]]
    lo, hi = min(0.0, math.floor(min(xs))), max(10.0, math.ceil(max(xs)))
    width = 60
    pos = lambda x: round((x - lo) / (hi - lo) * width)
    line = ["-"] * (width + 1)
    for c in world["cells"]:
        line[pos(c["x"])] = c["id"]
    marks = [" "] * (width + 1)
    for u in world["ues"]:
        marks[pos(u["x"])] = u["id"][-1]
    scale = f"{lo:g} km".ljust(width - 4) + f"{hi:g} km"
    return f"  {''.join(marks)}\n  {''.join(line)}\n  {scale}"


def status():
    world = call(AIR, AIR_TOKEN, "GET", "/world")
    try:
        subs = {s["imsi"]: s for s in call(PROV, PROV_TOKEN, "GET", "/subscribers")}
    except Fail:
        subs = None
    imsis, enbs = ue_imsis(), enb_names()
    radio = world.get("radio") or {}
    speed = radio.get("speed") or 0
    print(strip(world))
    print()
    print(f"  {'UE':<4} {'position':<12} {'travel':<36} {'gain A/B':<11} serving")
    for u in world["ues"]:
        t = u["travel"]
        if u["shielded"]:
            move = "shielded (Faraday cage)"
        elif t:
            real = f", ~{t['eta_s'] / speed / 60:.1f} min real" if speed else ""
            move = f"-> {fmt_xy(t['x'], t['y'])} {t['speed_kmh']:g} km/h{real}"
        else:
            move = "-"
        g = u["gains"]
        gains = "/".join(f"{g[c]:.2f}" for c in sorted(g))
        serve = serving(imsis.get(u["id"]), subs, enbs) if subs is not None else "? (provisioner unreachable)"
        if not any(g.values()):
            # No radio at all: the network only notices after its timers run out, so what the
            # MME reports is out of date until then.
            serve = "no radio" + ("" if serve == "no service" else f" (MME still says {serve})")
        print(f"  {u['id']:<4} {fmt_xy(u['x'], u['y']):<12} {move:<36} {gains:<11} {serve}")
    if speed:
        print(f"\n  radio runs at {speed:.2f}x real time ({radio['time_s']:.0f} radio-s so far)")


def parse_xy(text, name):
    parts = text.split(",")
    try:
        nums = [float(p) for p in parts]
    except ValueError:
        raise Fail(f"{name}={text}: give x or x,y in km, e.g. {name}=8 or {name}=8,3")
    if len(nums) == 1:
        nums.append(0.0)
    if len(nums) != 2 or not all(math.isfinite(n) for n in nums):
        raise Fail(f"{name}={text}: give x or x,y in km, e.g. {name}=8 or {name}=8,3")
    return nums


def main(argv):
    args = {}
    for a in argv:
        if "=" not in a:
            raise Fail(f"unexpected argument {a!r}; use KEY=VALUE (see: make 4g-travel)")
        k, v = a.split("=", 1)
        if v != "":
            args[k.upper()] = v
    unknown = set(args) - {"UE", "TO", "AT", "SPEED", "SHIELD", "WATCH"}
    if unknown:
        raise Fail(f"unknown option(s): {', '.join(sorted(unknown))} (see: make 4g-travel)")

    if args.get("WATCH"):
        try:
            while True:
                print("\033[2J\033[H", end="")
                print(time.strftime("%H:%M:%S"), "(Ctrl-C to stop)\n")
                status()
                time.sleep(2)
        except KeyboardInterrupt:
            return 0

    if not args:
        print(HELP)
        try:
            status()
        except Fail as e:
            print(f"  (no live view: {e})")
        return 0

    ue = args.get("UE")
    actions = [k for k in ("TO", "AT", "SHIELD") if k in args]
    if not ue or len(actions) != 1:
        raise Fail("give UE=<ue1|ue2|ue3> and exactly one of TO=, AT= or SHIELD= (see: make 4g-travel)")
    action = actions[0]
    if "SPEED" in args and action != "TO":
        raise Fail("SPEED= only goes with TO=")

    if action == "TO":
        x, y = parse_xy(args["TO"], "TO")
        try:
            speed = float(args.get("SPEED", DEFAULT_SPEED))
        except ValueError:
            raise Fail(f"SPEED={args['SPEED']}: give km/h as a number, e.g. SPEED=600")
        if not math.isfinite(speed) or speed <= 0:
            raise Fail("SPEED must be more than 0 km/h")
        if speed > SAFE_MAX_SPEED:
            print(f"warning: above {SAFE_MAX_SPEED:g} km/h {ue} may cross the handover zone too fast "
                  "and lose its connection instead of handing over")
        call(AIR, AIR_TOKEN, "POST", f"/ues/{ue}/travel", {"x": x, "y": y, "speed_kmh": speed})
        print(f"{ue} is travelling to {fmt_xy(x, y)} at {speed:g} km/h. Follow it: make 4g-travel WATCH=1\n")
    elif action == "AT":
        x, y = parse_xy(args["AT"], "AT")
        call(AIR, AIR_TOKEN, "PUT", f"/ues/{ue}/position", {"x": x, "y": y})
        print(f"{ue} is now at {fmt_xy(x, y)}.\n")
    else:
        val = args["SHIELD"].lower()
        if val not in ("on", "off"):
            raise Fail("SHIELD=on or SHIELD=off")
        call(AIR, AIR_TOKEN, "PUT", f"/ues/{ue}/shielded", {"shielded": val == "on"})
        print(f"{ue} is {'shielded: no radio at all' if val == 'on' else 'out of the Faraday cage'}.\n")
    status()
    return 0


if __name__ == "__main__":
    try:
        sys.exit(main(sys.argv[1:]))
    except Fail as e:
        print(f"4g-travel: {e}", file=sys.stderr)
        sys.exit(2)
