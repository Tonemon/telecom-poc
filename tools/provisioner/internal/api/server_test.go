package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/telecom-poc/provisioner/internal/network"
	"github.com/telecom-poc/provisioner/internal/store"
)

const testToken = "test-token"

func newTestServer() *httptest.Server {
	fixedKey := func() []byte { return bytes.Repeat([]byte{0xAB}, 16) }
	srv := NewServer(store.NewFake(), testToken, "test-operator", fixedKey, network.NewMMEClient(""))
	return httptest.NewServer(srv.Handler())
}

func do(t *testing.T, method, url, token string, body any) *http.Response {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		json.NewEncoder(&buf).Encode(body)
	}
	req, _ := http.NewRequest(method, url, &buf)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestAuth_Required(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()
	resp := do(t, http.MethodGet, ts.URL+"/subscribers", "", nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("no token: got %d want 401", resp.StatusCode)
	}
	resp = do(t, http.MethodGet, ts.URL+"/subscribers", "wrong", nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("bad token: got %d want 401", resp.StatusCode)
	}
}

func TestIssue_ThenGet(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()
	resp := do(t, http.MethodPost, ts.URL+"/subscribers/issue", testToken, IssueRequest{Reason: "NEW_ACTIVATION"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("issue: got %d", resp.StatusCode)
	}
	var issued []IssuedSIM
	json.NewDecoder(resp.Body).Decode(&issued)
	if len(issued) != 1 || issued[0].IMSI != "999700000000001" {
		t.Fatalf("issued = %+v", issued)
	}
	if issued[0].K == "" || issued[0].UsimBlock == "" {
		t.Fatalf("expected Ki + usim block in issue response")
	}
	resp = do(t, http.MethodGet, ts.URL+"/subscribers/999700000000001", testToken, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get after issue: %d", resp.StatusCode)
	}
}

func TestMutating_RequiresReason(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()
	resp := do(t, http.MethodPost, ts.URL+"/subscribers/issue", testToken, IssueRequest{})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("missing reason: got %d want 400", resp.StatusCode)
	}
}
