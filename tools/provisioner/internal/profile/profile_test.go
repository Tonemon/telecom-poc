package profile

import (
	"strings"
	"testing"

	"github.com/telecom-poc/provisioner/internal/subscriber"
)

func sampleSub() subscriber.Subscriber {
	return subscriber.Subscriber{
		IMSI: "999700000000001",
		K:    "465B5CE8B199B49FAA5F0A2EE238A6BC",
		OPc:  "E8ED289DEBA952E4283B54E88E6183CA",
		AMF:  "8000", APN: "internet",
	}
}

func TestUsimBlock_ContainsKeys(t *testing.T) {
	p := FromSubscriber(sampleSub())
	block := p.UsimBlock()
	for _, want := range []string{"algo = milenage", "opc  = E8ED289DEBA952E4283B54E88E6183CA", "k    = 465B5CE8B199B49FAA5F0A2EE238A6BC", "imsi = 999700000000001"} {
		if !strings.Contains(block, want) {
			t.Fatalf("usim block missing %q:\n%s", want, block)
		}
	}
}

func TestValidate_DetectsMismatch(t *testing.T) {
	s := sampleSub()
	p := FromSubscriber(s)
	if err := p.Validate(s); err != nil {
		t.Fatalf("expected valid, got %v", err)
	}
	bad := p
	bad.OPc = "00000000000000000000000000000000"
	if err := bad.Validate(s); err == nil {
		t.Fatal("expected mismatch error when OPc differs")
	}
}
