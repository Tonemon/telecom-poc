// Package api is the agent's HTTP API: a fixed set of actions behind a bearer token. There is
// no generic exec or Docker passthrough.
package api

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/telecom-poc/agent/internal/actions"
	"github.com/telecom-poc/agent/internal/inventory"
)

// Actions is what the routes need from internal/actions.
type Actions interface {
	Components(ctx context.Context) ([]actions.Component, error)
	StartPcap(ctx context.Context, component string) (actions.PcapInfo, error)
	StopPcap(ctx context.Context, component string) (actions.PcapInfo, error)
	Capturing() []actions.PcapInfo
	Pcaps() ([]actions.PcapFile, error)
	PcapPath(name string) (string, error)
	SetPing(ctx context.Context, ue string, on bool) error
	PingSince(ue string, since int) actions.PingState
	Speedtest(ctx context.Context, ue string) (actions.Speed, error)
}

type server struct {
	token string
	inv   inventory.Inventory
	acts  Actions
}

// New returns the agent's handler. Every route except /healthz needs the bearer token.
func New(token string, inv inventory.Inventory, acts Actions) http.Handler {
	s := &server{token: token, inv: inv, acts: acts}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, 200, map[string]bool{"ok": true}) })
	mux.HandleFunc("GET /inventory", s.auth(func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, 200, s.inv) }))
	mux.HandleFunc("GET /components", s.auth(s.components))
	mux.HandleFunc("POST /pcaps", s.auth(s.startPcap))
	mux.HandleFunc("DELETE /pcaps/{component}", s.auth(s.stopPcap))
	mux.HandleFunc("GET /pcaps", s.auth(s.listPcaps))
	mux.HandleFunc("GET /pcaps/{file}", s.auth(s.downloadPcap))
	mux.HandleFunc("PUT /ues/{id}/ping", s.auth(s.setPing))
	mux.HandleFunc("GET /ues/{id}/ping", s.auth(s.getPing))
	mux.HandleFunc("POST /ues/{id}/speedtest", s.auth(s.speedtest))
	mux.HandleFunc("/", s.auth(func(w http.ResponseWriter, r *http.Request) {
		writeError(w, 404, "no route: "+r.Method+" "+r.URL.Path)
	}))
	return mux
}

func (s *server) components(w http.ResponseWriter, r *http.Request) {
	comps, err := s.acts.Components(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, comps)
}

func (s *server) startPcap(w http.ResponseWriter, r *http.Request) {
	var body struct{ Component string }
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Component == "" {
		writeError(w, 400, `want {"component": "enb-a"}`)
		return
	}
	info, err := s.acts.StartPcap(r.Context(), body.Component)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, info)
}

func (s *server) stopPcap(w http.ResponseWriter, r *http.Request) {
	info, err := s.acts.StopPcap(r.Context(), r.PathValue("component"))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, info)
}

func (s *server) listPcaps(w http.ResponseWriter, _ *http.Request) {
	files, err := s.acts.Pcaps()
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"files": files, "capturing": s.acts.Capturing()})
}

func (s *server) downloadPcap(w http.ResponseWriter, r *http.Request) {
	path, err := s.acts.PcapPath(r.PathValue("file"))
	if err != nil {
		fail(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/vnd.tcpdump.pcap")
	w.Header().Set("Content-Disposition", `attachment; filename="`+r.PathValue("file")+`"`)
	http.ServeFile(w, r, path)
}

func (s *server) setPing(w http.ResponseWriter, r *http.Request) {
	var body struct{ On *bool }
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.On == nil {
		writeError(w, 400, `on must be true or false: {"on": true}`)
		return
	}
	ue := r.PathValue("id")
	if err := s.acts.SetPing(r.Context(), ue, *body.On); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, s.acts.PingSince(ue, 1<<62))
}

func (s *server) getPing(w http.ResponseWriter, r *http.Request) {
	since := 0
	if v := r.URL.Query().Get("since"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			writeError(w, 400, "since must be a number >= 0")
			return
		}
		since = n
	}
	writeJSON(w, 200, s.acts.PingSince(r.PathValue("id"), since))
}

func (s *server) speedtest(w http.ResponseWriter, r *http.Request) {
	sp, err := s.acts.Speedtest(r.Context(), r.PathValue("id"))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, sp)
}

// fail maps action errors to HTTP statuses.
func fail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, actions.ErrUnknown), errors.Is(err, actions.ErrNotCapturing):
		writeError(w, 404, err.Error())
	case errors.Is(err, actions.ErrNotRunning), errors.Is(err, actions.ErrBusy):
		writeError(w, 409, err.Error())
	default:
		writeError(w, 502, err.Error())
	}
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
