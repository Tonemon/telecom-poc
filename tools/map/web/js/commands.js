// Terminal commands: every action has an icon that shows how to do the same by hand. After the
// action ran, the command the server returned is shown (authoritative); before that, a preview
// built from the same formats as the air, the agent and the map server.

import { page } from "./main.js";
import { deviceOf, model } from "./state.js";

export const COMPOSE = "docker compose -f deploy/4g/docker-compose.yml -f deploy/4g/docker-compose.multi.yml";
const esc = s => String(s).replace(/[&<>"']/g, ch => `&#${ch.charCodeAt(0)};`);
const g = v => String(+(+v).toFixed(3));
const xy = (x, y) => `{"x": ${g(x)}, "y": ${g(y)}}`;
const curl = (method, path, data) =>
  `curl -X ${method} -H "Authorization: Bearer $AIR_TOKEN"${data ? ` -d '${data}'` : ""} http://127.0.0.1:8090${path}`;

function preview(key) {
  const [kind, id, arg] = key.split(":");
  const ue = model.world?.ues.find(u => u.id === id);
  const cell = model.world?.cells.find(c => c.id === id);
  const imsi = deviceOf(id)?.imsi || "<imsi>";
  switch (kind) {
    case "travel": return `make 4g-travel UE=${id} TO=<x>,<y> SPEED=1000`;
    case "jump": return `make 4g-travel UE=${id} AT=<x>,<y>`;
    case "shield": return `make 4g-travel UE=${id} SHIELD=${arg || "on"}`;
    case "stop": return curl("POST", `/ues/${id}/stop`);
    case "cell": return curl("PUT", `/cells/${id}/position`, xy(cell?.x ?? 0, cell?.y ?? 0));
    case "random": return curl("POST", "/activity/random");
    case "realtime": return curl("PUT", "/activity/realtime", `{"on": true}`);
    case "pcap": {
      const tower = model.inventory?.cells.find(c => c.service === id);
      return tower
        ? `${COMPOSE} exec ${id} tcpdump -p -Z root -i eth0 -U -w /pcap/${id}-<time>.pcap 'sctp or udp port 2152'    # Ctrl-C to stop`
        : `${COMPOSE} exec ${id} tcpdump -p -Z root -i tun_srsue -U -w /pcap/${id}-<time>.pcap    # Ctrl-C to stop`;
    }
    case "ping": return `${COMPOSE} exec ${id} ping -I tun_srsue -i 0.2 8.8.8.8`;
    case "speedtest": return `${COMPOSE} exec upf iperf3 -s -D && ${COMPOSE} exec ${id} iperf3 -c 10.45.0.1 -R -t 5`;
    case "suspend": return `./bin/telcoctl suspend ${imsi} --reason <REASON>`;
    case "resume": return `./bin/telcoctl resume ${imsi} --reason <REASON>`;
    case "plan": return `./bin/telcoctl set-plan ${imsi} --dl <dl> --ul <ul> --reason <REASON>`;
    case "power": { // key power:<action>:<service>; id is the action here, arg the service
      const svc = arg, ctr = `telecom-poc-4g-${svc}-1`, net = "telecom-poc-4g_epc";
      return {
        switch_off: `${COMPOSE} stop ${svc}`, shut_down: `${COMPOSE} stop ${svc}`,
        pull_battery: `${COMPOSE} kill ${svc}`, switch_on: `${COMPOSE} start ${svc}`,
        power_cut: `docker network disconnect -f ${net} ${ctr} && ${COMPOSE} kill ${svc}`,
        power_on: `docker network connect --ip <its address> --alias ${svc} ${net} ${ctr} && ${COMPOSE} start ${svc}    # connect only after a power cut`,
      }[id] || "";
    }
  }
  return ue ? "" : "";
}

export function cmdButton(key) {
  return `<button class="cmd" data-cmd="${esc(key)}" title="Show the terminal command">&gt;_</button>`;
}

export function remember(key, command) {
  if (command) page.lastCommand[key] = command;
}

function show(key) {
  const ran = page.lastCommand[key];
  const text = ran || preview(key);
  document.getElementById("bottom").innerHTML = `<div class="panel cmdpanel">
    <div>${ran ? "The command that does the same (from your repo root):" : "<b>Preview</b>: the command for this action (from your repo root); after you run the action here, this shows the exact one:"}</div>
    <pre>${esc(text)}</pre>
    <div><button data-copy>Copy</button> <button data-close>Close</button></div></div>`;
  document.querySelector("[data-copy]").addEventListener("click", () => navigator.clipboard?.writeText(text));
}

export function init() {
  document.addEventListener("click", ev => {
    const b = ev.target.closest("[data-cmd]");
    if (b) show(b.dataset.cmd);
  });
}
