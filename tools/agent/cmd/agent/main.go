// Command agent exposes a fixed set of actions on one telecom-poc instance over HTTP:
// inventory, health, pcaps, live ping and speedtest (see internal/api).
package main

import (
	"log"
	"net/http"
	"os"

	"github.com/telecom-poc/agent/internal/api"
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
	log.Fatal(http.ListenAndServe(addr, api.New(env("AGENT_TOKEN", "dev-agent-token"), inv)))
}
