"""Small HTTP control API for the air (stdlib only).

  GET  /healthz               no auth, {"ok": true}
  GET  /world                 cells, UEs (position, travel, shielded, gains) and the radio clock
  POST /ues/{id}/travel       {"x": 8, "y": 0, "speed_kmh": 300}: travel there (202)
  PUT  /ues/{id}/position     {"x": 8, "y": 0}: be there now (cancels travel)
  PUT  /ues/{id}/shielded     {"shielded": true}: the Faraday cage, no radio at all
  PUT  /ues/{id}/gains        {"A": 0.1, "B": 1.0}: debug override of that UE's link gains,
                              until its next move/position/shielding change
  POST /ues/{id}/stop         cancel a travel where the UE is now
  PUT  /cells/{id}/position   {"x": 6, "y": 0}: move a cell; ?dry_run=1 only reports, per UE,
                              the gains before/after and whether it gains or loses coverage
  POST /activity/random       one decision per UE (stay, or a random trip); 409 while realtime
  PUT  /activity/realtime     {"on": true}: keep deciding after each arrival; off stops every
                              automated UE where it is

Action responses carry "command": the same action as a terminal command.

Every route except /healthz needs `Authorization: Bearer <token>`. A gains request is validated
completely before anything is applied, so a bad value never leaves a half-changed UE.
"""
import hmac
import json
import math
import re
import urllib.parse
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

from control import Busy

UE_PATH = re.compile(r"^/ues/([^/]+)/(gains|travel|position|shielded|stop)$")
CELL_PATH = re.compile(r"^/cells/([^/]+)/position$")
CURL = 'curl -X {method} -H "Authorization: Bearer $AIR_TOKEN"{data} http://127.0.0.1:8090{path}'


def curl_command(method, path, body=None):
    data = f" -d '{json.dumps(body)}'" if body is not None else ""
    return CURL.format(method=method, data=data, path=path)


def _g(v):
    return f"{v:g}"


def _num_json(v):
    """9.0 -> 9 so a command reads like what a person would type."""
    return int(v) if float(v).is_integer() else v


class ApiError(Exception):
    def __init__(self, status, message):
        super().__init__(message)
        self.status = status
        self.message = message


def _json_object(raw, example):
    try:
        body = json.loads(raw, parse_constant=lambda c: (_ for _ in ()).throw(ValueError(c)))
    except ValueError:
        raise ApiError(400, f"body must be a JSON object like {example}")
    if not isinstance(body, dict) or not body:
        raise ApiError(400, f"body must be a non-empty JSON object like {example}")
    return body


def _number(body, key):
    if key not in body:
        raise ApiError(400, f"missing field: {key}")
    v = body[key]
    if isinstance(v, bool) or not isinstance(v, (int, float)) or not math.isfinite(v):
        raise ApiError(400, f"{key} must be a number")
    return float(v)


def _parse_gains(raw, cell_ids):
    body = _json_object(raw, '{"A": 0.5, "B": 1.0}')
    gains = []
    for cell, value in body.items():
        if cell not in cell_ids:
            raise ApiError(404, f"unknown cell: {cell}")
        if isinstance(value, bool) or not isinstance(value, (int, float)) or not math.isfinite(value):
            raise ApiError(400, f"gain for {cell} must be a number")
        if value < 0:
            raise ApiError(400, f"gain for {cell} must not be negative")
        gains.append((cell, float(value)))
    return gains


