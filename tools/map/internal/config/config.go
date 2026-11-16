// Package config reads the map's list of telecom-poc instances (instances.json).
package config

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"regexp"
)

type Endpoint struct {
	URL   string `json:"url"`
	Token string `json:"token"`
}

// Instance is one telecom-poc network the map shows: where its APIs are and their tokens.
type Instance struct {
	Name        string   `json:"name"`
	Air         Endpoint `json:"air"`
	Agent       Endpoint `json:"agent"`
	Provisioner Endpoint `json:"provisioner"`
}

var validName = regexp.MustCompile(`^[a-z0-9-]+$`)

func Load(path string) ([]Instance, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var list []Instance
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if len(list) == 0 {
		return nil, fmt.Errorf("%s: no instances", path)
	}
	seen := map[string]bool{}
	for _, in := range list {
		if !validName.MatchString(in.Name) {
			return nil, fmt.Errorf("instance name %q: use lowercase letters, digits and -", in.Name)
		}
		if seen[in.Name] {
			return nil, fmt.Errorf("duplicate instance name %q", in.Name)
		}
		seen[in.Name] = true
		for svc, ep := range map[string]Endpoint{"air": in.Air, "agent": in.Agent, "provisioner": in.Provisioner} {
			u, err := url.Parse(ep.URL)
			if err != nil || u.Scheme != "http" && u.Scheme != "https" || u.Host == "" {
				return nil, fmt.Errorf("instance %s: %s url %q must be an absolute http(s) URL", in.Name, svc, ep.URL)
			}
		}
	}
	return list, nil
}
