package actions

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/telecom-poc/agent/internal/docker"
)

const epc = "telecom-poc-4g_epc"

func powerActions(t *testing.T, f *fakeDocker) *Actions {
	t.Helper()
	return newActions(t, f).WithPower(epc, map[string]string{"A": "172.22.0.40"})
}

func stopped(service string) docker.Container {
	c := running(service)
	c.State = "exited"
	return c
}

func TestPowerSwitchOffStopsTheUE(t *testing.T) {
	f := &fakeDocker{containers: []docker.Container{running("ue2")}}
	r, err := powerActions(t, f).Power(context.Background(), "ue2", "switch_off")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(f.calls, []string{"stop id-ue2 15"}) {
		t.Errorf("calls %v", f.calls)
	}
	if r.Command != Compose+" stop ue2" || r.State != "off" || r.Action != "switch_off" || r.Service != "ue2" {
		t.Errorf("result %+v", r)
	}
}

func TestPowerPullBatteryKills(t *testing.T) {
	f := &fakeDocker{containers: []docker.Container{running("ue2")}}
	r, err := powerActions(t, f).Power(context.Background(), "ue2", "pull_battery")
	if err != nil || !reflect.DeepEqual(f.calls, []string{"kill id-ue2"}) || r.Command != Compose+" kill ue2" {
		t.Fatalf("%+v %v %v", r, f.calls, err)
	}
}

func TestPowerSwitchOnStarts(t *testing.T) {
	f := &fakeDocker{containers: []docker.Container{stopped("ue2")}}
	r, err := powerActions(t, f).Power(context.Background(), "ue2", "switch_on")
	if err != nil || !reflect.DeepEqual(f.calls, []string{"start id-ue2"}) || r.Command != Compose+" start ue2" || r.State != "on" {
		t.Fatalf("%+v %v %v", r, f.calls, err)
	}
}

func TestPowerShutDownStopsTheTower(t *testing.T) {
	f := &fakeDocker{containers: []docker.Container{running("enb-a")}}
	r, err := powerActions(t, f).Power(context.Background(), "enb-a", "shut_down")
	if err != nil || !reflect.DeepEqual(f.calls, []string{"stop id-enb-a 15"}) || r.Command != Compose+" stop enb-a" {
		t.Fatalf("%+v %v %v", r, f.calls, err)
	}
}

func TestPowerCutDisconnectsBeforeKill(t *testing.T) {
	f := &fakeDocker{containers: []docker.Container{running("enb-a")}}
	r, err := powerActions(t, f).Power(context.Background(), "enb-a", "power_cut")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(f.calls, []string{"disconnect " + epc + " id-enb-a", "kill id-enb-a"}) {
		t.Errorf("calls %v", f.calls)
	}
	want := "docker network disconnect -f " + epc + " telecom-poc-4g-enb-a-1 && " + Compose + " kill enb-a"
	if r.Command != want {
		t.Errorf("command %q, want %q", r.Command, want)
	}
}

func TestPowerOnReconnectsWithFixedIP(t *testing.T) {
	f := &fakeDocker{containers: []docker.Container{stopped("enb-a")}, networks: map[string]map[string]string{}}
	r, err := powerActions(t, f).Power(context.Background(), "enb-a", "power_on")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(f.calls, []string{"connect " + epc + " id-enb-a 172.22.0.40", "start id-enb-a"}) {
		t.Errorf("calls %v", f.calls)
	}
	want := "docker network connect --ip 172.22.0.40 " + epc + " telecom-poc-4g-enb-a-1 && " + Compose + " start enb-a"
	if r.Command != want {
		t.Errorf("command %q, want %q", r.Command, want)
	}
}

func TestPowerOnAfterShutDownSkipsConnect(t *testing.T) {
	f := &fakeDocker{containers: []docker.Container{stopped("enb-a")},
		networks: map[string]map[string]string{"id-enb-a": {epc: "172.22.0.40"}}}
	r, err := powerActions(t, f).Power(context.Background(), "enb-a", "power_on")
	if err != nil || !reflect.DeepEqual(f.calls, []string{"start id-enb-a"}) || strings.Contains(r.Command, "network") {
		t.Fatalf("%+v %v %v", r, f.calls, err)
	}
}

