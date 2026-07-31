package profile

import (
	"fmt"
	"strings"

	"github.com/telecom-poc/provisioner/internal/subscriber"
)

const defaultIMEI = "353490069873319"

type UEProfile struct {
	IMSI string
	K    string
	OPc  string
	IMEI string
}

func FromSubscriber(s subscriber.Subscriber) UEProfile {
	return UEProfile{IMSI: s.IMSI, K: s.K, OPc: s.OPc, IMEI: defaultIMEI}
}

// UsimBlock renders the srsue [usim] section (matches deploy/4g/config/ue.conf).
func (p UEProfile) UsimBlock() string {
	return strings.Join([]string{
		"[usim]",
		"mode = soft",
		"algo = milenage",
		"opc  = " + p.OPc,
		"k    = " + p.K,
		"imsi = " + p.IMSI,
		"imei = " + p.IMEI,
	}, "\n") + "\n"
}

// Validate is the fail-loud guarantee: the profile must match the subscriber
// the server just wrote (same keys + IMSI), preventing mismatched-key auth reject.
func (p UEProfile) Validate(s subscriber.Subscriber) error {
	if !strings.EqualFold(p.K, s.K) {
		return fmt.Errorf("profile K does not match subscriber K")
	}
	if !strings.EqualFold(p.OPc, s.OPc) {
		return fmt.Errorf("profile OPc does not match subscriber OPc")
	}
	if p.IMSI != s.IMSI {
		return fmt.Errorf("profile IMSI does not match subscriber IMSI")
	}
	return nil
}
