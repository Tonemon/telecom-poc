package inventory

import "testing"

func TestBand7(t *testing.T) {
	b, mhz, ok := Band(3350)
	if !ok || b != 7 || mhz != 2680 {
		t.Fatalf("Band(3350) = %d, %v, %v; want 7, 2680, true", b, mhz, ok)
	}
}

func TestBandEdges(t *testing.T) {
	cases := []struct {
		earfcn int
		band   int
		mhz    float64
	}{
		{0, 1, 2110}, {599, 1, 2169.9}, {1200, 3, 1805}, {2750, 7, 2620}, {3449, 7, 2689.9},
		{3450, 8, 925}, {6150, 20, 791},
	}
	for _, c := range cases {
		b, mhz, ok := Band(c.earfcn)
		if !ok || b != c.band || mhz != c.mhz {
			t.Errorf("Band(%d) = %d, %v, %v; want %d, %v", c.earfcn, b, mhz, ok, c.band, c.mhz)
		}
	}
}

func TestBandUnknown(t *testing.T) {
	if _, _, ok := Band(99999); ok {
		t.Fatal("want !ok for an EARFCN outside the table")
	}
}

func TestBandwidth(t *testing.T) {
	for prb, mhz := range map[int]float64{6: 1.4, 15: 3, 25: 5, 50: 10, 75: 15, 100: 20, 7: 0} {
		if got := BandwidthMHz(prb); got != mhz {
			t.Errorf("BandwidthMHz(%d) = %v, want %v", prb, got, mhz)
		}
	}
}
