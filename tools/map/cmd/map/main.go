// Command map serves the world map page and proxies its API calls to the instances in
// instances.json (internal/proxy), keeping their tokens server-side.
package main

import (
	"log"
	"net/http"
	"os"

	"github.com/telecom-poc/map/internal/config"
	"github.com/telecom-poc/map/internal/proxy"
	"github.com/telecom-poc/map/web"
)

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	instances, err := config.Load(env("MAP_INSTANCES", "/conf/instances.json"))
	if err != nil {
		log.Fatalf("map: %v", err)
	}
	mux := http.NewServeMux()
	mux.Handle("/api/", proxy.New(instances))
	mux.Handle("/", http.FileServerFS(web.Files))
	addr := env("MAP_LISTEN_ADDR", ":8088")
	log.Printf("map: %d instance(s); listening on %s", len(instances), addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
