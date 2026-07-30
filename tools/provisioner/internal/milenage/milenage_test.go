package milenage

import (
	"bytes"
	"encoding/hex"
	"testing"
)

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatalf("bad hex %q: %v", s, err)
	}
	return b
}

// 3GPP TS 35.208 §4.3, Test Set 1 — authoritative known-answer vectors.
var ts1 = struct{ k, op, opc, rand, sqn, amf, macA, macS, res, ck, ik, ak string }{
	k:    "465b5ce8b199b49faa5f0a2ee238a6bc",
	op:   "cdc202d5123e20f62b6d676ac72cb318",
	opc:  "cd63cb71954a9f4e48a5994e37a02baf",
	rand: "23553cbe9637a89d218ae64dae47bf35",
	sqn:  "ff9bb4d0b607",
	amf:  "b9b9",
	macA: "4a9ffac354dfafb3",
	macS: "01cfaf9ec4e871e9",
	res:  "a54211d5e3ba50bf",
	ck:   "b40ba9a3c58b2a05bbf0d987b21bf8cb",
	ik:   "f769bcd751044604127672711c6d3441",
}

func TestComputeOPc_TS1(t *testing.T) {
	got, err := ComputeOPc(mustHex(t, ts1.k), mustHex(t, ts1.op))
	if err != nil {
		t.Fatal(err)
	}
	if want := mustHex(t, ts1.opc); !bytes.Equal(got, want) {
		t.Fatalf("OPc = %x, want %x", got, want)
	}
}

func TestF1_TS1(t *testing.T) {
	macA, macS, err := F1(mustHex(t, ts1.k), mustHex(t, ts1.opc), mustHex(t, ts1.rand), mustHex(t, ts1.sqn), mustHex(t, ts1.amf))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(macA, mustHex(t, ts1.macA)) {
		t.Fatalf("MAC-A = %x, want %s", macA, ts1.macA)
	}
	if !bytes.Equal(macS, mustHex(t, ts1.macS)) {
		t.Fatalf("MAC-S = %x, want %s", macS, ts1.macS)
	}
}

func TestF2345_TS1(t *testing.T) {
	res, ck, ik, ak, err := F2345(mustHex(t, ts1.k), mustHex(t, ts1.opc), mustHex(t, ts1.rand))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(res, mustHex(t, ts1.res)) {
		t.Fatalf("RES = %x, want %s", res, ts1.res)
	}
	if !bytes.Equal(ck, mustHex(t, ts1.ck)) {
		t.Fatalf("CK = %x, want %s", ck, ts1.ck)
	}
	if !bytes.Equal(ik, mustHex(t, ts1.ik)) {
		t.Fatalf("IK = %x", ik)
	}
	_ = ak
}
