package actions

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/telecom-poc/agent/internal/docker"
	"github.com/telecom-poc/agent/internal/inventory"
)

type fakeDocker struct {
	mu         sync.Mutex
	containers []docker.Container
	execs      [][]string // container + cmd
	stdout     map[string]string
	stderr     map[string]string // matching commands exit 1 with this on stderr
	stream     []string
}

func (f *fakeDocker) List(context.Context, string) ([]docker.Container, error) {
	return f.containers, nil
}

func (f *fakeDocker) Exec(_ context.Context, c string, cmd []string) (docker.ExecResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.execs = append(f.execs, append([]string{c}, cmd...))
	joined := strings.Join(cmd, " ")
	for k, v := range f.stderr {
		if strings.Contains(joined, k) {
			return docker.ExecResult{Stderr: v, Exit: 1}, nil
		}
	}
	for k, v := range f.stdout {
		if strings.Contains(joined, k) {
			return docker.ExecResult{Stdout: v}, nil
		}
	}
	return docker.ExecResult{}, nil
}

func (f *fakeDocker) ExecStream(ctx context.Context, c string, cmd []string, line func(string)) error {
	f.mu.Lock()
	f.execs = append(f.execs, append([]string{c}, cmd...))
	f.mu.Unlock()
	for _, l := range f.stream {
		line(l)
	}
	<-ctx.Done()
	return nil
}

func (f *fakeDocker) joined() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, e := range f.execs {
		out = append(out, strings.Join(e, " "))
	}
	return out
}

var inv = inventory.Inventory{
	Cells:   []inventory.Cell{{ID: "A", Service: "enb-a"}},
	Devices: []inventory.Device{{ID: "ue1", Service: "ue1"}, {ID: "ue2", Service: "ue2"}},
}

func running(service string) docker.Container {
	return docker.Container{ID: "id-" + service, Name: "telecom-poc-4g-" + service + "-1", Service: service,
		State: "running", StartedAt: time.Now().Add(-time.Minute)}
}

func newActions(t *testing.T, f *fakeDocker) *Actions {
	t.Helper()
	a := New(f, inv, "telecom-poc-4g", t.TempDir(), "")
	a.stopWait = 10 * time.Millisecond
	return a
}

func TestStartPcapTowerCommand(t *testing.T) {
	f := &fakeDocker{containers: []docker.Container{running("enb-a")}}
	a := newActions(t, f)
	info, err := a.StartPcap(context.Background(), "enb-a")
	if err != nil {
		t.Fatal(err)
	}
	ex := f.joined()[0]
	for _, want := range []string{"telecom-poc-4g-enb-a-1 sh -c", "tcpdump -p -Z root -i eth0 -U -w /pcap/enb-a-", "'sctp or udp port 2152'"} {
		if !strings.Contains(ex, want) {
			t.Errorf("exec %q lacks %q", ex, want)
		}
	}
	if !strings.HasPrefix(info.Command, "docker compose -f deploy/4g/docker-compose.yml -f deploy/4g/docker-compose.multi.yml exec enb-a tcpdump -p -Z root -i eth0 -U -w /pcap/enb-a-") {
		t.Errorf("command %q", info.Command)
	}
	if !strings.HasSuffix(info.File, ".pcap") || !strings.HasPrefix(info.File, "enb-a-") {
		t.Errorf("file %q", info.File)
	}
}

func TestStartPcapUECapturesTun(t *testing.T) {
	f := &fakeDocker{containers: []docker.Container{running("ue2")}}
	if _, err := newActions(t, f).StartPcap(context.Background(), "ue2"); err != nil {
		t.Fatal(err)
	}
	if ex := f.joined()[0]; !strings.Contains(ex, "-i tun_srsue") || strings.Contains(ex, "sctp") {
		t.Errorf("exec %q", ex)
	}
}

