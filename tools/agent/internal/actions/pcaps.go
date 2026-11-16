package actions

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Towers: S1AP (SCTP) and GTP-U on the eNB's network interface, not the ZMQ radio samples.
const towerFilter = "sctp or udp port 2152"

type PcapInfo struct {
	Component string    `json:"component"`
	File      string    `json:"file"`
	Started   time.Time `json:"started"`
	Command   string    `json:"command"`
}

type PcapFile struct {
	Name     string    `json:"name"`
	Bytes    int64     `json:"bytes"`
	Modified time.Time `json:"modified"`
}

func (a *Actions) tcpdumpArgs(component, file string) ([]string, error) {
	switch {
	case a.isTower(component):
		return []string{"tcpdump", "-p", "-Z", "root", "-i", "eth0", "-U", "-w", "/pcap/" + file, "'" + towerFilter + "'"}, nil
	case a.isDevice(component):
		return []string{"tcpdump", "-p", "-Z", "root", "-i", "tun_srsue", "-U", "-w", "/pcap/" + file}, nil
	}
	return nil, fmt.Errorf("%s: %w (pcaps are for towers and devices)", component, ErrUnknown)
}

// StartPcap starts tcpdump in the component's container, writing to the shared pcap folder.
func (a *Actions) StartPcap(ctx context.Context, component string) (PcapInfo, error) {
	file := fmt.Sprintf("%s-%s.pcap", component, time.Now().UTC().Format("20060102-150405"))
	args, err := a.tcpdumpArgs(component, file)
	if err != nil {
		return PcapInfo{}, err
	}
	info := PcapInfo{Component: component, File: file, Started: time.Now(),
		Command: command(component, args...) + "    # Ctrl-C to stop"}
	// Reserve the slot before starting, so two starts at once cannot both run tcpdump.
	a.mu.Lock()
	if _, busy := a.pcaps[component]; busy {
		a.mu.Unlock()
		return PcapInfo{}, fmt.Errorf("%s is already capturing: %w", component, ErrBusy)
	}
	a.pcaps[component] = info
	a.mu.Unlock()
	if err := a.startTcpdump(ctx, component, args); err != nil {
		a.mu.Lock()
		delete(a.pcaps, component)
		a.mu.Unlock()
		return PcapInfo{}, err
	}
	return info, nil
}

func (a *Actions) startTcpdump(ctx context.Context, component string, args []string) error {
	c, err := a.running(ctx, component)
	if err != nil {
		return err
	}
	// Stop a capture left over from before an agent restart, background the new one, keep its
	// pid, and fail if it died straight away (e.g. no permission).
	script := "kill -INT $(cat /tmp/agent-pcap.pid) 2>/dev/null; " +
		strings.Join(args, " ") + " >/tmp/agent-pcap.log 2>&1 & echo $! > /tmp/agent-pcap.pid; " +
		"sleep 0.5; kill -0 $! 2>/dev/null || { cat /tmp/agent-pcap.log; exit 1; }"
	res, err := a.cli.Exec(ctx, c.Name, []string{"sh", "-c", script})
	if err != nil {
		return err
	}
	if res.Exit != 0 {
		return fmt.Errorf("tcpdump in %s failed: %s", component, strings.TrimSpace(res.Stdout+res.Stderr))
	}
	return nil
}

// StopPcap stops the component's capture (SIGINT, so the file is complete) and waits for it to settle.
func (a *Actions) StopPcap(ctx context.Context, component string) (PcapInfo, error) {
	a.mu.Lock()
	info, ok := a.pcaps[component]
	a.mu.Unlock()
	if !ok {
		return PcapInfo{}, fmt.Errorf("%s: %w", component, ErrNotCapturing)
	}
	c, err := a.running(ctx, component)
	if err == nil {
		_, err = a.cli.Exec(ctx, c.Name, []string{"sh", "-c", "kill -INT $(cat /tmp/agent-pcap.pid) && rm -f /tmp/agent-pcap.pid"})
	}
	a.mu.Lock()
	delete(a.pcaps, component)
	a.mu.Unlock()
	a.waitSettled(filepath.Join(a.pcapDir, info.File))
	return info, err
}

func (a *Actions) waitSettled(path string) {
	deadline := time.Now().Add(a.stopWait)
	last := int64(-1)
	for time.Now().Before(deadline) {
		st, err := os.Stat(path)
		if err == nil && st.Size() == last {
			return
		}
		if err == nil {
			last = st.Size()
		}
		time.Sleep(min(100*time.Millisecond, a.stopWait))
	}
}

// Capturing lists the running captures.
func (a *Actions) Capturing() []PcapInfo {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]PcapInfo, 0, len(a.pcaps))
	for _, p := range a.pcaps {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Component < out[j].Component })
	return out
}

// Pcaps lists the .pcap files in the pcap folder, newest first.
func (a *Actions) Pcaps() ([]PcapFile, error) {
	entries, err := os.ReadDir(a.pcapDir)
	if err != nil {
		return nil, err
	}
	out := []PcapFile{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".pcap") {
			continue
		}
		if st, err := e.Info(); err == nil {
			out = append(out, PcapFile{Name: e.Name(), Bytes: st.Size(), Modified: st.ModTime()})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Modified.After(out[j].Modified) })
	return out, nil
}

// PcapPath is the full path of a pcap file name, refusing anything outside the pcap folder.
func (a *Actions) PcapPath(name string) (string, error) {
	if name == "" || strings.ContainsAny(name, `/\`) || !strings.HasSuffix(name, ".pcap") ||
		name == ".pcap" || strings.HasPrefix(name, ".") {
		return "", fmt.Errorf("bad pcap name %q: %w", name, ErrUnknown)
	}
	return filepath.Join(a.pcapDir, name), nil
}
