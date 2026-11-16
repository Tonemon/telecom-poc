// Package actions is what the agent can do on its instance: report health, capture packets on
// a tower or a device, run a live ping and a speedtest in a device. Every action also returns
// the terminal command that does the same by hand.
package actions

import (
	"context"
	"errors"
	"fmt"
	"regexp"
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
	stopWait    time.Duration // how long StopPcap waits for the file to settle

	mu         sync.Mutex
	compCache  []Component
	compAt     time.Time
	pcaps      map[string]PcapInfo // component -> running capture
	pings      map[string]*pinger
	speedtests map[string]bool
}

func New(cli docker.Client, inv inventory.Inventory, project, pcapDir, iperfServer string) *Actions {
	return &Actions{cli: cli, inv: inv, project: project, pcapDir: pcapDir, iperfServer: iperfServer,
		stopWait: 3 * time.Second, pcaps: map[string]PcapInfo{}, pings: map[string]*pinger{},
		speedtests: map[string]bool{}}
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
				return c, fmt.Errorf("%s is %s: %w", service, c.State, ErrNotRunning)
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
	IP       string `json:"ip,omitempty"` // devices: the address the network gave them (tun_srsue)
}

var tunAddr = regexp.MustCompile(`inet (\d+\.\d+\.\d+\.\d+)/`)

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
	out := make([]Component, 0, len(list))
	for _, c := range list {
		up := 0
		if c.State == "running" && !c.StartedAt.IsZero() {
			up = int(time.Since(c.StartedAt).Seconds())
		}
		comp := Component{Service: c.Service, Name: c.Name, Hostname: c.Hostname, State: c.State,
			Health: c.Health, UptimeS: up}
		if c.State == "running" && a.isDevice(c.Service) {
			if res, err := a.cli.Exec(ctx, c.Name, []string{"ip", "-4", "-o", "addr", "show", "tun_srsue"}); err == nil {
				if m := tunAddr.FindStringSubmatch(res.Stdout); m != nil {
					comp.IP = m[1]
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
