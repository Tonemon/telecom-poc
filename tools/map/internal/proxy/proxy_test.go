package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/telecom-poc/map/internal/config"
)

type seen struct {
	method, path, query, auth, body string
}

func upstream(t *testing.T, got *seen, status int, reply string) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		*got = seen{r.Method, r.URL.Path, r.URL.RawQuery, r.Header.Get("Authorization"), string(b)}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		io.WriteString(w, reply)
	}))
	t.Cleanup(s.Close)
	return s
}

func handler(t *testing.T, got *seen, status int, reply string) http.Handler {
	t.Helper()
	up := upstream(t, got, status, reply)
	return New([]config.Instance{{Name: "telecom-poc",
		Air:         config.Endpoint{URL: up.URL, Token: "dev-air-token"},
		Agent:       config.Endpoint{URL: up.URL, Token: "dev-agent-token"},
		Provisioner: config.Endpoint{URL: up.URL, Token: "dev-operator-token"}}})
}

func do(h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
	return rec
}

func TestProxyAddsTokenAndForwards(t *testing.T) {
	var got seen
	h := handler(t, &got, 200, `{"cells": []}`)
	rec := do(h, "GET", "/api/instances/telecom-poc/air/world", "")
	if rec.Code != 200 || rec.Body.String() != `{"cells": []}` {
		t.Fatalf("got %d %q", rec.Code, rec.Body.String())
	}
	if got.path != "/world" || got.auth != "Bearer dev-air-token" {
		t.Fatalf("upstream saw %+v", got)
	}
	do(h, "POST", "/api/instances/telecom-poc/agent/ues/ue2/speedtest", "")
	if got.path != "/ues/ue2/speedtest" || got.auth != "Bearer dev-agent-token" {
		t.Fatalf("upstream saw %+v", got)
	}
}

func TestProxyForwardsBodyAndStatus(t *testing.T) {
	var got seen
	h := handler(t, &got, 409, `{"error": "busy"}`)
	rec := do(h, "POST", "/api/instances/telecom-poc/agent/pcaps", `{"component":"enb-a"}`)
	if rec.Code != 409 || got.body != `{"component":"enb-a"}` {
		t.Fatalf("got %d, upstream %+v", rec.Code, got)
	}
}

func TestProxyRejectsUnlisted(t *testing.T) {
	var got seen
	h := handler(t, &got, 200, `{}`)
	for _, c := range []struct{ method, path string }{
		{"DELETE", "/api/instances/telecom-poc/provisioner/subscribers/999700000000003"},
		{"GET", "/api/instances/telecom-poc/air/../agent/inventory"},
		{"GET", "/api/instances/telecom-poc/air/ues%2fue1%2fgains"},
		{"PUT", "/api/instances/telecom-poc/air/ues/ue1/gains"},
		{"GET", "/api/instances/telecom-poc/mongo/x"},
		{"POST", "/api/instances/telecom-poc/air/world"},
	} {
		got = seen{}
		rec := do(h, c.method, c.path, "")
		// 403, or for a ".." path ServeMux's redirect to the cleaned path (which is checked again).
		if rec.Code != 403 && rec.Code != 307 || got.method != "" {
			t.Errorf("%s %s: %d (upstream saw %+v), want 403/307 and no upstream call", c.method, c.path, rec.Code, got)
		}
	}
}

func TestUnknownInstance404(t *testing.T) {
	var got seen
	if rec := do(handler(t, &got, 200, `{}`), "GET", "/api/instances/nope/air/world", ""); rec.Code != 404 {
		t.Fatalf("got %d", rec.Code)
	}
}

func TestInstancesHidesTokens(t *testing.T) {
	var got seen
	rec := do(handler(t, &got, 200, `{}`), "GET", "/api/instances", "")
	if rec.Code != 200 || rec.Body.String() != `[{"name":"telecom-poc"}]`+"\n" {
		t.Fatalf("got %d %q", rec.Code, rec.Body.String())
	}
}

func TestDryRunQueryAllowed(t *testing.T) {
	var got seen
	h := handler(t, &got, 200, `{}`)
	do(h, "PUT", "/api/instances/telecom-poc/air/cells/B/position?dry_run=1&evil=2", `{"x":1,"y":0}`)
	if got.path != "/cells/B/position" || got.query != "dry_run=1" {
		t.Fatalf("upstream saw %+v", got)
	}
	do(h, "GET", "/api/instances/telecom-poc/agent/ues/ue2/ping?since=17", "")
	if got.query != "since=17" {
		t.Fatalf("upstream saw %+v", got)
	}
	do(h, "GET", "/api/instances/telecom-poc/air/world?since=17", "")
	if got.query != "" {
		t.Fatalf("query passed to a route that takes none: %+v", got)
	}
}

func TestProvisionerCommandHeader(t *testing.T) {
	var got seen
	h := handler(t, &got, 200, `{}`)
	rec := do(h, "POST", "/api/instances/telecom-poc/provisioner/subscribers/999700000000003/suspend",
		`{"reason":"FRAUD","note":"x"}`)
	if want := `./bin/telcoctl suspend 999700000000003 --reason FRAUD --note "x"`; rec.Header().Get("X-Command") != want {
		t.Fatalf("X-Command %q, want %q", rec.Header().Get("X-Command"), want)
	}
	if got.body != `{"reason":"FRAUD","note":"x"}` || got.auth != "Bearer dev-operator-token" {
		t.Fatalf("upstream saw %+v", got)
	}
}

func TestTelcoctlCommand(t *testing.T) {
	cases := []struct{ method, path, body, want string }{
		{"POST", "/subscribers/999700000000003/suspend", `{"reason":"FRAUD","note":"x"}`,
			`./bin/telcoctl suspend 999700000000003 --reason FRAUD --note "x"`},
		{"POST", "/subscribers/999700000000003/resume", `{"reason":"CLEARED"}`,
			`./bin/telcoctl resume 999700000000003 --reason CLEARED`},
		{"PATCH", "/subscribers/999700000000003/plan", `{"dl":"256K","ul":"128K","reason":"DOWNGRADE"}`,
			`./bin/telcoctl set-plan 999700000000003 --dl 256K --ul 128K --reason DOWNGRADE`},
		{"GET", "/subscribers", ``, ``},
	}
	for _, c := range cases {
		if got := TelcoctlCommand(c.method, c.path, []byte(c.body)); got != c.want {
			t.Errorf("%s %s: %q, want %q", c.method, c.path, got, c.want)
		}
	}
}