func TestPowerRejectsWrongKindAndAlreadyInState(t *testing.T) {
	f := &fakeDocker{containers: []docker.Container{running("ue2"), running("enb-a")}}
	a := powerActions(t, f)
	ctx := context.Background()
	for _, c := range []struct{ svc, action string }{{"ue2", "power_cut"}, {"enb-a", "pull_battery"}, {"ue2", "explode"}, {"upf", "switch_off"}} {
		if _, err := a.Power(ctx, c.svc, c.action); !errors.Is(err, ErrUnknown) {
			t.Errorf("%s %s: %v", c.svc, c.action, err)
		}
	}
	if _, err := a.Power(ctx, "ue2", "switch_on"); !errors.Is(err, ErrState) || !strings.Contains(err.Error(), "ue2 is already on") {
		t.Errorf("switch_on running: %v", err)
	}
	f.containers = []docker.Container{stopped("ue2")}
	a.compAt = a.compAt.Add(-1e12)
	if _, err := a.Power(ctx, "ue2", "pull_battery"); !errors.Is(err, ErrState) || !strings.Contains(err.Error(), "ue2 is already off") {
		t.Errorf("pull_battery stopped: %v", err)
	}
	if len(f.calls) != 0 {
		t.Errorf("no Docker call expected, got %v", f.calls)
	}
}

func TestTowerAddrsFromAirConfig(t *testing.T) {
	got, err := TowerAddrs([]byte(`{"cells": [{"id": "A", "dl": "tcp://172.22.0.40:2000"}, {"id": "B", "dl": "tcp://172.22.0.50:2000"}]}`))
	if err != nil || !reflect.DeepEqual(got, map[string]string{"A": "172.22.0.40", "B": "172.22.0.50"}) {
		t.Fatalf("%v %v", got, err)
	}
	if _, err := TowerAddrs([]byte(`{"cells": [{"id": "A", "dl": "tcp://*:2000"}]}`)); err == nil {
		t.Error("want an error for an address that is not an IPv4")
	}
}

const mmeLog = "\x1b[32m11/20 12:29:05.350\x1b[0m: [\x1b[33memm\x1b[0m] \x1b[1;32mINFO\x1b[0m: [999700000000002] Attach request (../src/mme/emm-sm.c:488)\n" +
	"11/20 12:30:00.000: [emm] INFO: [999700000000003] Attach complete (../src/mme/emm-sm.c:1603)\n" +
	"11/20 12:35:00.000: [emm] INFO: [999700000000002] Mobile Reachable timer expired (../src/mme/emm-sm.c:211)\n" +
	"11/20 12:35:01.000: [emm] INFO: [999700000000003] Mobile Reachable timer expired (../src/mme/emm-sm.c:211)\n" +
	"11/20 12:36:00.000: [emm] INFO: [999700000000003] Attach request (../src/mme/emm-sm.c:488)\n" +
	"11/20 12:36:00.000: [mme] INFO: Removed Session: UE IMSI:[999700000000003] APN:[internet]\n"

func TestUnreachableFromLog(t *testing.T) {
	if got := unreachableFrom(mmeLog); !reflect.DeepEqual(got, []string{"999700000000002"}) {
		t.Fatalf("got %v", got)
	}
	if got := unreachableFrom(""); len(got) != 0 {
		t.Fatalf("empty log: %v", got)
	}
}

func TestUnreachableReadsTheMMELog(t *testing.T) {
	f := &fakeDocker{containers: []docker.Container{running("mme")}, logs: mmeLog}
	got, err := powerActions(t, f).Unreachable(context.Background())
	if err != nil || !reflect.DeepEqual(got, []string{"999700000000002"}) {
		t.Fatalf("%v %v", got, err)
	}
}
