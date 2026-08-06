package main

import (
	"context"
	"crypto/rand"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/telecom-poc/provisioner/internal/api"
	"github.com/telecom-poc/provisioner/internal/network"
	"github.com/telecom-poc/provisioner/internal/store"
)

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func main() {
	token := os.Getenv("PROVISIONER_API_TOKEN")
	if token == "" {
		log.Fatal("PROVISIONER_API_TOKEN is required")
	}
	uri := env("PROVISIONER_MONGODB_URI", "mongodb://172.22.0.2/open5gs")
	addr := env("PROVISIONER_LISTEN_ADDR", ":8080")
	actor := env("PROVISIONER_ACTOR", "operator")
	mmeURL := env("PROVISIONER_MME_METRICS_URL", "http://172.22.0.5:9090")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	st, err := store.NewMongoStore(ctx, uri)
	if err != nil {
		log.Fatalf("mongo: %v", err)
	}

	keygen := func() []byte {
		b := make([]byte, 16)
		if _, err := rand.Read(b); err != nil {
			panic(err) // crypto/rand failure is unrecoverable
		}
		return b
	}
	mme := network.NewMMEClient(mmeURL)
	srv := api.NewServer(st, token, actor, keygen, mme)
	log.Printf("provisioner listening on %s (mongo %s, mme %s)", addr, uri, mmeURL)
	log.Fatal(http.ListenAndServe(addr, srv.Handler()))
}
