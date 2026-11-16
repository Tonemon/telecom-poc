package network

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

// SMFClient reads the IP address of every live session from the SMF's /pdu-info (served by the
// SMF's metrics server). The SMF assigns the addresses, so this is where an operator looks them
// up. A zero-value baseURL disables it.
type SMFClient struct {
	baseURL string
	http    *http.Client
}

func NewSMFClient(baseURL string) *SMFClient {
	return &SMFClient{baseURL: baseURL, http: &http.Client{Timeout: 2 * time.Second}}
}

type pduInfoResponse struct {
	Items []struct {
		SUPI string `json:"supi"`
		PDU  []struct {
			IPv4 string `json:"ipv4"`
		} `json:"pdu"`
	} `json:"items"`
}

// FetchIPs returns the IPv4 address of each subscriber's first session, keyed by IMSI. Like the
// MME's state it is best-effort: any failure yields an empty map, and reachable says whether
// the SMF answered.
func (c *SMFClient) FetchIPs(ctx context.Context) (ips map[string]string, reachable bool) {
	ips = map[string]string{}
	if c == nil || c.baseURL == "" {
		return ips, false
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/pdu-info", nil)
	if err != nil {
		return ips, false
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return ips, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ips, false
	}
	var parsed pduInfoResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return ips, false
	}
	for _, item := range parsed.Items {
		imsi := strings.TrimPrefix(item.SUPI, "imsi-") // the SMF writes "imsi-<digits>", the MME just the digits
		for _, p := range item.PDU {
			if p.IPv4 != "" {
				ips[imsi] = p.IPv4
				break
			}
		}
	}
	return ips, true
}
