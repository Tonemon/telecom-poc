"""Small HTTP control API for the air (stdlib only).

  GET  /healthz               no auth, {"ok": true}
  GET  /world                 cells, UEs (position, travel, shielded, gains) and the radio clock
  POST /ues/{id}/travel       {"x": 8, "y": 0, "speed_kmh": 300}: travel there (202)
  PUT  /ues/{id}/position     {"x": 8, "y": 0}: be there now (cancels travel)
  PUT  /ues/{id}/shielded     {"shielded": true}: the Faraday cage, no radio at all
  PUT  /ues/{id}/gains        {"A": 0.1, "B": 1.0}: debug override of that UE's link gains,
                              until its next move/position/shielding change

Every route except /healthz needs `Authorization: Bearer <token>`. A gains request is validated
completely before anything is applied, so a bad value never leaves a half-changed UE.
"""
import hmac
import json
import math
import re
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

UE_PATH = re.compile(r"^/ues/([^/]+)/(gains|travel|position|shielded)$")


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
                if method == "GET" and self.path == "/world":
                    return self._send(200, snap)
                m = UE_PATH.match(self.path)
                route = (method, m.group(2)) if m else None
                if route not in {("PUT", "gains"), ("POST", "travel"), ("PUT", "position"),
                                 ("PUT", "shielded")}:
                    raise ApiError(404, f"no route: {method} {self.path}")
                ue = m.group(1)
                ues = {u["id"]: u for u in snap["ues"]}
                if ue not in ues:
                    raise ApiError(404, f"unknown UE: {ue}")
                raw = self.rfile.read(int(self.headers.get("Content-Length") or 0))
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
                    return self._send(202, self._ue(ue))
                if kind == "position":
                    body = _json_object(raw, '{"x": 8, "y": 0}')
                    world.place(ue, _number(body, "x"), _number(body, "y"))
                    return self._send(200, self._ue(ue))
                body = _json_object(raw, '{"shielded": true}')
                if not isinstance(body.get("shielded"), bool):
                    raise ApiError(400, "shielded must be true or false")
                world.set_shielded(ue, body["shielded"])
                return self._send(200, self._ue(ue))
            except ApiError as e:
                self._send(e.status, {"error": e.message})

        def _ue(self, ue):
            return next(u for u in world.snapshot()["ues"] if u["id"] == ue)

        def do_GET(self):
            self._handle("GET")

        def do_PUT(self):
            self._handle("PUT")

        def do_POST(self):
            self._handle("POST")

    return ThreadingHTTPServer(("0.0.0.0", port), Handler)
