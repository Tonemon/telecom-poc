package actions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"time"
)

// ErrState: the device is already in the state an action would put it in.
var ErrState = errors.New("wrong state")

// PowerResult is what a power action did, and the terminal command that does the same.
type PowerResult struct {
	Service string `json:"service"`
	Action  string `json:"action"`
	State   string `json:"state"` // on or off, after the action
	Command string `json:"command"`
}

// The power actions only act on the device itself (stop, kill, cut its network, start). Whatever
// the core learns, it learns as it would in real life: from the phone over the radio, from the
// tower over S1, or by noticing on its own (see docs/parts/5-hot-join.md).
var (
	phoneActions = map[string]bool{"switch_off": false, "pull_battery": false, "switch_on": true}
	towerActions = map[string]bool{"shut_down": false, "power_cut": false, "power_on": true}
)

const stopTimeoutS = 15 // srsue waits up to 5 s for its detach to go out (TS 24.301 5.5.2.2)

// WithPower sets the Docker network the radios are on and each tower's fixed IPv4 address on
// it (cell id -> address), which power on needs after a power cut.
func (a *Actions) WithPower(network string, towerAddrs map[string]string) *Actions {
	a.network, a.towerAddrs = network, towerAddrs
	return a
}

// TowerAddrs reads each cell's eNB address from the air's config (the eNB's tx address,
// "tcp://<ipv4>:<port>"): cell id -> IPv4.
func TowerAddrs(airJSON []byte) (map[string]string, error) {
	var cfg struct {
		Cells []struct{ ID, DL string } `json:"cells"`
	}
	if err := json.Unmarshal(airJSON, &cfg); err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, c := range cfg.Cells {
		u, err := url.Parse(c.DL)
		if err != nil || net.ParseIP(u.Hostname()).To4() == nil {
			return nil, fmt.Errorf("cell %s: no IPv4 address in %q", c.ID, c.DL)
		}
		out[c.ID] = u.Hostname()
	}
	return out, nil
}

// Power switches a phone or a tower on or off. Phones: switch_off (a clean stop: srsue sends a
// detach), pull_battery (kill: nothing is sent), switch_on. Towers: shut_down (a clean stop:
// srsenb closes S1), power_cut (the network goes first, so nothing at all is sent, then kill),
// power_on (network back with its fixed address, then start).
func (a *Actions) Power(ctx context.Context, service, action string) (PowerResult, error) {
	var on, ok bool
	cellID := ""
	switch {
	case a.isDevice(service):
		on, ok = phoneActions[action]
	case a.isTower(service):
		on, ok = towerActions[action]
		for _, c := range a.inv.Cells {
			if c.Service == service {
				cellID = c.ID
			}
		}
	}
	if !ok {
		return PowerResult{}, fmt.Errorf("%s cannot %s: %w", service, action, ErrUnknown)
	}
	list, err := a.cli.List(ctx, a.project)
	if err != nil {
		return PowerResult{}, err
	}
	var id, name, state string
	for _, c := range list {
		if c.Service == service {
			id, name, state = c.ID, c.Name, c.State
		}
	}
	if id == "" {
		return PowerResult{}, fmt.Errorf("%s has no container: %w", service, ErrNotRunning)
	}
	if running := state == "running"; running == on {
		word := map[bool]string{true: "on", false: "off"}[running]
		return PowerResult{}, fmt.Errorf("%s is already %s: %w", service, word, ErrState)
	}

	cmd := ""
	switch action {
	case "switch_off", "shut_down":
		err, cmd = a.cli.Stop(ctx, id, stopTimeoutS), Compose+" stop "+service
	case "pull_battery":
		err, cmd = a.cli.Kill(ctx, id), Compose+" kill "+service
	case "power_cut":
		if err = a.cli.NetworkDisconnect(ctx, a.network, id); err == nil {
			err = a.cli.Kill(ctx, id)
		}
		cmd = "docker network disconnect -f " + a.network + " " + name + " && " + Compose + " kill " + service
	case "switch_on":
		err, cmd = a.cli.Start(ctx, id), Compose+" start "+service
	case "power_on":
		cmd = Compose + " start " + service
		nets, nerr := a.cli.Networks(ctx, id)
		if nerr != nil {
			return PowerResult{}, nerr
		}
		if _, on := nets[a.network]; !on {
			ip := a.towerAddrs[cellID]
			if ip == "" {
				return PowerResult{}, fmt.Errorf("no address known for %s on %s", service, a.network)
			}
			if err = a.cli.NetworkConnect(ctx, a.network, id, ip); err != nil {
				return PowerResult{}, err
			}
			cmd = "docker network connect --ip " + ip + " " + a.network + " " + name + " && " + cmd
		}
		err = a.cli.Start(ctx, id)
	}
	if err != nil {
		return PowerResult{}, err
	}
	a.mu.Lock()
	a.compAt = time.Time{} // the map sees the new state on its next poll
	a.mu.Unlock()
	return PowerResult{Service: service, Action: action, State: map[bool]string{true: "on", false: "off"}[on], Command: cmd}, nil
}
