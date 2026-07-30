package provisioning

import "time"

type Action string

const (
	ActionIssue   Action = "issue"
	ActionAdd     Action = "add"
	ActionRemove  Action = "remove"
	ActionSuspend Action = "suspend"
	ActionResume  Action = "resume"
	ActionPlan    Action = "plan"
	ActionIP      Action = "ip"
)

// allowedReasons per action. "OTHER" is always accepted (note encouraged).
var allowedReasons = map[Action]map[string]bool{
	ActionSuspend: {"NON_PAYMENT": true, "LOST_STOLEN": true, "FRAUD": true, "MAINTENANCE": true},
	ActionResume:  {"PAYMENT_RECEIVED": true, "RECOVERED": true, "CLEARED": true},
	ActionPlan:    {"UPGRADE": true, "DOWNGRADE": true, "PROMOTION": true},
	ActionIP:      {"ENTERPRISE": true, "M2M": true, "IOT": true},
	ActionIssue:   {"NEW_ACTIVATION": true, "BATCH": true, "REPLACEMENT": true},
	ActionAdd:     {"NEW_ACTIVATION": true, "BATCH": true, "REPLACEMENT": true},
	ActionRemove:  {"DEPROVISION": true, "CLEANUP": true},
}

// ValidReason reports whether reason is allowed for action. "OTHER" is universal.
func ValidReason(a Action, reason string) bool {
	if reason == "" {
		return false
	}
	if reason == "OTHER" {
		return true
	}
	return allowedReasons[a][reason]
}

// AuditRecord is one append-only provisioning-audit entry.
type AuditRecord struct {
	IMSI   string    `bson:"imsi" json:"imsi"`
	Action string    `bson:"action" json:"action"`
	Reason string    `bson:"reason" json:"reason"`
	Note   string    `bson:"note,omitempty" json:"note,omitempty"`
	Actor  string    `bson:"actor" json:"actor"`
	At     time.Time `bson:"at" json:"at"`
}
