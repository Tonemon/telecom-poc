package subscriber

import "testing"

func TestParseAMBR(t *testing.T) {
	cases := map[string]AMBR{
		"1000000000": {1000000000, 0},
		"100M":       {100, 2},
		"50M":        {50, 2},
		"1G":         {1, 3},
		"512K":       {512, 1},
	}
	for in, want := range cases {
		got, err := ParseAMBR(in)
		if err != nil || got != want {
			t.Errorf("ParseAMBR(%q)=%v,%v want %v", in, got, err, want)
		}
	}
	if _, err := ParseAMBR("bogus"); err == nil {
		t.Error("expected error for bogus AMBR")
	}
}

func TestDocument_EnforcedFields(t *testing.T) {
	s := Subscriber{
		IMSI: "999700000000001",
		K:    "465B5CE8B199B49FAA5F0A2EE238A6BC",
		OPc:  "E8ED289DEBA952E4283B54E88E6183CA",
		AMF:  "8000",
		APN:  "internet",
		DL:   AMBR{1, 3}, UL: AMBR{1, 3},
	}
	doc := s.Document()
	if doc["imsi"] != "999700000000001" {
		t.Fatalf("imsi = %v", doc["imsi"])
	}
	if doc["subscriber_status"].(int) != 0 {
		t.Fatalf("subscriber_status must default to 0 (granted)")
	}
	sec := doc["security"].(map[string]any)
	if sec["k"] != s.K || sec["opc"] != s.OPc || sec["amf"] != "8000" || sec["op"] != nil {
		t.Fatalf("security block wrong: %v", sec)
	}
	if doc["access_restriction_data"] != 32 || doc["network_access_mode"] != 0 {
		t.Fatalf("restriction/access-mode wrong")
	}
}
