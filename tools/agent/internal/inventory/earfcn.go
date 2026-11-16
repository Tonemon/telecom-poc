package inventory

// LTE downlink bands from 3GPP TS 36.101 table 5.7.3-1: F_DL = F_DL_low + 0.1 * (N_DL - N_Offs-DL).
// Only the bands a lab like this is likely to use; others report !ok.
var bands = []struct {
	band          int
	fLow10        int // F_DL_low in units of 0.1 MHz, so the result is exact
	nOffs, nFirst int
	nLast         int
}{
	{1, 21100, 0, 0, 599},
	{3, 18050, 1200, 1200, 1949},
	{7, 26200, 2750, 2750, 3449},
	{8, 9250, 3450, 3450, 3799},
	{20, 7910, 6150, 6150, 6449},
}

// Band returns the LTE band and downlink centre frequency (MHz) of a downlink EARFCN.
func Band(earfcn int) (band int, dlMHz float64, ok bool) {
	for _, b := range bands {
		if earfcn >= b.nFirst && earfcn <= b.nLast {
			return b.band, float64(b.fLow10+earfcn-b.nOffs) / 10, true
		}
	}
	return 0, 0, false
}

// BandwidthMHz is the channel bandwidth of an LTE cell with n_prb resource blocks (0 if not a standard size).
func BandwidthMHz(prb int) float64 {
	return map[int]float64{6: 1.4, 15: 3, 25: 5, 50: 10, 75: 15, 100: 20}[prb]
}
