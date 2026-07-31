package main

import (
	"encoding/json"
	"fmt"
	"os"

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
  telcoctl list
  telcoctl get         <imsi>
  telcoctl history     <imsi>
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
			printJSON(v)
		}
	case "get":
		var v api.SubscriberView
		v, err = c.Get(arg0(args))
		if err == nil {
			printJSON(v)
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
