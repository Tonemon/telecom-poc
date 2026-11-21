// Command agent exposes a fixed set of actions on one telecom-poc instance over HTTP:
// inventory, health, pcaps, live ping, speedtest, power (phones and towers off and on) and the
// core's unreachable phones (see internal/api).
package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/telecom-poc/agent/internal/actions"
	"github.com/telecom-poc/agent/internal/api"
	"github.com/telecom-poc/agent/internal/docker"
	"github.com/telecom-poc/agent/internal/inventory"
)

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	inv, err := inventory.Load(env("AGENT_CONF_DIR", "/conf"))
	if err != nil {
		log.Fatalf("agent: inventory: %v", err)
	}
	addr := env("AGENT_LISTEN_ADDR", ":8091")
	log.Printf("agent: %d cells, %d devices; listening on %s", len(inv.Cells), len(inv.Devices), addr)
	acts := actions.New(docker.NewSocket(env("AGENT_DOCKER_SOCKET", "/var/run/docker.sock")), inv,
		env("AGENT_PROJECT", "telecom-poc-4g"), env("AGENT_PCAP_DIR", "/pcap"), env("AGENT_IPERF_SERVER", "")).
		WithMME(env("AGENT_MME_METRICS_URL", "http://172.22.0.5:9090"))
	airJSON, err := os.ReadFile(env("AGENT_AIR_CONFIG", filepath.Join(env("AGENT_CONF_DIR", "/conf"), "air.json")))
	if err != nil {
		log.Fatalf("agent: air config: %v", err)
	}
	towers, err := actions.TowerAddrs(airJSON)
	if err != nil {
		log.Fatalf("agent: air config: %v", err)
	}
	acts.WithPower(env("AGENT_NETWORK", "telecom-poc-4g_epc"), towers)
	log.Fatal(http.ListenAndServe(addr, api.New(env("AGENT_TOKEN", "dev-agent-token"), inv, acts)))
}
