// Package inventory describes an instance's towers and devices: static facts read from the
// srsRAN configs the stack already uses, plus instance.json for what configs cannot say
// (operator name, device models, supported generations).
package inventory

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Software is what every srsRAN-based tower and device runs.
const Software = "srsRAN 4G 23.11"

type Inventory struct {
	Name     string   `json:"name"`
	Operator Operator `json:"operator"`
	Cells    []Cell   `json:"cells"`
	Devices  []Device `json:"devices"`
}

type Operator struct {
	Name string `json:"name"`
	PLMN string `json:"plmn"`
}

type Cell struct {
	ID           string      `json:"id"`
	Service      string      `json:"service"`
	Generation   string      `json:"generation"`
	Software     string      `json:"software"`
	ENBID        int         `json:"enb_id"`
	CellID       int         `json:"cell_id"`
	ECI          int         `json:"eci"`
	PCI          int         `json:"pci"`
	TAC          int         `json:"tac"`
	EARFCN       int         `json:"earfcn"`
	PRB          int         `json:"prb"`
	Band         int         `json:"band"`
	DLMHz        float64     `json:"dl_mhz"`
	BandwidthMHz float64     `json:"bandwidth_mhz"`
	PLMN         string      `json:"plmn"`
	Neighbours   []Neighbour `json:"neighbours"`
	A3           A3          `json:"a3"`
}

type Neighbour struct {
	ECI    int `json:"eci"`
	PCI    int `json:"pci"`
	EARFCN int `json:"earfcn"`
}

// A3 is the handover trigger: neighbour better than serving by OffsetDB + HysteresisDB
// for TimeToTriggerMS (radio time).
type A3 struct {
	OffsetDB        float64 `json:"offset_db"`
	HysteresisDB    float64 `json:"hysteresis_db"`
	TimeToTriggerMS int     `json:"time_to_trigger_ms"`
}

type Device struct {
	ID          string   `json:"id"`
	Service     string   `json:"service"`
	Model       string   `json:"model"`
	Generations []string `json:"generations"`
	IMSI        string   `json:"imsi"`
	HomePLMN    string   `json:"home_plmn"`
	APN         string   `json:"apn"`
}

type instanceFile struct {
	Name     string   `json:"name"`
	Operator Operator `json:"operator"`
	Cells    []struct {
		ID, Service, Generation string
		ENBConf                 string `json:"enb_conf"`
		RRConf                  string `json:"rr_conf"`
	} `json:"cells"`
	Devices []struct {
		ID, Service, Model, Conf string
		Generations              []string
	} `json:"devices"`
}

// Load reads instance.json and every config it names from confDir.
func Load(confDir string) (Inventory, error) {
	var inst instanceFile
	raw, err := os.ReadFile(filepath.Join(confDir, "instance.json"))
	if err != nil {
		return Inventory{}, fmt.Errorf("instance.json: %w", err)
	}
	if err := json.Unmarshal(raw, &inst); err != nil {
		return Inventory{}, fmt.Errorf("instance.json: %w", err)
	}
	inv := Inventory{Name: inst.Name, Operator: inst.Operator}
	for _, c := range inst.Cells {
		enb, err := readConf(confDir, c.ENBConf)
		if err != nil {
			return Inventory{}, err
		}
		rr, err := readConf(confDir, c.RRConf)
		if err != nil {
			return Inventory{}, err
		}
		cell := parseCell(enb, rr)
		cell.ID, cell.Service, cell.Generation, cell.Software = c.ID, c.Service, c.Generation, Software
		inv.Cells = append(inv.Cells, cell)
	}
	for _, d := range inst.Devices {
		conf, err := readConf(confDir, d.Conf)
		if err != nil {
			return Inventory{}, err
		}
		imsi := value(conf, "imsi")
		home := ""
		if len(imsi) >= 5 {
			home = imsi[:3] + "-" + imsi[3:5]
		}
		inv.Devices = append(inv.Devices, Device{ID: d.ID, Service: d.Service, Model: d.Model,
			Generations: d.Generations, IMSI: imsi, HomePLMN: home, APN: value(conf, "apn")})
	}
	return inv, nil
}

func readConf(dir, name string) (string, error) {
	b, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return "", fmt.Errorf("%s: %w", name, err)
	}
	return stripComments(string(b)), nil
}

var comment = regexp.MustCompile(`(?m)(//.*$|^\s*#.*$)`)

func stripComments(s string) string { return comment.ReplaceAllString(s, "") }

// value is the first `key = value` (ini or libconfig style) in s, without a trailing ';'.
func value(s, key string) string {
	m := regexp.MustCompile(`(?m)^\s*` + regexp.QuoteMeta(key) + `\s*=\s*"?([^\s;"]+)`).FindStringSubmatch(s)
	if m == nil {
		return ""
	}
	return m[1]
}

func intValue(s, key string) int {
	n, _ := strconv.ParseInt(value(s, key), 0, 64)
	return int(n)
}

var block = regexp.MustCompile(`\{([^{}]*)\}`)

func parseCell(enb, rr string) Cell {
	head, rest, _ := strings.Cut(rr, "meas_cell_list")
	meas, report, _ := strings.Cut(rest, "meas_report_desc")
	c := Cell{
		ENBID: intValue(enb, "enb_id"), PRB: intValue(enb, "n_prb"),
		PLMN:   value(enb, "mcc") + "-" + value(enb, "mnc"),
		CellID: intValue(head, "cell_id"), TAC: intValue(head, "tac"),
		PCI: intValue(head, "pci"), EARFCN: intValue(head, "dl_earfcn"),
		A3: A3{OffsetDB: float64(intValue(report, "a3_offset")) / 2,
			HysteresisDB: float64(intValue(report, "hysteresis")) / 2, TimeToTriggerMS: intValue(report, "time_to_trigger")},
	}
	c.ECI = c.ENBID<<8 | c.CellID
	c.Band, c.DLMHz, _ = Band(c.EARFCN)
	c.BandwidthMHz = BandwidthMHz(c.PRB)
	for _, m := range block.FindAllStringSubmatch(meas, -1) {
		if value(m[1], "eci") == "" {
			continue
		}
		c.Neighbours = append(c.Neighbours, Neighbour{ECI: intValue(m[1], "eci"), PCI: intValue(m[1], "pci"),
			EARFCN: intValue(m[1], "dl_earfcn")})
	}
	return c
}
