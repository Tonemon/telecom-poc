// Package api is the agent's HTTP API: a fixed set of actions behind a bearer token.
package api

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"

	"github.com/telecom-poc/agent/internal/inventory"
)

type server struct {
	token string
	inv   inventory.Inventory
}

// New returns the agent's handler. Every route except /healthz needs the bearer token.
func New(token string, inv inventory.Inventory) http.Handler {
	s := &server{token: token, inv: inv}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, 200, map[string]bool{"ok": true}) })
	mux.HandleFunc("GET /inventory", s.auth(func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, 200, s.inv) }))
	mux.HandleFunc("/", s.auth(func(w http.ResponseWriter, r *http.Request) {
		writeError(w, 404, "no route: "+r.Method+" "+r.URL.Path)
	}))
	return mux
}

func (s *server) auth(h http.HandlerFunc) http.HandlerFunc {
	want := []byte("Bearer " + s.token)
	return func(w http.ResponseWriter, r *http.Request) {
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), want) != 1 {
			writeError(w, 401, "missing or wrong bearer token")
			return
		}
		h(w, r)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
