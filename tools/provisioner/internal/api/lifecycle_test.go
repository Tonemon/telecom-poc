package api

import (
	"encoding/json"
	"net/http"
	"testing"
)

func issueOne(t *testing.T, url string) string {
	t.Helper()
	resp := do(t, http.MethodPost, url+"/subscribers/issue", testToken, IssueRequest{Reason: "NEW_ACTIVATION"})
	var issued []IssuedSIM
	json.NewDecoder(resp.Body).Decode(&issued)
	return issued[0].IMSI
}

func TestSuspendResume(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()
	imsi := issueOne(t, ts.URL)

	// reason required
	if r := do(t, http.MethodPost, ts.URL+"/subscribers/"+imsi+"/suspend", testToken, ReasonRequest{}); r.StatusCode != http.StatusBadRequest {
		t.Fatalf("suspend w/o reason: %d want 400", r.StatusCode)
	}
	if r := do(t, http.MethodPost, ts.URL+"/subscribers/"+imsi+"/suspend", testToken, ReasonRequest{Reason: "NON_PAYMENT"}); r.StatusCode != http.StatusOK {
		t.Fatalf("suspend: %d", r.StatusCode)
	}
	resp := do(t, http.MethodGet, ts.URL+"/subscribers/"+imsi, testToken, nil)
	var v SubscriberView
	json.NewDecoder(resp.Body).Decode(&v)
	if v.Status != "suspended" {
		t.Fatalf("status = %q want suspended", v.Status)
	}
	if r := do(t, http.MethodPost, ts.URL+"/subscribers/"+imsi+"/resume", testToken, ReasonRequest{Reason: "PAYMENT_RECEIVED"}); r.StatusCode != http.StatusOK {
		t.Fatalf("resume: %d", r.StatusCode)
	}
}

func TestPlanAndIP(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()
	imsi := issueOne(t, ts.URL)
	if r := do(t, http.MethodPatch, ts.URL+"/subscribers/"+imsi+"/plan", testToken, PlanRequest{DL: "100M", UL: "50M", Reason: "UPGRADE"}); r.StatusCode != http.StatusOK {
		t.Fatalf("plan: %d", r.StatusCode)
	}
	if r := do(t, http.MethodPatch, ts.URL+"/subscribers/"+imsi+"/ip", testToken, IPRequest{IPv4: "10.45.0.9", Reason: "ENTERPRISE"}); r.StatusCode != http.StatusOK {
		t.Fatalf("ip: %d", r.StatusCode)
	}
	resp := do(t, http.MethodGet, ts.URL+"/subscribers/"+imsi, testToken, nil)
	var v SubscriberView
	json.NewDecoder(resp.Body).Decode(&v)
	if v.DL != "100M" || v.StaticIPv4 != "10.45.0.9" {
		t.Fatalf("view=%+v", v)
	}
}

func TestHistoryAndUsageSeam(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()
	imsi := issueOne(t, ts.URL)
	do(t, http.MethodPost, ts.URL+"/subscribers/"+imsi+"/suspend", testToken, ReasonRequest{Reason: "FRAUD"})
	resp := do(t, http.MethodGet, ts.URL+"/subscribers/"+imsi+"/history", testToken, nil)
	var hist []map[string]any
	json.NewDecoder(resp.Body).Decode(&hist)
	if len(hist) < 2 { // issue + suspend
		t.Fatalf("history len=%d want >=2", len(hist))
	}
	if r := do(t, http.MethodGet, ts.URL+"/subscribers/"+imsi+"/usage", testToken, nil); r.StatusCode != http.StatusNotImplemented {
		t.Fatalf("usage seam: %d want 501", r.StatusCode)
	}
}
