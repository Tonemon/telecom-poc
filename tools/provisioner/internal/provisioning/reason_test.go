package provisioning

import "testing"

func TestValidReason(t *testing.T) {
	cases := []struct {
		a      Action
		reason string
		ok     bool
	}{
		{ActionSuspend, "NON_PAYMENT", true},
		{ActionSuspend, "LOST_STOLEN", true},
		{ActionSuspend, "FRAUD", true},
		{ActionSuspend, "OTHER", true},
		{ActionSuspend, "UPGRADE", false},
		{ActionPlan, "PROMOTION", true},
		{ActionIP, "ENTERPRISE", true},
		{ActionResume, "PAYMENT_RECEIVED", true},
		{ActionIssue, "NEW_ACTIVATION", true},
		{ActionSuspend, "", false},
	}
	for _, c := range cases {
		if got := ValidReason(c.a, c.reason); got != c.ok {
			t.Errorf("ValidReason(%s,%q)=%v want %v", c.a, c.reason, got, c.ok)
		}
	}
}
