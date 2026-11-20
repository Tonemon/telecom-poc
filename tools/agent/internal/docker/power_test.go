package docker

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"
)

// fakeDaemon records every request (method, path with query, body) on a Unix socket.
type fakeDaemon struct {
	mu   sync.Mutex
	reqs []string
}

func startDaemon(t *testing.T, handler func(w http.ResponseWriter, r *http.Request)) (*fakeDaemon, Client) {
	t.Helper()
	d := &fakeDaemon{}
	path := filepath.Join(t.TempDir(), "docker.sock")
	l, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		d.mu.Lock()
		d.reqs = append(d.reqs, r.Method+" "+r.URL.RequestURI()+" "+string(body))
		d.mu.Unlock()
		if handler != nil {
			handler(w, r)
			return
		}
		w.WriteHeader(204)
	})}
	go srv.Serve(l)
	t.Cleanup(func() { srv.Close() })
	return d, NewSocket(path)
}

func TestPowerCalls(t *testing.T) {
	d, c := startDaemon(t, nil)
	ctx := context.Background()
	for _, err := range []error{
		c.Stop(ctx, "id1", 15),
		c.Kill(ctx, "id1"),
		c.Start(ctx, "id1"),
		c.NetworkDisconnect(ctx, "net1", "id1"),
		c.NetworkConnect(ctx, "net1", "id1", "172.22.0.50"),
	} {
		if err != nil {
			t.Fatal(err)
		}
	}
	want := []string{
		"POST /containers/id1/stop?t=15 ",
		"POST /containers/id1/kill ",
		"POST /containers/id1/start ",
		`POST /networks/net1/disconnect {"Container":"id1","Force":true}`,
		`POST /networks/net1/connect {"Container":"id1","EndpointConfig":{"IPAMConfig":{"IPv4Address":"172.22.0.50"}}}`,
	}
	if !reflect.DeepEqual(d.reqs, want) {
		t.Fatalf("requests\n%q\nwant\n%q", d.reqs, want)
	}
}

func TestPowerCallError(t *testing.T) {
	_, c := startDaemon(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(409)
		w.Write([]byte(`{"message":"container already started"}`))
	})
	if err := c.Start(context.Background(), "id1"); err == nil {
		t.Fatal("want the daemon's error")
	}
}

func TestNetworks(t *testing.T) {
	_, c := startDaemon(t, func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"NetworkSettings": map[string]any{"Networks": map[string]any{
			"telecom-poc-4g_epc": map[string]any{"IPAddress": "172.22.0.50"}}}})
	})
	got, err := c.Networks(context.Background(), "id1")
	if err != nil || !reflect.DeepEqual(got, map[string]string{"telecom-poc-4g_epc": "172.22.0.50"}) {
		t.Fatalf("%v %v", got, err)
	}
}

func TestLogs(t *testing.T) {
	since := time.Unix(1700000000, 0)
	d, c := startDaemon(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Write(frame(1, "out line\n"))
		w.Write(frame(2, "err line\n"))
	})
	got, err := c.Logs(context.Background(), "id1", since)
	if err != nil || got != "out line\nerr line\n" {
		t.Fatalf("%q %v", got, err)
	}
	if d.reqs[0] != "GET /containers/id1/logs?stdout=1&stderr=1&since=1700000000 " {
		t.Fatalf("request %q", d.reqs[0])
	}
}
