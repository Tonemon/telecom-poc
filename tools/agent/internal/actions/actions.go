// Package actions is what the agent can do on its instance: report health, capture packets on
// a tower or a device, run a live ping and a speedtest in a device. Every action also returns
// the terminal command that does the same by hand.
package actions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/telecom-poc/agent/internal/docker"
	"github.com/telecom-poc/agent/internal/inventory"
)

var (
	ErrUnknown      = errors.New("unknown component")
	ErrNotRunning   = errors.New("not running")
	ErrBusy         = errors.New("busy")
	ErrNotCapturing = errors.New("not capturing")
)

// Compose is how a person reaches the same containers from the repo root.
const Compose = "docker compose -f deploy/4g/docker-compose.yml -f deploy/4g/docker-compose.multi.yml"

type Actions struct {
	cli         docker.Client
	inv         inventory.Inventory
	project     string
	pcapDir     string
	iperfServer string
	stopWait    time.Duration     // how long StopPcap waits for the file to settle
	mmeURL      string            // the MME's metrics server (/enb-info): S1 state as the core sees it
	network     string            // the Docker network the radios are on (power cut / power on)
	towerAddrs  map[string]string // cell id -> the eNB's fixed IPv4 address on it
	http        *http.Client

	mu         sync.Mutex
	compCache  []Component
	compAt     time.Time
	pcaps      map[string]PcapInfo // component -> running capture
	pings      map[string]*pinger
	speedtests map[string]bool
	powering   map[string]bool // services with a power action under way
}

func New(cli docker.Client, inv inventory.Inventory, project, pcapDir, iperfServer string) *Actions {
	return &Actions{cli: cli, inv: inv, project: project, pcapDir: pcapDir, iperfServer: iperfServer,
		stopWait: 3 * time.Second, pcaps: map[string]PcapInfo{}, pings: map[string]*pinger{},
		speedtests: map[string]bool{}, http: &http.Client{Timeout: 2 * time.Second}}
}

// WithMME sets the MME's metrics server, the source of each tower's S1 state.
func (a *Actions) WithMME(url string) *Actions {
	a.mmeURL = url
	return a
}

func (a *Actions) isTower(service string) bool {
	for _, c := range a.inv.Cells {
		if c.Service == service {
			return true
		}
	}
	return false
}

func (a *Actions) isDevice(service string) bool {
	for _, d := range a.inv.Devices {
		if d.Service == service {
			return true
		}
	}
	return false
}

// running returns the container of a service, or ErrNotRunning.
func (a *Actions) running(ctx context.Context, service string) (docker.Container, error) {
	list, err := a.cli.List(ctx, a.project)
	if err != nil {
		return docker.Container{}, err
	}
	for _, c := range list {
		if c.Service == service {
			if c.State != "running" {
				return c, fmt.Errorf("%s is off (%s): %w", service, c.State, ErrNotRunning)
			}
			return c, nil
		}
	}
	return docker.Container{}, fmt.Errorf("%s has no container: %w", service, ErrNotRunning)
}

// command is the terminal equivalent of running args in a service's container.
func command(service string, args ...string) string {
	return Compose + " exec " + service + " " + strings.Join(args, " ")
}

// Component is one container of the instance and how it is doing.
type Component struct {
	Service  string `json:"service"`
	Name     string `json:"name"`
	Hostname string `json:"hostname"`
	State    string `json:"state"`
	Health   string `json:"health"`
	UptimeS  int    `json:"uptime_s"`
	S1       *S1    `json:"s1,omitempty"` // towers: the link to the core, as the MME sees it (nil: unknown)
}

// S1 is a tower's link to the core according to the MME: S1 Setup done, SCTP peer, UEs it serves.
type S1 struct {
	Up           bool   `json:"up"`
	Peer         string `json:"peer,omitempty"`
	ConnectedUEs int    `json:"connected_ues"`
}

// s1State asks the MME which eNBs it has; nil when the MME cannot be asked.
func (a *Actions) s1State(ctx context.Context) map[int]S1 {
	if a.mmeURL == "" {
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.mmeURL+"/enb-info", nil)
	if err != nil {
		return nil
	}
	resp, err := a.http.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	var info struct {
		Items []struct {
			ENBID int `json:"enb_id"`
			S1    struct {
				SCTP struct {
					Peer string `json:"peer"`
				} `json:"sctp"`
				SetupSuccess bool `json:"setup_success"`
			} `json:"s1"`
			ConnectedUEs int `json:"num_connected_ues"`
		} `json:"items"`
	}
	if resp.StatusCode != http.StatusOK || json.NewDecoder(resp.Body).Decode(&info) != nil {
		return nil
	}
	out := map[int]S1{}
	for _, it := range info.Items {
		out[it.ENBID] = S1{Up: it.S1.SetupSuccess, Peer: it.S1.SCTP.Peer, ConnectedUEs: it.ConnectedUEs}
	}
	return out
}

// Components lists every container of the instance; refreshed at most every 5 s.
func (a *Actions) Components(ctx context.Context) ([]Component, error) {
	a.mu.Lock()
	if time.Since(a.compAt) < 5*time.Second && a.compCache != nil {
		defer a.mu.Unlock()
		return a.compCache, nil
	}
	a.mu.Unlock()
	list, err := a.cli.List(ctx, a.project)
	if err != nil {
		return nil, err
	}
	s1 := a.s1State(ctx)
	out := make([]Component, 0, len(list))
	for _, c := range list {
		up := 0
		if c.State == "running" && !c.StartedAt.IsZero() {
			up = int(time.Since(c.StartedAt).Seconds())
		}
		comp := Component{Service: c.Service, Name: c.Name, Hostname: c.Hostname, State: c.State,
			Health: c.Health, UptimeS: up}
		if s1 != nil {
			for _, cell := range a.inv.Cells {
				if cell.Service == c.Service {
					st := s1[cell.ENBID] // an eNB the MME does not know: S1 down
					comp.S1 = &st
				}
			}
		}
		out = append(out, comp)
	}
	a.mu.Lock()
	a.compCache, a.compAt = out, time.Now()
	a.mu.Unlock()
	return out, nil
}
