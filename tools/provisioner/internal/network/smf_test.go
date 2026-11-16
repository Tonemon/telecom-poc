package network

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

const pduInfo = `{"items": [
  {"supi": "imsi-999700000000001", "pdu": [{"ebi": 5, "apn": "internet", "ipv4": "10.45.0.4", "pdu_state": "unknown"}], "ue_activity": "unknown"},
  {"supi": "imsi-999700000000002", "pdu": [{"ebi": 5, "apn": "internet", "ipv6": "2001:db8::2"}]},
  {"supi": "999700000000003", "pdu": [{"ebi": 5, "apn": "internet", "ipv4": "10.45.0.3"}]}
], "pager": {"page": 0, "page_size": 100, "count": 3}}`

func TestSMFFetchIPs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/pdu-info" {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(pduInfo))
	}))
	defer srv.Close()
	ips, ok := NewSMFClient(srv.URL).FetchIPs(context.Background())
	if !ok {
		t.Fatal("want reachable")
	}
	if ips["999700000000001"] != "10.45.0.4" || ips["999700000000003"] != "10.45.0.3" {
		t.Fatalf("ips %v", ips)
	}
	if _, has := ips["999700000000002"]; has {
		t.Fatalf("a session without IPv4 should not get an address: %v", ips)
	}
}

func TestSMFDisabledOrDown(t *testing.T) {
	if ips, ok := NewSMFClient("").FetchIPs(context.Background()); ok || len(ips) != 0 {
		t.Fatal("empty URL must disable the client")
	}
	if ips, ok := NewSMFClient("http://127.0.0.1:1").FetchIPs(context.Background()); ok || len(ips) != 0 {
		t.Fatal("an unreachable SMF must give an empty map")
	}
}
