package actions

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

var (
	ansi     = regexp.MustCompile(`\x1b\[[0-9;]*m`)
	imsiLine = regexp.MustCompile(`\[(\d{5,15})\] ([A-Z][A-Za-z ]+?)(?:\(| \(|$)`)
)

// unreachableFrom reads the MME's log: the IMSIs whose last event is "Mobile Reachable timer
// expired" (the core still has them registered but has given up hearing from them; any later
// event of that IMSI, e.g. an attach or a detach, ends it). Sorted.
func unreachableFrom(log string) []string {
	last := map[string]string{}
	for _, line := range strings.Split(ansi.ReplaceAllString(log, ""), "\n") {
		if m := imsiLine.FindStringSubmatch(line); m != nil {
			last[m[1]] = strings.TrimSpace(m[2])
		}
	}
	out := []string{}
	for imsi, ev := range last {
		if ev == "Mobile Reachable timer expired" {
			out = append(out, imsi)
		}
	}
	sort.Strings(out)
	return out
}

// Unreachable lists the phones the core marks unreachable, from the MME's log of the last 2 hours
// (/ue-info does not show the mobile-reachable timer).
func (a *Actions) Unreachable(ctx context.Context) ([]string, error) {
	c, err := a.running(ctx, "mme")
	if err != nil {
		return nil, err
	}
	log, err := a.cli.Logs(ctx, c.ID, time.Now().Add(-2*time.Hour))
	if err != nil {
		return nil, fmt.Errorf("mme log: %w", err)
	}
	return unreachableFrom(log), nil
}
