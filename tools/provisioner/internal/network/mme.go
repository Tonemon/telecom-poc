// Package network enriches subscriber lookups with live state from the
// MME's own metrics server (Open5GS exposes /ue-info there, unrelated to
// the Plan 4 Prometheus/Grafana observability work -- this just reads an
// endpoint the MME already serves).
package network

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

// Info is one UE's live network state, as last reported by the MME.
type Info struct {
	CMState     string // "connected" | "idle"
	MMState     string // "registered" | "deregistered"
	ENBID       int
	CellID      int
	TAC         int
	APN         string
	QCI         int
	BearerCount int
	PDUState    string
}

type ueInfoResponse struct {
	Items []ueInfoItem `json:"items"`
}

type ueInfoItem struct {
	SUPI    string `json:"supi"`
	CMState string `json:"cm_state"`
	MMState string `json:"mm_state"`
	ENB     struct {
		ENBID  int `json:"enb_id"`
		CellID int `json:"cell_id"`
	} `json:"enb"`
	Location struct {
		TAI struct {
			TAC int `json:"tac"`
		} `json:"tai"`
	} `json:"location"`
	PDN []struct {
		APN         string `json:"apn"`
		QCI         int    `json:"qci"`
		BearerCount int    `json:"bearer_count"`
		PDUState    string `json:"pdu_state"`
	} `json:"pdn"`
}

// MMEClient reads the MME's live UE state. A zero-value baseURL disables
// it entirely (FetchAll becomes a no-op) -- used by tests and by any
// deployment that doesn't have (or care about) a reachable MME.
type MMEClient struct {
	baseURL string
	http    *http.Client
}

func NewMMEClient(baseURL string) *MMEClient {
	return &MMEClient{baseURL: baseURL, http: &http.Client{Timeout: 2 * time.Second}}
}

// FetchAll returns live info for every currently-known UE, keyed by IMSI
// (the MME calls it SUPI). Best-effort: live network state is an
// enrichment, never a reason to fail a subscriber lookup, so any failure
// (MME unreachable, RAN not up, slow response) just yields an empty map
// rather than an error. reachable reports whether the query itself
// succeeded, so callers can distinguish "we asked and this UE isn't
// connected" from "we couldn't ask at all".
func (c *MMEClient) FetchAll(ctx context.Context) (info map[string]Info, reachable bool) {
	info = map[string]Info{}
	if c == nil || c.baseURL == "" {
		return info, false
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/ue-info", nil)
	if err != nil {
		return info, false
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return info, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return info, false
	}
	var parsed ueInfoResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return info, false
	}
	for _, item := range parsed.Items {
		i := Info{
			CMState: item.CMState, MMState: item.MMState,
			ENBID: item.ENB.ENBID, CellID: item.ENB.CellID, TAC: item.Location.TAI.TAC,
		}
		if len(item.PDN) > 0 {
			i.APN = item.PDN[0].APN
			i.QCI = item.PDN[0].QCI
			i.BearerCount = item.PDN[0].BearerCount
			i.PDUState = item.PDN[0].PDUState
		}
		info[item.SUPI] = i
	}
	return info, true
}
