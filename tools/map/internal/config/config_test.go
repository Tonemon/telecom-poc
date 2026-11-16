package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "instances.json")
	os.WriteFile(p, []byte(body), 0o644)
	return p
}

const one = `{"name": "%s", "air": {"url": "%s", "token": "a"}, "agent": {"url": "http://172.22.0.42:8091", "token": "b"},
 "provisioner": {"url": "http://172.22.0.30:8080", "token": "c"}}`

func inst(name, air string) string {
	return strings.Replace(strings.Replace(one, "%s", name, 1), "%s", air, 1)
}

func TestLoadRepoInstances(t *testing.T) {
	const repo = "../../../../deploy/map/instances.json"
	if _, err := os.Stat(repo); err != nil {
		t.Skip("repo config not available (image build)")
	}
	list, err := Load(repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Name != "telecom-poc" || list[0].Air.URL != "http://172.22.0.41:8090" ||
		list[0].Agent.Token != "dev-agent-token" || list[0].Provisioner.URL != "http://172.22.0.30:8080" {
		t.Fatalf("got %+v", list)
	}
}

func TestConfigRejectsDuplicateNames(t *testing.T) {
	_, err := Load(write(t, "["+inst("x", "http://a:1")+","+inst("x", "http://b:1")+"]"))
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("got %v", err)
	}
}

func TestConfigRejectsRelativeURL(t *testing.T) {
	_, err := Load(write(t, "["+inst("x", "/air")+"]"))
	if err == nil || !strings.Contains(err.Error(), "air") {
		t.Fatalf("got %v", err)
	}
}

func TestConfigRejectsBadName(t *testing.T) {
	for _, name := range []string{"", "Big", "a b", "../x"} {
		if _, err := Load(write(t, "["+inst(name, "http://a:1")+"]")); err == nil {
			t.Errorf("name %q accepted", name)
		}
	}
}

func TestConfigRejectsEmptyList(t *testing.T) {
	if _, err := Load(write(t, "[]")); err == nil {
		t.Fatal("empty list accepted")
	}
}
