package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// /metrics must be reachable WITHOUT the operator bearer token: it is not
// operator API, and the port is published host-locally only (spec §4.3).
func TestMetricsEndpointNeedsNoToken(t *testing.T) {
	srv := NewServer(nil, "secret-token", "operator", nil, nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /metrics = %d, want 200 (must not require auth)", rec.Code)
	}
	// Asserts on provisioner_subscribers, a plain Gauge, rather than the
	// operations CounterVec: client_golang emits nothing at all -- not even
	// HELP/TYPE -- for a CounterVec or HistogramVec with no observed label
	// combination, and this test deliberately performs no operation before
	// scraping. A Gauge is always present, so this still proves what it needs
	// to: /metrics is reachable without a bearer token and returns provisioner
	// series. Do not swap this back to provisioner_operations_total.
	if !strings.Contains(rec.Body.String(), "provisioner_subscribers") {
		t.Fatalf("metrics body missing provisioner series:\n%s", rec.Body.String())
	}
}

// The operator API must still be protected — proving /metrics was carved out
// deliberately rather than by disabling auth.
func TestSubscriberRoutesStillRequireToken(t *testing.T) {
	srv := NewServer(nil, "secret-token", "operator", nil, nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/subscribers", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("GET /subscribers without token = %d, want 401", rec.Code)
	}
}

// The counter must actually be fed by the handlers, not merely exist. This is
// the assertion that catches "metric defined but never incremented".
//
// Reuses newTestServer/issueOne/do from server_test.go and lifecycle_test.go
// rather than duplicating that setup.
func TestSuspendIncrementsTheOperationCounter(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()
	imsi := issueOne(t, ts.URL)

	if r := do(t, http.MethodPost, ts.URL+"/subscribers/"+imsi+"/suspend", testToken, ReasonRequest{Reason: "NON_PAYMENT"}); r.StatusCode != http.StatusOK {
		t.Fatalf("suspend: %d", r.StatusCode)
	}

	resp := do(t, http.MethodGet, ts.URL+"/metrics", "", nil)
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `provisioner_operations_total{operation="suspend"`) {
		t.Fatalf("suspend did not increment the operation counter:\n%s", body)
	}
}
