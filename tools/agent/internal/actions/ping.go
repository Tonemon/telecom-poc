package actions

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"time"
)

const pingKeep = 3000 // results kept per UE (10 minutes at 5 per second)

// No -O: with round trips above the 0.2 s interval it reports replies that are merely late as
// missing. A ping counts as lost when a later reply skips its sequence number (pinger.add).
var pingArgs = []string{"ping", "-I", "tun_srsue", "-i", "0.2", "8.8.8.8"}

// PingResult is one ping: N counts every result of this UE since the agent started.
type PingResult struct {
	N     int       `json:"n"`
	Seq   int       `json:"seq"`
	OK    bool      `json:"ok"`
	RTTms float64   `json:"rtt_ms"`
	At    time.Time `json:"at"`
}

type PingState struct {
	On      bool         `json:"on"`
	Results []PingResult `json:"results"`
	Next    int          `json:"next"` // pass as since= to get only newer results
	Command string       `json:"command"`
}

type pinger struct {
	on      bool
	gen     int // which stream is current: an older stream that ends late must not turn this one off
	cancel  context.CancelFunc
	results []PingResult
	next    int
	lastSeq int // highest icmp_seq answered by the current ping process
}

var pingOK = regexp.MustCompile(`icmp_seq=(\d+) .*time=([\d.]+) ms`)

func parsePing(line string) (PingResult, bool) {
	m := pingOK.FindStringSubmatch(line)
	if m == nil {
		return PingResult{}, false
	}
	seq, _ := strconv.Atoi(m[1])
	rtt, _ := strconv.ParseFloat(m[2], 64)
	return PingResult{Seq: seq, OK: true, RTTms: rtt}, true
}

// add records a ping output line: a reply, preceded by a lost result for every sequence
// number it skipped. A reply older than one already seen (reordered) is ignored.
func (p *pinger) add(line string, at time.Time) {
	r, ok := parsePing(line)
	if !ok || r.Seq <= p.lastSeq {
		return
	}
	for seq := p.lastSeq + 1; seq < r.Seq; seq++ {
		p.push(PingResult{Seq: seq, At: at})
	}
	r.At = at
	p.push(r)
	p.lastSeq = r.Seq
}

func (p *pinger) push(r PingResult) {
	r.N = p.next
	p.next++
	p.results = append(p.results, r)
	if len(p.results) > pingKeep {
		p.results = p.results[len(p.results)-pingKeep:]
	}
}

func (a *Actions) pinger(ue string) *pinger {
	p, ok := a.pings[ue]
	if !ok {
		p = &pinger{}
		a.pings[ue] = p
	}
	return p
}

// SetPing starts or stops a continuous ping (every 0.2 s, through tun_srsue) in a device.
func (a *Actions) SetPing(ctx context.Context, ue string, on bool) error {
	if !a.isDevice(ue) {
		return fmt.Errorf("%s: %w (ping runs on devices)", ue, ErrUnknown)
	}
	// Decide and record the new state in one step, so two calls at once cannot both start a ping.
	a.mu.Lock()
	p := a.pinger(ue)
	if p.on == on {
		a.mu.Unlock()
		return nil
	}
	p.on = on
	p.gen++
	gen := p.gen
	if !on && p.cancel != nil {
		p.cancel()
	}
	a.mu.Unlock()

	c, err := a.running(ctx, ue)
	if !on {
		if err == nil {
			_, err = a.cli.Exec(ctx, c.Name, []string{"sh", "-c", "kill -INT $(cat /tmp/agent-ping.pid) 2>/dev/null; rm -f /tmp/agent-ping.pid"})
		}
		return err
	}
	if err != nil {
		a.mu.Lock()
		if p.gen == gen {
			p.on = false
		}
		a.mu.Unlock()
		return err
	}
	streamCtx, cancel := context.WithCancel(context.Background()) // outlives the request
	a.mu.Lock()
	p.cancel, p.lastSeq = cancel, 0
	a.mu.Unlock()
	// Stop a ping left over from before an agent restart, then become the ping (same pid).
	script := "kill -INT $(cat /tmp/agent-ping.pid) 2>/dev/null; echo $$ > /tmp/agent-ping.pid; exec " + joinArgs(pingArgs)
	go func() {
		a.cli.ExecStream(streamCtx, c.Name, []string{"sh", "-c", script}, func(line string) {
			a.mu.Lock()
			p.add(line, time.Now())
			a.mu.Unlock()
		})
		a.mu.Lock()
		if p.gen == gen {
			p.on = false // this ping ended (stopped, container gone, or the stream closed)
		}
		a.mu.Unlock()
	}()
	return nil
}

// PingSince returns the UE's ping results with N >= since.
func (a *Actions) PingSince(ue string, since int) PingState {
	a.mu.Lock()
	defer a.mu.Unlock()
	p := a.pinger(ue)
	st := PingState{On: p.on, Next: p.next, Results: []PingResult{}, Command: command(ue, pingArgs...)}
	for _, r := range p.results {
		if r.N >= since {
			st.Results = append(st.Results, r)
		}
	}
	return st
}

func joinArgs(args []string) string {
	s := ""
	for i, a := range args {
		if i > 0 {
			s += " "
		}
		s += a
	}
	return s
}