func TestStartPcapTwiceIsBusy(t *testing.T) {
	f := &fakeDocker{containers: []docker.Container{running("enb-a")}}
	a := newActions(t, f)
	ctx := context.Background()
	if _, err := a.StartPcap(ctx, "enb-a"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.StartPcap(ctx, "enb-a"); !errors.Is(err, ErrBusy) {
		t.Fatalf("second start: %v, want ErrBusy", err)
	}
	if _, err := a.StopPcap(ctx, "enb-a"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.StopPcap(ctx, "enb-a"); !errors.Is(err, ErrNotCapturing) {
		t.Fatalf("second stop: %v, want ErrNotCapturing", err)
	}
	if ex := f.joined(); !strings.Contains(ex[1], "kill -INT") {
		t.Errorf("stop exec %q", ex[1])
	}
}

func TestUnknownComponent(t *testing.T) {
	a := newActions(t, &fakeDocker{containers: []docker.Container{running("mme")}})
	ctx := context.Background()
	if _, err := a.StartPcap(ctx, "mme"); !errors.Is(err, ErrUnknown) {
		t.Errorf("pcap on mme: %v, want ErrUnknown (only towers and devices)", err)
	}
	if err := a.SetPing(ctx, "enb-a", true); !errors.Is(err, ErrUnknown) {
		t.Errorf("ping on a tower: %v, want ErrUnknown", err)
	}
}

func TestActionsOnExitedContainer(t *testing.T) {
	dead := running("ue2")
	dead.State = "exited"
	f := &fakeDocker{containers: []docker.Container{dead}}
	a := newActions(t, f)
	ctx := context.Background()
	if _, err := a.StartPcap(ctx, "ue2"); !errors.Is(err, ErrNotRunning) {
		t.Errorf("pcap: %v", err)
	}
	if err := a.SetPing(ctx, "ue2", true); !errors.Is(err, ErrNotRunning) {
		t.Errorf("ping: %v", err)
	}
	if _, err := a.Speedtest(ctx, "ue2"); !errors.Is(err, ErrNotRunning) {
		t.Errorf("speedtest: %v", err)
	}
	comps, err := a.Components(ctx)
	if err != nil || len(comps) != 1 || comps[0].State != "exited" {
		t.Fatalf("components %+v, %v", comps, err)
	}
	if len(f.joined()) != 0 {
		t.Errorf("no exec expected on an exited container, got %v", f.joined())
	}
}

func TestPingParse(t *testing.T) {
	if _, ok := parsePing("PING 8.8.8.8 (8.8.8.8) from 10.45.0.4 tun_srsue: 56(84) bytes of data."); ok {
		t.Error("header parsed as a result")
	}
	r, ok := parsePing("64 bytes from 8.8.8.8: icmp_seq=3 ttl=115 time=187 ms")
	if !ok || r.Seq != 3 || !r.OK || r.RTTms != 187 {
		t.Fatalf("got %+v %v", r, ok)
	}
}

// A slow reply is not a lost one: only a sequence number that a later reply skips is lost.
func TestPingLossFromGaps(t *testing.T) {
	var p pinger
	for _, l := range []string{
		"64 bytes from 8.8.8.8: icmp_seq=1 ttl=115 time=350 ms",
		"64 bytes from 8.8.8.8: icmp_seq=4 ttl=115 time=90 ms",
		"64 bytes from 8.8.8.8: icmp_seq=5 ttl=115 time=95 ms",
	} {
		p.add(l, time.Now())
	}
	var got []string
	for _, r := range p.results {
		got = append(got, fmt.Sprintf("%d:%d:%v", r.N, r.Seq, r.OK))
	}
	if strings.Join(got, " ") != "0:1:true 1:2:false 2:3:false 3:4:true 4:5:true" {
		t.Fatalf("got %v", got)
	}
}

func TestPingOnOff(t *testing.T) {
	f := &fakeDocker{containers: []docker.Container{running("ue2")},
		stream: []string{"64 bytes from 8.8.8.8: icmp_seq=1 ttl=115 time=90.5 ms", "64 bytes from 8.8.8.8: icmp_seq=2 ttl=115 time=99 ms"}}
	a := newActions(t, f)
	ctx := context.Background()
	if err := a.SetPing(ctx, "ue2", true); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	var res PingState
	for time.Now().Before(deadline) {
		if res = a.PingSince("ue2", 0); len(res.Results) == 2 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !res.On || len(res.Results) != 2 || res.Results[0].N != 0 || res.Results[1].N != 1 || res.Next != 2 {
		t.Fatalf("ping state %+v", res)
	}
	if later := a.PingSince("ue2", 2); len(later.Results) != 0 {
		t.Fatalf("since=2: %+v", later)
	}
	if !strings.Contains(f.joined()[0], "ping -I tun_srsue -i 0.2 8.8.8.8") {
		t.Errorf("exec %q", f.joined()[0])
	}
	if err := a.SetPing(ctx, "ue2", false); err != nil {
		t.Fatal(err)
	}
	if a.PingSince("ue2", 0).On {
		t.Error("still on after off")
	}
	if ex := f.joined(); !strings.Contains(ex[len(ex)-1], "kill -INT") {
		t.Errorf("stop exec %q", ex[len(ex)-1])
	}
}

const iperfJSON = `{"start": {}, "end": {"sum_received": {"bits_per_second": 750000.0}}}`

// By default the speedtest measures to the network's gateway (the UPF): the radio and core path,
// without depending on a public iperf3 server.
func TestSpeedtestToUPF(t *testing.T) {
	f := &fakeDocker{containers: []docker.Container{running("ue2"), running("upf")},
		stdout: map[string]string{"iperf3 -c": iperfJSON}}
	s, err := newActions(t, f).Speedtest(context.Background(), "ue2")
	if err != nil {
		t.Fatal(err)
	}
	if s.DownMbps != 0.75 || s.UpMbps != 0.75 || s.Server != "10.45.0.1" {
		t.Fatalf("speed %+v", s)
	}
	ex := f.joined()
	if !strings.HasPrefix(ex[0], "telecom-poc-4g-upf-1 ") || !strings.Contains(ex[0], "iperf3 -s -D") {
		t.Errorf("first exec %q: want the server started in the UPF", ex[0])
	}
	if !strings.Contains(ex[1], "telecom-poc-4g-ue2-1 iperf3 -c 10.45.0.1 -t 5 -J -R") {
		t.Errorf("down exec %q", ex[1])
	}
	want := Compose + " exec upf iperf3 -s -D && " + Compose + " exec ue2 iperf3 -c 10.45.0.1 -R -t 5"
	if s.Command != want {
		t.Errorf("command\n got %q\nwant %q", s.Command, want)
	}
}

func TestSpeedtestPublicServer(t *testing.T) {
	f := &fakeDocker{containers: []docker.Container{running("ue2")}, stdout: map[string]string{"iperf3 -c": iperfJSON}}
	a := New(f, inv, "telecom-poc-4g", t.TempDir(), "iperf.he.net")
	s, err := a.Speedtest(context.Background(), "ue2")
	if err != nil || s.Server != "iperf.he.net" || strings.Contains(strings.Join(f.joined(), "|"), "iperf3 -s") {
		t.Fatalf("speed %+v err %v execs %v", s, err, f.joined())
	}
}

func TestSpeedtestServerError(t *testing.T) {
	f := &fakeDocker{containers: []docker.Container{running("ue2"), running("upf")},
		stderr: map[string]string{"iperf3 -c": "iperf3: error - the server is busy running a test. try again later"}}
	if _, err := newActions(t, f).Speedtest(context.Background(), "ue2"); err == nil || !strings.Contains(err.Error(), "busy") {
		t.Fatalf("want the iperf3 error, got %v", err)
	}
}

func TestSpeedtestEmptyResultIsError(t *testing.T) {
	f := &fakeDocker{containers: []docker.Container{running("ue2"), running("upf")},
		stdout: map[string]string{"iperf3 -c": `{"start": {}, "end": {}}`}}
	if _, err := newActions(t, f).Speedtest(context.Background(), "ue2"); err == nil {
		t.Fatal("want an error when iperf3 reports no result")
	}
}

func TestPcapFiles(t *testing.T) {
	a := newActions(t, &fakeDocker{})
	os.WriteFile(filepath.Join(a.pcapDir, "enb-a-20261116-120000.pcap"), []byte("0123456789"), 0o644)
	os.WriteFile(filepath.Join(a.pcapDir, "notes.txt"), []byte("x"), 0o644)
	files, err := a.Pcaps()
	if err != nil || len(files) != 1 || files[0].Name != "enb-a-20261116-120000.pcap" || files[0].Bytes != 10 {
		t.Fatalf("files %+v, %v", files, err)
	}
	if p, err := a.PcapPath("enb-a-20261116-120000.pcap"); err != nil || filepath.Base(p) != "enb-a-20261116-120000.pcap" {
		t.Fatalf("path %q, %v", p, err)
	}
}

func TestPcapPathRejectsTraversal(t *testing.T) {
	a := newActions(t, &fakeDocker{})
	for _, bad := range []string{"../x.pcap", "a/b.pcap", "x.txt", "", ".pcap"} {
		if _, err := a.PcapPath(bad); err == nil {
			t.Errorf("PcapPath(%q) accepted", bad)
		}
	}
}

// Tower health includes the S1 link as the core (MME) sees it, not just the container state.
func TestComponentsReportTowerS1FromMME(t *testing.T) {
	mme := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/enb-info" {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(`{"items": [{"enb_id": 411, "plmn": "99970",
			"s1": {"sctp": {"peer": "[172.22.0.40]:36412"}, "setup_success": true}, "num_connected_ues": 2}]}`))
	}))
	defer mme.Close()
	inv2 := inventory.Inventory{
		Cells:   []inventory.Cell{{ID: "A", Service: "enb-a", ENBID: 411}, {ID: "B", Service: "enb-b", ENBID: 412}},
		Devices: inv.Devices,
	}
	f := &fakeDocker{containers: []docker.Container{running("enb-a"), running("enb-b"), running("ue2")}}
	a := New(f, inv2, "telecom-poc-4g", t.TempDir(), "")
	a.mmeURL = mme.URL
	comps, err := a.Components(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]Component{}
	for _, c := range comps {
		by[c.Service] = c
	}
	if s := by["enb-a"].S1; s == nil || !s.Up || s.Peer != "[172.22.0.40]:36412" || s.ConnectedUEs != 2 {
		t.Fatalf("enb-a S1 %+v", s)
	}
	if s := by["enb-b"].S1; s == nil || s.Up {
		t.Fatalf("enb-b is not known to the MME, want S1 down: %+v", s)
	}
	if by["ue2"].S1 != nil {
		t.Fatal("devices have no S1")
	}
	if len(f.joined()) != 0 {
		t.Fatalf("health must not exec in containers any more: %v", f.joined())
	}
}

func TestComponentsS1UnknownWhenMMEUnreachable(t *testing.T) {
	f := &fakeDocker{containers: []docker.Container{running("enb-a")}}
	a := newActions(t, f)
	a.mmeURL = "http://127.0.0.1:1"
	comps, _ := a.Components(context.Background())
	if comps[0].S1 != nil {
		t.Fatalf("an unreachable MME must leave S1 unknown (nil), got %+v", comps[0].S1)
	}
}

// blockingDocker blocks Exec of tcpdump until released, so two starts overlap.
type blockingDocker struct {
	*fakeDocker
	entered, release chan struct{}
}

func (b *blockingDocker) Exec(ctx context.Context, c string, cmd []string) (docker.ExecResult, error) {
	if strings.Contains(strings.Join(cmd, " "), "tcpdump") {
		b.entered <- struct{}{}
		<-b.release
	}
	return b.fakeDocker.Exec(ctx, c, cmd)
}

func TestConcurrentStartPcapStartsOneTcpdump(t *testing.T) {
	f := &fakeDocker{containers: []docker.Container{running("enb-a")}}
	b := &blockingDocker{fakeDocker: f, entered: make(chan struct{}, 2), release: make(chan struct{})}
	a := New(b, inv, "telecom-poc-4g", t.TempDir(), "")
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() { _, err := a.StartPcap(context.Background(), "enb-a"); errs <- err }()
	}
	<-b.entered
	select {
	case <-b.entered:
		close(b.release)
		t.Fatal("two tcpdumps started for one component")
	case err := <-errs:
		if !errors.Is(err, ErrBusy) {
			t.Fatalf("second start: %v, want ErrBusy", err)
		}
	case <-time.After(time.Second):
		t.Fatal("second start neither ran nor failed")
	}
	close(b.release)
	if err := <-errs; err != nil {
		t.Fatalf("first start: %v", err)
	}
}

func TestFailedStartPcapFreesTheSlot(t *testing.T) {
	f := &fakeDocker{containers: []docker.Container{running("enb-a")}, stderr: map[string]string{"tcpdump": "no permission"}}
	a := newActions(t, f)
	if _, err := a.StartPcap(context.Background(), "enb-a"); err == nil {
		t.Fatal("want an error")
	}
	f.stderr = nil
	if _, err := a.StartPcap(context.Background(), "enb-a"); err != nil {
		t.Fatalf("after a failed start: %v", err)
	}
}

// slowExitDocker: a ping stream returns only after its context ends AND it is let go, so an old
// stream can finish after a new one started.
type slowExitDocker struct {
	*fakeDocker
	exit chan struct{}
}

func (s *slowExitDocker) ExecStream(ctx context.Context, c string, cmd []string, line func(string)) error {
	s.fakeDocker.ExecStream(ctx, c, cmd, line)
	<-s.exit
	return nil
}

func TestOldPingStreamDoesNotTurnOffNewOne(t *testing.T) {
	f := &fakeDocker{containers: []docker.Container{running("ue2")}}
	s := &slowExitDocker{fakeDocker: f, exit: make(chan struct{})}
	a := New(s, inv, "telecom-poc-4g", t.TempDir(), "")
	ctx := context.Background()
	if err := a.SetPing(ctx, "ue2", true); err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)
	a.SetPing(ctx, "ue2", false)
	if err := a.SetPing(ctx, "ue2", true); err != nil {
		t.Fatal(err)
	}
	s.exit <- struct{}{} // the first stream ends now, after the second started
	time.Sleep(20 * time.Millisecond)
	if !a.PingSince("ue2", 0).On {
		t.Fatal("the old stream's exit turned the new ping off")
	}
	close(s.exit)
}

func TestPingStartKillsALeftoverPing(t *testing.T) {
	f := &fakeDocker{containers: []docker.Container{running("ue2")}}
	a := newActions(t, f)
	a.SetPing(context.Background(), "ue2", true)
	time.Sleep(20 * time.Millisecond)
	if ex := f.joined()[0]; !strings.Contains(ex, "kill -INT $(cat /tmp/agent-ping.pid)") {
		t.Fatalf("ping start does not clean up a leftover ping: %q", ex)
	}
}
