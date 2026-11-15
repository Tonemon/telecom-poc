"""Small HTTP control API for the air (stdlib only).

  GET /healthz                 no auth, {"ok": true}
  GET /world                   cells, UEs and their current gains
  PUT /ues/{id}/gains          {"A": 0.1, "B": 1.0}: set that UE's link gain to each listed cell

Every route except /healthz needs `Authorization: Bearer <token>`. A gains request is validated
completely before anything is applied, so a bad value never leaves a half-changed UE.
"""
import hmac
import json
import math
import re
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

GAINS_PATH = re.compile(r"^/ues/([^/]+)/gains$")


class ApiError(Exception):
    def __init__(self, status, message):
        super().__init__(message)
        self.status = status
        self.message = message


def _parse_gains(raw, cell_ids):
    try:
        body = json.loads(raw, parse_constant=lambda c: (_ for _ in ()).throw(ValueError(c)))
    except ValueError:
        raise ApiError(400, "body must be a JSON object like {\"A\": 0.5, \"B\": 1.0}")
    if not isinstance(body, dict) or not body:
        raise ApiError(400, "body must be a non-empty JSON object of cell id -> gain")
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
                m = GAINS_PATH.match(self.path)
                if method == "PUT" and m:
                    ue = m.group(1)
                    if ue not in {u["id"] for u in snap["ues"]}:
                        raise ApiError(404, f"unknown UE: {ue}")
                    length = int(self.headers.get("Content-Length") or 0)
                    gains = _parse_gains(self.rfile.read(length), {c["id"] for c in snap["cells"]})
                    for cell, value in gains:
                        set_gain(ue, cell, value)
                    return self._send(200, {"ue": ue, "gains": dict(gains)})
                raise ApiError(404, f"no route: {method} {self.path}")
            except ApiError as e:
                self._send(e.status, {"error": e.message})

        def do_GET(self):
            self._handle("GET")

        def do_PUT(self):
            self._handle("PUT")

    return ThreadingHTTPServer(("0.0.0.0", port), Handler)
