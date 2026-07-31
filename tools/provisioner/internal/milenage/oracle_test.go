package milenage

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"testing"

	wm "github.com/wmnsk/milenage"
)

func TestComputeOPc_MatchesWmnskOracle(t *testing.T) {
	for i := 0; i < 100; i++ {
		k := make([]byte, 16)
		op := make([]byte, 16)
		rand.Read(k)
		rand.Read(op)

		ours, err := ComputeOPc(k, op)
		if err != nil {
			t.Fatal(err)
		}
		ref, err := wm.ComputeOPc(k, op)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(ours, ref) {
			t.Fatalf("iter %d: ours=%x oracle=%x", i, ours, ref)
		}
	}
}

// TestF2345_MatchesOracle arbitrates our f2-f5 against the trusted library
// over TS1 inputs (independent of any hardcoded expected literal).
func TestF2345_MatchesOracle(t *testing.T) {
	k := mustHex(t, ts1.k)
	opc := mustHex(t, ts1.opc)
	rnd := mustHex(t, ts1.rand)
	sqn := binary.BigEndian.Uint64(append([]byte{0, 0}, mustHex(t, ts1.sqn)...))
	amf := binary.BigEndian.Uint16(mustHex(t, ts1.amf))

	res, ck, ik, ak, err := F2345(k, opc, rnd)
	if err != nil {
		t.Fatal(err)
	}
	ref := wm.NewWithOPc(k, opc, rnd, sqn, amf)
	rRes, rCk, rIk, rAk, err := ref.F2345()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(res, rRes) {
		t.Errorf("RES ours=%x oracle=%x", res, rRes)
	}
	if !bytes.Equal(ck, rCk) {
		t.Errorf("CK ours=%x oracle=%x", ck, rCk)
	}
	if !bytes.Equal(ik, rIk) {
		t.Errorf("IK ours=%x oracle=%x", ik, rIk)
	}
	if !bytes.Equal(ak, rAk) {
		t.Errorf("AK ours=%x oracle=%x", ak, rAk)
	}
}
