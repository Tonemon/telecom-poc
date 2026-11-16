// Package proxy passes the page's requests on to an instance's air, agent and provisioner,
// adding their tokens, but only for a fixed allow-list of routes. The browser never sees a token.
package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"regexp"
	"strings"

	"github.com/telecom-poc/map/internal/config"
)

type route struct {
	method  string
	pattern *regexp.Regexp
	query   []string // query keys passed on; everything else is dropped
}

func r(method, pattern string, query ...string) route {
	return route{method, regexp.MustCompile("^" + pattern + "$"), query}
}

const id = `[A-Za-z0-9_-]+`

var allow = map[string][]route{
	"air": {
		r("GET", "/world"),
		r("POST", "/ues/"+id+"/travel"),
		r("PUT", "/ues/"+id+"/position"),
		r("PUT", "/ues/"+id+"/shielded"),
		r("POST", "/ues/"+id+"/stop"),
		r("PUT", "/cells/"+id+"/position", "dry_run"),
		r("POST", "/activity/random"),
		r("PUT", "/activity/realtime"),
	},
	"agent": {
		r("GET", "/inventory"),
		r("GET", "/components"),
		r("POST", "/pcaps"),
		r("DELETE", "/pcaps/"+id),
		r("GET", "/pcaps"),
		r("GET", `/pcaps/[A-Za-z0-9_.-]+\.pcap`),
		r("PUT", "/ues/"+id+"/ping"),
		r("GET", "/ues/"+id+"/ping", "since"),
		r("POST", "/ues/"+id+"/speedtest"),
	},
	"provisioner": {
		r("GET", "/subscribers"),
		r("POST", `/subscribers/\d+/suspend`),
		r("POST", `/subscribers/\d+/resume`),
		r("PATCH", `/subscribers/\d+/plan`),
	},
}

type ctxKey struct{}

// New serves GET /api/instances and /api/instances/{name}/{air|agent|provisioner}/{path...}.
func New(instances []config.Instance) http.Handler {
	byName := map[string]config.Instance{}
	var names []map[string]string
	for _, in := range instances {
		byName[in.Name] = in
		names = append(names, map[string]string{"name": in.Name})
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/instances", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, 200, names) })
	mux.HandleFunc("/api/instances/{name}/{service}/{rest...}", func(w http.ResponseWriter, req *http.Request) {
		in, ok := byName[req.PathValue("name")]
		if !ok {
			writeJSON(w, 404, map[string]string{"error": "unknown instance"})
			return
		}
		service, rest := req.PathValue("service"), "/"+req.PathValue("rest")
		rt, ok := allowed(service, req.Method, rest, req.URL)
		if !ok {
			writeJSON(w, 403, map[string]string{"error": "not allowed"})
			return
		}
		ep := map[string]config.Endpoint{"air": in.Air, "agent": in.Agent, "provisioner": in.Provisioner}[service]
		target, _ := url.Parse(ep.URL)
		q := url.Values{}
		for _, k := range rt.query {
			if v := req.URL.Query().Get(k); v != "" {
				q.Set(k, v)
			}
		}
		body, _ := io.ReadAll(io.LimitReader(req.Body, 1<<20))
		cmd := ""
		if service == "provisioner" {
			cmd = TelcoctlCommand(req.Method, rest, body)
		}
		p := &httputil.ReverseProxy{
			Rewrite: func(pr *httputil.ProxyRequest) {
				pr.Out.URL.Scheme, pr.Out.URL.Host = target.Scheme, target.Host
				pr.Out.URL.Path, pr.Out.URL.RawPath, pr.Out.URL.RawQuery = strings.TrimSuffix(target.Path, "/")+rest, "", q.Encode()
				pr.Out.Host = target.Host
				pr.Out.Header.Set("Authorization", "Bearer "+ep.Token)
				pr.Out.Header.Del("Cookie")
				pr.Out.Body = io.NopCloser(bytes.NewReader(body))
				pr.Out.ContentLength = int64(len(body))
			},
			ModifyResponse: func(resp *http.Response) error {
				if c, _ := resp.Request.Context().Value(ctxKey{}).(string); c != "" && resp.StatusCode < 300 {
					resp.Header.Set("X-Command", c)
				}
				return nil
			},
			ErrorHandler: func(w http.ResponseWriter, _ *http.Request, err error) {
				writeJSON(w, 502, map[string]string{"error": service + " unreachable: " + err.Error()})
			},
		}
		p.ServeHTTP(w, req.WithContext(context.WithValue(req.Context(), ctxKey{}, cmd)))
	})
	mux.HandleFunc("/api/", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 403, map[string]string{"error": "not allowed"})
	})
	return mux
}

// allowed checks the route against the allow-list. Encoded slashes and paths that change under
// cleaning ("..", "//") never match.
func allowed(service, method, rest string, u *url.URL) (route, bool) {
	if u.RawPath != "" || strings.Contains(rest, "..") || strings.Contains(rest, "//") {
		return route{}, false
	}
	for _, rt := range allow[service] {
		if rt.method == method && rt.pattern.MatchString(rest) {
			return rt, true
		}
	}
	return route{}, false
}

var subscriberAction = regexp.MustCompile(`^/subscribers/(\d+)/(suspend|resume|plan)$`)

// TelcoctlCommand is the telcoctl command that does the same as a provisioner request ("" if none).
func TelcoctlCommand(method, path string, body []byte) string {
	m := subscriberAction.FindStringSubmatch(path)
	if m == nil || method == "GET" {
		return ""
	}
	var b struct{ Reason, Note, DL, UL string }
	json.Unmarshal(body, &b)
	verb := map[string]string{"suspend": "suspend", "resume": "resume", "plan": "set-plan"}[m[2]]
	cmd := "./bin/telcoctl " + verb + " " + m[1]
	if verb == "set-plan" {
		cmd += " --dl " + b.DL + " --ul " + b.UL
	}
	cmd += " --reason " + b.Reason
	if b.Note != "" {
		cmd += ` --note "` + strings.ReplaceAll(b.Note, `"`, `\"`) + `"`
	}
	return cmd
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
