package subscriber

import (
	"fmt"
	"strconv"
	"strings"

	"go.mongodb.org/mongo-driver/bson"
)

// AMBR value + unit (0=bps,1=Kbps,2=Mbps,3=Gbps,4=Tbps) per open5gs-dbctl.
type AMBR struct {
	Value int
	Unit  int
}

func (a AMBR) doc() bson.M {
	return bson.M{"value": a.Value, "unit": a.Unit}
}

var ambrUnits = map[byte]int{'K': 1, 'M': 2, 'G': 3, 'T': 4}

// ParseAMBR parses "100M"/"1G"/"512K" or a raw bps integer into an AMBR.
func ParseAMBR(s string) (AMBR, error) {
	s = strings.TrimSpace(strings.ToUpper(s))
	if s == "" {
		return AMBR{}, fmt.Errorf("empty AMBR")
	}
	last := s[len(s)-1]
	if unit, ok := ambrUnits[last]; ok {
		v, err := strconv.Atoi(s[:len(s)-1])
		if err != nil {
			return AMBR{}, fmt.Errorf("bad AMBR %q: %w", s, err)
		}
		return AMBR{Value: v, Unit: unit}, nil
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return AMBR{}, fmt.Errorf("bad AMBR %q: %w", s, err)
	}
	return AMBR{Value: v, Unit: 0}, nil
}

type Subscriber struct {
	IMSI       string
	K          string
	OPc        string
	AMF        string
	APN        string
	DL, UL     AMBR
	StaticIPv4 string // "" = dynamic
}

// Document returns the exact Open5GS subscriber insert document.
func (s Subscriber) Document() bson.M {
	amf := s.AMF
	if amf == "" {
		amf = "8000"
	}
	session := bson.M{
		"name": s.APN,
		"type": 3,
		"qos": bson.M{
			"index": 9,
			"arp": bson.M{
				"priority_level":            8,
				"pre_emption_capability":    1,
				"pre_emption_vulnerability": 2,
			},
		},
		"ambr":     bson.M{"downlink": s.DL.doc(), "uplink": s.UL.doc()},
		"pcc_rule": bson.A{},
	}
	if s.StaticIPv4 != "" {
		session["ue"] = bson.M{"ipv4": s.StaticIPv4}
	}
	return bson.M{
		"schema_version": 1,
		"imsi":           s.IMSI,
		"msisdn":         bson.A{},
		"imeisv":         bson.A{},
		"mme_host":       bson.A{},
		"mm_realm":       bson.A{},
		"purge_flag":     bson.A{},
		"slice": bson.A{bson.M{
			"sst":               1,
			"default_indicator": true,
			"session":           bson.A{session},
		}},
		"security": map[string]any{
			"k":   s.K,
			"op":  nil,
			"opc": s.OPc,
			"amf": amf,
		},
		"ambr":                        bson.M{"downlink": s.DL.doc(), "uplink": s.UL.doc()},
		"access_restriction_data":     32,
		"network_access_mode":         0,
		"subscriber_status":           0,
		"operator_determined_barring": 0,
		"subscribed_rau_tau_timer":    12,
		"__v":                         0,
	}
}

// DefaultAMBR is the plan applied when the caller does not specify one (1 Gbps up/down).
func DefaultAMBR() (AMBR, AMBR) { return AMBR{1, 3}, AMBR{1, 3} }
