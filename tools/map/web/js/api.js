// Calls to the map server, which passes them on to the instance's air, agent or provisioner.

let instance = null;

export async function instances() {
  const r = await fetch("/api/instances");
  return r.json();
}

export function use(name) {
  instance = name;
}

function url(service, path) {
  return `/api/instances/${encodeURIComponent(instance)}/${service}${path}`;
}

export async function get(service, path) {
  const r = await fetch(url(service, path));
  if (!r.ok) throw new Error(`${service}${path}: ${r.status}`);
  return r.json();
}

// send returns {status, json, command}: command is the terminal equivalent from the JSON
// "command" field (air, agent) or the X-Command header (provisioner).
export async function send(service, method, path, body) {
  // X-Map: the map server only accepts changes that carry it (another website cannot add it).
  const init = { method, headers: { "X-Map": "1" } };
  if (body !== undefined) {
    init.body = JSON.stringify(body);
    init.headers["Content-Type"] = "application/json";
  }
  const r = await fetch(url(service, path), init);
  let json = null;
  try { json = await r.json(); } catch { /* no body */ }
  const command = (json && json.command) || r.headers.get("X-Command") || "";
  return { status: r.status, ok: r.ok, json, command };
}

export function href(service, path) {
  return url(service, path);
}
