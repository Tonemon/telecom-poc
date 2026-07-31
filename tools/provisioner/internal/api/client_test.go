package api

import (
	"net/http/httptest"
	"testing"

	"github.com/telecom-poc/provisioner/internal/store"
)

func TestClient_RoundTrip(t *testing.T) {
	fixedKey := func() []byte {
		b := make([]byte, 16)
		for i := range b {
			b[i] = 0x11
		}
		return b
	}
	ts := httptest.NewServer(NewServer(store.NewFake(), testToken, "op", fixedKey).Handler())
	defer ts.Close()
	c := NewClient(ts.URL, testToken)

	issued, err := c.Issue(IssueRequest{Reason: "NEW_ACTIVATION"})
	if err != nil || len(issued) != 1 {
		t.Fatalf("issue: %v %+v", err, issued)
	}
	imsi := issued[0].IMSI
	if err := c.Suspend(imsi, ReasonRequest{Reason: "FRAUD"}); err != nil {
		t.Fatalf("suspend: %v", err)
	}
	v, err := c.Get(imsi)
	if err != nil || v.Status != "suspended" {
		t.Fatalf("get: %v %+v", err, v)
	}
	if _, err := c.List(); err != nil {
		t.Fatalf("list: %v", err)
	}
}

func TestClient_SurfacesServerError(t *testing.T) {
	fixedKey := func() []byte { return make([]byte, 16) }
	ts := httptest.NewServer(NewServer(store.NewFake(), testToken, "op", fixedKey).Handler())
	defer ts.Close()
	c := NewClient(ts.URL, testToken)
	if _, err := c.Issue(IssueRequest{}); err == nil { // missing reason -> 400
		t.Fatal("expected error surfaced from 400")
	}
}
