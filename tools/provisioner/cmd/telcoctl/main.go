package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/telecom-poc/provisioner/internal/api"
)

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func usage() {
	fmt.Fprintln(os.Stderr, `telcoctl — operator console for the provisioner
Usage:
  telcoctl issue-sim   [--count N] [--apn internet] --reason NEW_ACTIVATION [--note ...] [--write-ue-conf PATH]
  telcoctl add         --imsi I --ki K --opc O [--apn internet] --reason NEW_ACTIVATION
  telcoctl remove      <imsi>
  telcoctl suspend     <imsi> --reason NON_PAYMENT|LOST_STOLEN|FRAUD|MAINTENANCE [--note ...]
  telcoctl resume      <imsi> --reason PAYMENT_RECEIVED|RECOVERED|CLEARED
  telcoctl set-plan    <imsi> --dl 100M --ul 50M --reason UPGRADE|DOWNGRADE|PROMOTION
  telcoctl set-ip      <imsi> <ipv4|--clear> --reason ENTERPRISE|M2M|IOT
  telcoctl list        [--json]
  telcoctl get         <imsi> [--json]
  telcoctl history     <imsi>
list/get show a human-readable summary by default (subscriber record plus,
when known, the UE's live network state from the MME) -- pass --json for
the full structured data.
Env: TELCOCTL_SERVER (default http://127.0.0.1:8080), TELCOCTL_TOKEN`)
}