def make_server(port, token, world, set_gain):
    """world: any object with snapshot() -> {"cells": [{"id", ...}], "ues": [{"id", ...}]}.
    set_gain(ue, cell, value) applies one link gain."""

    class Handler(BaseHTTPRequestHandler):
        def log_message(self, fmt, *args):  # keep the air's log readable
            pass

        def _send(self, status, body):
            data = json.dumps(body).encode()
            self.send_response(status)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(data)))
            self.end_headers()
            self.wfile.write(data)

        def _authorized(self):
            got = self.headers.get("Authorization", "")
            return hmac.compare_digest(got, f"Bearer {token}")

        def _handle(self, method):
            try:
                if method == "GET" and self.path == "/healthz":
                    return self._send(200, {"ok": True})
                if not self._authorized():
                    raise ApiError(401, "missing or wrong bearer token")
                snap = world.snapshot()
                url = urllib.parse.urlsplit(self.path)
                if method == "GET" and url.path == "/world":
                    return self._send(200, snap)
                raw = self.rfile.read(int(self.headers.get("Content-Length") or 0))
                if url.path == "/activity/random" and method == "POST":
                    try:
                        plans = world.random_activity()
                    except Busy as e:
                        raise ApiError(409, str(e))
                    return self._send(200, {"plans": plans,
                                            "command": curl_command("POST", "/activity/random")})
                if url.path == "/activity/realtime" and method == "PUT":
                    body = _json_object(raw, '{"on": true}')
                    if not isinstance(body.get("on"), bool):
                        raise ApiError(400, "on must be true or false")
                    stopped = world.set_realtime(body["on"])
                    return self._send(200, {"realtime": body["on"], "stopped": stopped,
                                            "command": curl_command("PUT", "/activity/realtime",
                                                                    {"on": body["on"]})})
                m = CELL_PATH.match(url.path)
                if m and method == "PUT":
                    return self._cell(m.group(1), url.query, raw, snap)
                m = UE_PATH.match(url.path)
                route = (method, m.group(2)) if m else None
                if route not in {("PUT", "gains"), ("POST", "travel"), ("PUT", "position"),
                                 ("PUT", "shielded"), ("POST", "stop")}:
                    raise ApiError(404, f"no route: {method} {self.path}")
                ue = m.group(1)
                ues = {u["id"]: u for u in snap["ues"]}
                if ue not in ues:
                    raise ApiError(404, f"unknown UE: {ue}")
                kind = m.group(2)
                if kind == "gains":
                    gains = _parse_gains(raw, {c["id"] for c in snap["cells"]})
                    for cell, value in gains:
                        set_gain(ue, cell, value)
                    return self._send(200, {"ue": ue, "gains": dict(gains)})
                if kind == "travel":
                    body = _json_object(raw, '{"x": 8, "y": 0, "speed_kmh": 300}')
                    x, y, speed = _number(body, "x"), _number(body, "y"), _number(body, "speed_kmh")
                    if speed <= 0:
                        raise ApiError(400, "speed_kmh must be greater than 0")
                    if (x, y) == (ues[ue]["x"], ues[ue]["y"]):
                        raise ApiError(400, f"{ue} is already at ({x:g}, {y:g})")
                    world.travel(ue, x, y, speed)
                    return self._send(202, self._ue(
                        ue, f"make 4g-travel UE={ue} TO={_g(x)},{_g(y)} SPEED={_g(speed)}"))
                if kind == "position":
                    body = _json_object(raw, '{"x": 8, "y": 0}')
                    x, y = _number(body, "x"), _number(body, "y")
                    world.place(ue, x, y)
                    return self._send(200, self._ue(ue, f"make 4g-travel UE={ue} AT={_g(x)},{_g(y)}"))
                if kind == "stop":
                    world.stop(ue)
                    return self._send(200, self._ue(ue, curl_command("POST", f"/ues/{ue}/stop")))
                body = _json_object(raw, '{"shielded": true}')
                if not isinstance(body.get("shielded"), bool):
                    raise ApiError(400, "shielded must be true or false")
                world.set_shielded(ue, body["shielded"])
                return self._send(200, self._ue(
                    ue, f"make 4g-travel UE={ue} SHIELD={'on' if body['shielded'] else 'off'}"))
            except ApiError as e:
                self._send(e.status, {"error": e.message})

        def _cell(self, cell, query, raw, snap):
            if cell not in {c["id"] for c in snap["cells"]}:
                raise ApiError(404, f"unknown cell: {cell}")
            body = _json_object(raw, '{"x": 6, "y": 0}')
            x, y = _number(body, "x"), _number(body, "y")
            if urllib.parse.parse_qs(query).get("dry_run") == ["1"]:
                return self._send(200, {"id": cell, "x": x, "y": y, "ues": world.dry_run_cell(cell, x, y)})
            world.place_cell(cell, x, y)
            path = f"/cells/{cell}/position"
            return self._send(200, {"id": cell, "x": x, "y": y,
                                    "command": curl_command("PUT", path, {"x": _num_json(x), "y": _num_json(y)})})

        def _ue(self, ue, command=None):
            u = next(u for u in world.snapshot()["ues"] if u["id"] == ue)
            return {**u, "command": command} if command else u

        def do_GET(self):
            self._handle("GET")

        def do_PUT(self):
            self._handle("PUT")

        def do_POST(self):
            self._handle("POST")

    return ThreadingHTTPServer(("0.0.0.0", port), Handler)
