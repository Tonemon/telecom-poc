package inventory

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const repoConf = "../../../../deploy/4g/config/multi"

// needRepo skips tests that read the repo's configs when they are not there (the image build
// only has tools/agent in its context).
func needRepo(t *testing.T) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(repoConf, "instance.json")); err != nil {
		t.Skip("repo configs not available")
	}
}

func TestLoadRepoConfig(t *testing.T) {
	needRepo(t)
	inv, err := Load(repoConf)
	if err != nil {
		t.Fatal(err)
	}
	if inv.Name != "telecom-poc" || inv.Operator.Name != "Telecom PoC" || inv.Operator.PLMN != "999-70" {
		t.Errorf("identity: %+v", inv)
	}
	if len(inv.Cells) != 2 || len(inv.Devices) != 3 {
		t.Fatalf("got %d cells, %d devices", len(inv.Cells), len(inv.Devices))
	}
	a := inv.Cells[0]
	want := Cell{
		ID: "A", Service: "enb-a", Generation: "4G", Software: "srsRAN 4G 23.11",
		ENBID: 0x19B, CellID: 1, ECI: 0x19B01, PCI: 1, TAC: 1, EARFCN: 3350, PRB: 25,
		Band: 7, DLMHz: 2680, BandwidthMHz: 5, PLMN: "999-70",
		Neighbours: []Neighbour{{ECI: 0x19C02, PCI: 2, EARFCN: 3350}},
		A3:         A3{OffsetDB: 2, HysteresisDB: 1, TimeToTriggerMS: 640},
	}
	if !reflect.DeepEqual(a, want) {
		t.Errorf("cell A:\n got %+v\nwant %+v", a, want)
	}
	if inv.Cells[1].ECI != 0x19C02 || inv.Cells[1].Neighbours[0].ECI != 0x19B01 {
		t.Errorf("cell B: %+v", inv.Cells[1])
	}
	ue2 := inv.Devices[1]
	wantUE := Device{ID: "ue2", Service: "ue2", Model: "Google Pixel 8", Generations: []string{"4G"},
		IMSI: "999700000000002", HomePLMN: "999-70", APN: "internet"}
	if !reflect.DeepEqual(ue2, wantUE) {
		t.Errorf("ue2:\n got %+v\nwant %+v", ue2, wantUE)
	}
}

func TestLoadMissingConfIsError(t *testing.T) {
	needRepo(t)
	dir := t.TempDir()
	for _, f := range []string{"enb-a.conf", "enb-b.conf", "rr-a.conf", "rr-b.conf", "ue1.conf", "ue2.conf"} {
		b, err := os.ReadFile(filepath.Join(repoConf, f))
		if err != nil {
			t.Fatal(err)
		}
		os.WriteFile(filepath.Join(dir, f), b, 0o644)
	}
	b, _ := os.ReadFile(filepath.Join(repoConf, "instance.json"))
	os.WriteFile(filepath.Join(dir, "instance.json"), b, 0o644)
	_, err := Load(dir) // ue3.conf missing
	if err == nil || !strings.Contains(err.Error(), "ue3.conf") {
		t.Fatalf("want an error naming ue3.conf, got %v", err)
	}
}

func TestLoadMissingInstanceJSON(t *testing.T) {
	if _, err := Load(t.TempDir()); err == nil || !strings.Contains(err.Error(), "instance.json") {
		t.Fatalf("want an error naming instance.json, got %v", err)
	}
}
