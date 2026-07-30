package api

// IssueRequest issues one or more SIMs. Count defaults to 1.
type IssueRequest struct {
	Count  int    `json:"count,omitempty"`
	APN    string `json:"apn,omitempty"`
	Reason string `json:"reason"`
	Note   string `json:"note,omitempty"`
}

// AddRequest adds a subscriber with caller-supplied identity/keys.
type AddRequest struct {
	IMSI   string `json:"imsi"`
	K      string `json:"k"`
	OPc    string `json:"opc"`
	APN    string `json:"apn,omitempty"`
	Reason string `json:"reason"`
	Note   string `json:"note,omitempty"`
}

// IssuedSIM is one issued profile (Ki returned once, at issue time only).
type IssuedSIM struct {
	IMSI      string `json:"imsi"`
	K         string `json:"k"`
	OPc       string `json:"opc"`
	UsimBlock string `json:"usim_block"`
}

type ReasonRequest struct {
	Reason string `json:"reason"`
	Note   string `json:"note,omitempty"`
}

type PlanRequest struct {
	DL     string `json:"dl"` // e.g. "100M"
	UL     string `json:"ul"`
	Reason string `json:"reason"`
	Note   string `json:"note,omitempty"`
}

type IPRequest struct {
	IPv4   string `json:"ipv4"` // "" clears the static IP
	Reason string `json:"reason"`
	Note   string `json:"note,omitempty"`
}

type SubscriberView struct {
	IMSI       string `json:"imsi"`
	Status     string `json:"status"` // "active" | "suspended"
	DL         string `json:"dl,omitempty"`
	UL         string `json:"ul,omitempty"`
	StaticIPv4 string `json:"static_ipv4,omitempty"`
	LastAction string `json:"last_action,omitempty"`
	LastReason string `json:"last_reason,omitempty"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}
