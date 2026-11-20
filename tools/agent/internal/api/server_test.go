package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/telecom-poc/agent/internal/actions"
	"github.com/telecom-poc/agent/internal/inventory"
)

func get(t *testing.T, h http.Handler, path, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("GET", path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestHealthzNoAuth(t *testing.T) {
	h := New("secret", inventory.Inventory{}, &stubActions{})
	if rec := get(t, h, "/healthz", ""); rec.Code != 200 {
		t.Fatalf("healthz = %d", rec.Code)
	}
}

func TestInventoryNeedsToken(t *testing.T) {
	h := New("secret", inventory.Inventory{Name: "telecom-poc"}, &stubActions{})
	if rec := get(t, h, "/inventory", ""); rec.Code != 401 {
		t.Fatalf("no token = %d, want 401", rec.Code)
	}
	if rec := get(t, h, "/inventory", "wrong"); rec.Code != 401 {
		t.Fatalf("wrong token = %d, want 401", rec.Code)
	}
	rec := get(t, h, "/inventory", "secret")
	if rec.Code != 200 {
		t.Fatalf("good token = %d", rec.Code)
	}
	var inv inventory.Inventory
	if err := json.Unmarshal(rec.Body.Bytes(), &inv); err != nil || inv.Name != "telecom-poc" {
		t.Fatalf("body %q: %v", rec.Body.String(), err)
	}
}

func TestUnknownRoute404JSON(t *testing.T) {
	rec := get(t, New("secret", inventory.Inventory{}, &stubActions{}), "/nope", "secret")
	if rec.Code != 404 || rec.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("got %d %q", rec.Code, rec.Header().Get("Content-Type"))
	}
}

type stubActions struct {
	file     string
	startErr error
	pingOn   map[string]bool
}

func (s *stubActions) Components(context.Context) ([]actions.Component, error) {
	return []actions.Component{{Service: "ue2", State: "exited"}}, nil
}
func (s *stubActions) StartPcap(_ context.Context, c string) (actions.PcapInfo, error) {
	return actions.PcapInfo{Component: c, File: c + "-x.pcap", Command: "docker compose ... tcpdump"}, s.startErr
}
func (s *stubActions) StopPcap(_ context.Context, c string) (actions.PcapInfo, error) {
	return actions.PcapInfo{}, fmt.Errorf("%s: %w", c, actions.ErrNotCapturing)
}
func (s *stubActions) Capturing() []actions.PcapInfo { return nil }
func (s *stubActions) Pcaps() ([]actions.PcapFile, error) {
	return []actions.PcapFile{{Name: "enb-a-x.pcap", Bytes: 3}}, nil
}
func (s *stubActions) PcapPath(name string) (string, error) {
	if name != "enb-a-x.pcap" {
		return "", actions.ErrUnknown
	}
	return s.file, nil
}
func (s *stubActions) SetPing(_ context.Context, ue string, on bool) error {
	if s.pingOn == nil {
		s.pingOn = map[string]bool{}
	}
	s.pingOn[ue] = on
	return nil
}
func (s *stubActions) PingSince(ue string, since int) actions.PingState {
	return actions.PingState{On: s.pingOn[ue], Next: since + 1}
}
func (s *stubActions) Speedtest(context.Context, string) (actions.Speed, error) {
	return actions.Speed{}, fmt.Errorf("ue2 is exited: %w", actions.ErrNotRunning)
}

func (s *stubActions) Power(_ context.Context, service, action string) (actions.PowerResult, error) {
	switch {
	case action == "switch_off":
		return actions.PowerResult{Service: service, Action: action, State: "off", Command: "docker compose ... stop " + service}, nil
	case action == "switch_on":
		return actions.PowerResult{}, fmt.Errorf("%s is already on: %w", service, actions.ErrState)
	}
	return actions.PowerResult{}, fmt.Errorf("%s cannot %s: %w", service, action, actions.ErrUnknown)
}
func (s *stubActions) Unreachable(context.Context) ([]string, error) {
	return []string{"999700000000002"}, nil
}

func call(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestActionRoutes(t *testing.T) {
	st := &stubActions{}
	h := New("secret", inventory.Inventory{}, st)
	cases := []struct {
		method, path, body string
		code               int
		contains           string
	}{
		{"GET", "/components", "", 200, `"state":"exited"`},
		{"POST", "/pcaps", `{"component":"enb-a"}`, 200, `"command":"docker compose ... tcpdump"`},
		{"POST", "/pcaps", `{"nope":1}`, 400, "component"},
		{"DELETE", "/pcaps/enb-a", "", 404, "not capturing"},
		{"GET", "/pcaps", "", 200, `"enb-a-x.pcap"`},
		{"GET", "/pcaps/..%2fetc.pcap", "", 404, "error"},
		{"PUT", "/ues/ue2/ping", `{"on":true}`, 200, `"on":true`},
		{"PUT", "/ues/ue2/ping", `{"on":"yes"}`, 400, "on must be"},
		{"GET", "/ues/ue2/ping?since=4", "", 200, `"next":5`},
		{"GET", "/ues/ue2/ping?since=x", "", 400, "since"},
		{"POST", "/ues/ue2/speedtest", "", 409, "not running"},
		{"POST", "/power/ue2", `{"action":"switch_off"}`, 200, `"command":"docker compose ... stop ue2"`},
		{"POST", "/power/ue2", `{"action":"switch_on"}`, 409, "already on"},
		{"POST", "/power/ue2", `{"action":"explode"}`, 404, "cannot explode"},
		{"POST", "/power/ue2", `{}`, 400, "action"},
		{"GET", "/core/unreachable", "", 200, `{"imsis":["999700000000002"]}`},
	}
	for _, c := range cases {
		rec := call(t, h, c.method, c.path, c.body)
		if rec.Code != c.code || !strings.Contains(rec.Body.String(), c.contains) {
			t.Errorf("%s %s: %d %s; want %d containing %q", c.method, c.path, rec.Code, rec.Body.String(), c.code, c.contains)
		}
	}
}

func TestStartPcapBusy409(t *testing.T) {
	h := New("secret", inventory.Inventory{}, &stubActions{startErr: fmt.Errorf("x: %w", actions.ErrBusy)})
	if rec := call(t, h, "POST", "/pcaps", `{"component":"enb-a"}`); rec.Code != 409 {
		t.Fatalf("got %d", rec.Code)
	}
}

func TestPcapDownload(t *testing.T) {
	f := filepath.Join(t.TempDir(), "enb-a-x.pcap")
	os.WriteFile(f, []byte("abc"), 0o644)
	h := New("secret", inventory.Inventory{}, &stubActions{file: f})
	rec := call(t, h, "GET", "/pcaps/enb-a-x.pcap", "")
	if rec.Code != 200 || rec.Body.String() != "abc" || rec.Header().Get("Content-Type") != "application/vnd.tcpdump.pcap" {
		t.Fatalf("got %d %q %q", rec.Code, rec.Body.String(), rec.Header().Get("Content-Type"))
	}
}