// tiny flag helper: pull "--key value" (and boolean "--flag") out of args.
func popFlag(args []string, name string) (val string, present bool, rest []string) {
	rest = []string{}
	for i := 0; i < len(args); i++ {
		if args[i] == "--"+name {
			present = true
			if i+1 < len(args) && len(args[i+1]) > 0 && args[i+1][0] != '-' {
				val = args[i+1]
				i++
			}
			continue
		}
		rest = append(rest, args[i])
	}
	return
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	c := api.NewClient(env("TELCOCTL_SERVER", "http://127.0.0.1:8080"), os.Getenv("TELCOCTL_TOKEN"))
	cmd, args := os.Args[1], os.Args[2:]

	reason, _, args := popFlag(args, "reason")
	note, _, args := popFlag(args, "note")
	_, jsonOut, args := popFlag(args, "json")

	var err error
	switch cmd {
	case "issue-sim":
		countStr, _, a := popFlag(args, "count")
		apn, _, a := popFlag(a, "apn")
		ueConf, writeUE, a := popFlag(a, "write-ue-conf")
		_ = a
		count := 1
		if countStr != "" {
			fmt.Sscanf(countStr, "%d", &count)
		}
		var issued []api.IssuedSIM
		issued, err = c.Issue(api.IssueRequest{Count: count, APN: apn, Reason: reason, Note: note})
		if err == nil {
			printJSON(issued)
			if writeUE && len(issued) > 0 {
				if werr := os.WriteFile(ueConf+".usim", []byte(issued[0].UsimBlock), 0o644); werr != nil {
					err = werr
				} else {
					fmt.Printf("wrote UE [usim] block to %s.usim\n", ueConf)
				}
			}
		}
	case "add":
		imsi, _, a := popFlag(args, "imsi")
		ki, _, a := popFlag(a, "ki")
		opc, _, a := popFlag(a, "opc")
		apn, _, a := popFlag(a, "apn")
		_ = a
		var s api.IssuedSIM
		s, err = c.Add(api.AddRequest{IMSI: imsi, K: ki, OPc: opc, APN: apn, Reason: reason, Note: note})
		if err == nil {
			printJSON(s)
		}
	case "remove":
		err = c.Remove(arg0(args))
	case "suspend":
		err = c.Suspend(arg0(args), api.ReasonRequest{Reason: reason, Note: note})
	case "resume":
		err = c.Resume(arg0(args), api.ReasonRequest{Reason: reason, Note: note})
	case "set-plan":
		dl, _, a := popFlag(args, "dl")
		ul, _, a := popFlag(a, "ul")
		err = c.SetPlan(arg0(a), api.PlanRequest{DL: dl, UL: ul, Reason: reason, Note: note})
	case "set-ip":
		_, clear, a := popFlag(args, "clear")
		ip := ""
		if !clear {
			ip = arg1(a)
		}
		err = c.SetIP(arg0(a), api.IPRequest{IPv4: ip, Reason: reason, Note: note})
	case "list":
		var v []api.SubscriberView
		v, err = c.List()
		if err == nil {
			if jsonOut {
				printJSON(v)
			} else {
				renderList(v)
			}
		}
	case "get":
		var v api.SubscriberView
		v, err = c.Get(arg0(args))
		if err == nil {
			if jsonOut {
				printJSON(v)
			} else {
				renderGet(v)
			}
		}
	case "history":
		h, herr := c.History(arg0(args))
		err = herr
		if err == nil {
			printJSON(h)
		}
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func arg0(a []string) string {
	if len(a) > 0 {
		return a[0]
	}
	return ""
}
func arg1(a []string) string {
	if len(a) > 1 {
		return a[1]
	}
	return arg0(a)
}
func printJSON(v any) {
	b, _ := json.MarshalIndent(v, "", "  ")
	fmt.Println(string(b))
}

// renderGet prints one subscriber as a labeled summary: the provisioning
// record (what the operator set) plus, when known, the UE's live network
// state (what it's actually doing right now) from the MME.
func renderGet(v api.SubscriberView) {
	w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintf(w, "IMSI\t%s\n", v.IMSI)
	fmt.Fprintf(w, "Status\t%s (%s/%s)\n", v.Status, v.DL, v.UL)
	if v.StaticIPv4 != "" {
		fmt.Fprintf(w, "Static IP\t%s\n", v.StaticIPv4)
	}
	for i, line := range networkLines(v.Network) {
		label := ""
		if i == 0 {
			label = "Network"
		}
		fmt.Fprintf(w, "%s\t%s\n", label, line)
	}
	if v.LastAction != "" {
		fmt.Fprintf(w, "Last\t%s / %s\n", v.LastAction, v.LastReason)
	}
	w.Flush()
}

// renderList prints one row per subscriber, compact enough to scan.
func renderList(vs []api.SubscriberView) {
	if len(vs) == 0 {
		fmt.Println("no subscribers")
		return
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(w, "IMSI\tSTATUS\tPLAN\tNETWORK\tLAST")
	for _, v := range vs {
		fmt.Fprintf(w, "%s\t%s\t%s/%s\t%s\t%s\n",
			v.IMSI, v.Status, v.DL, v.UL, networkSummary(v.Network), lastSummary(v))
	}
	w.Flush()
}

// networkLines renders a UE's live state for the multi-line `get` view:
// connection state on its own line, then cell/PDN details if any are known.
// A nil Network means the MME has no record of this IMSI at all (never
// attached, or the core network was unreachable when we asked) -- distinct
// from "idle", which is a UE that's attached with a live PDN session but no
// active radio connection right now (normal LTE power-saving behaviour).
func networkLines(n *api.NetworkInfo) []string {
	if n == nil {
		return []string{"not registered"}
	}
	lines := []string{joinNonEmpty(" · ", n.CMState, n.MMState)}
	var detail []string
	if n.CellID != 0 {
		detail = append(detail, fmt.Sprintf("cell %d (enb %d)", n.CellID, n.ENBID))
	}
	if n.TAC != 0 {
		detail = append(detail, fmt.Sprintf("TAC %d", n.TAC))
	}
	if n.APN != "" {
		pdn := "PDN " + n.APN
		if n.QCI != 0 {
			pdn += fmt.Sprintf(" · QCI %d", n.QCI)
		}
		if n.PDUState != "" {
			pdn += " · " + n.PDUState
		}
		detail = append(detail, pdn)
	}
	if len(detail) > 0 {
		lines = append(lines, strings.Join(detail, " · "))
	}
	return lines
}

// networkSummary is the one-cell version of networkLines, for the list table.
func networkSummary(n *api.NetworkInfo) string {
	if n == nil {
		return "-"
	}
	s := n.CMState
	if n.CellID != 0 {
		s += fmt.Sprintf(" (cell %d)", n.CellID)
	}
	return s
}

func lastSummary(v api.SubscriberView) string {
	if v.LastAction == "" {
		return "-"
	}
	return v.LastAction + "/" + v.LastReason
}

func joinNonEmpty(sep string, parts ...string) string {
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, sep)
}
