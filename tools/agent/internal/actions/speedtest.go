package actions

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// UPFAddress is the network's gateway as a UE sees it: the far end of the GTP tunnel.
const UPFAddress = "10.45.0.1"

type Speed struct {
	DownMbps float64 `json:"down_mbps"`
	UpMbps   float64 `json:"up_mbps"`
	Server   string  `json:"server"`
	Command  string  `json:"command"`
}

// Speedtest runs iperf3 in a device: 5 s down (-R), then 5 s up. Without a configured public
// server it measures to the UPF, starting an iperf3 server there first: that covers the radio
// and the core, which is what limits the speed, and needs no outside server.
func (a *Actions) Speedtest(ctx context.Context, ue string) (Speed, error) {
	if !a.isDevice(ue) {
		return Speed{}, fmt.Errorf("%s: %w (speedtests run on devices)", ue, ErrUnknown)
	}
	c, err := a.running(ctx, ue)
	if err != nil {
		return Speed{}, err
	}
	a.mu.Lock()
	if a.speedtests[ue] {
		a.mu.Unlock()
		return Speed{}, fmt.Errorf("%s is already running a speedtest: %w", ue, ErrBusy)
	}
	a.speedtests[ue] = true
	a.mu.Unlock()
	defer func() { a.mu.Lock(); delete(a.speedtests, ue); a.mu.Unlock() }()

	ctx, cancel := context.WithTimeout(ctx, 40*time.Second)
	defer cancel()
	server, prefix := a.iperfServer, ""
	if server == "" {
		server = UPFAddress
		upf, err := a.running(ctx, "upf")
		if err != nil {
			return Speed{}, err
		}
		// A second server on the same port just fails to bind; the first keeps serving.
		if _, err := a.cli.Exec(ctx, upf.Name, []string{"sh", "-c", "iperf3 -s -D >/dev/null 2>&1; true"}); err != nil {
			return Speed{}, err
		}
		prefix = command("upf", "iperf3", "-s", "-D") + " && "
	}
	down, err := a.iperf(ctx, c.Name, server, true)
	if err != nil {
		return Speed{}, err
	}
	up, err := a.iperf(ctx, c.Name, server, false)
	if err != nil {
		return Speed{}, err
	}
	return Speed{DownMbps: down, UpMbps: up, Server: server,
		Command: prefix + command(ue, "iperf3", "-c", server, "-R", "-t", "5")}, nil
}

func (a *Actions) iperf(ctx context.Context, container, server string, reverse bool) (float64, error) {
	cmd := []string{"iperf3", "-c", server, "-t", "5", "-J"}
	if reverse {
		cmd = append(cmd, "-R")
	}
	res, err := a.cli.Exec(ctx, container, cmd)
	if err != nil {
		return 0, err
	}
	var out struct {
		Error string
		End   struct {
			SumReceived *struct {
				BitsPerSecond float64 `json:"bits_per_second"`
			} `json:"sum_received"`
		}
	}
	jsonErr := json.Unmarshal([]byte(res.Stdout), &out)
	switch {
	case out.Error != "":
		return 0, fmt.Errorf("iperf3: %s", out.Error)
	case res.Exit != 0 || jsonErr != nil || out.End.SumReceived == nil:
		msg := strings.TrimSpace(res.Stderr)
		if msg == "" {
			msg = "no result"
		}
		return 0, fmt.Errorf("iperf3 to %s: %s", server, msg)
	}
	return out.End.SumReceived.BitsPerSecond / 1e6, nil
}
