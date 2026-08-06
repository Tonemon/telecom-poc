package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// scrape returns the exposition text from the metrics handler.
func scrape(t *testing.T, m *Metrics) string {
	t.Helper()
	rec := httptest.NewRecorder()
	m.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("handler status = %d, want 200", rec.Code)
	}
	return rec.Body.String()
}

func TestRecordOperationCountsByOperationAndReason(t *testing.T) {
	m := New()
	m.RecordOperation("suspend", "non-payment")
	m.RecordOperation("suspend", "non-payment")
	m.RecordOperation("resume", "payment-received")

	body := scrape(t, m)
	for _, want := range []string{
		`provisioner_operations_total{operation="suspend",reason_code="non-payment"} 2`,
		`provisioner_operations_total{operation="resume",reason_code="payment-received"} 1`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in:\n%s", want, body)
		}
	}
}

func TestSameOperationWithDifferentReasonsCountsSeparately(t *testing.T) {
	m := New()
	m.RecordOperation("suspend", "non-payment")
	m.RecordOperation("suspend", "lost-device")

	body := scrape(t, m)
	for _, want := range []string{
		`provisioner_operations_total{operation="suspend",reason_code="non-payment"} 1`,
		`provisioner_operations_total{operation="suspend",reason_code="lost-device"} 1`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in:\n%s", want, body)
		}
	}
}

func TestSetSubscribersReportsTotalAndSuspended(t *testing.T) {
	m := New()
	m.SetSubscribers(7, 2)

	body := scrape(t, m)
	for _, want := range []string{
		"provisioner_subscribers 7",
		"provisioner_subscribers_suspended 2",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in:\n%s", want, body)
		}
	}
}

func TestSetSubscribersOverwritesRatherThanAccumulates(t *testing.T) {
	m := New()
	m.SetSubscribers(7, 2)
	m.SetSubscribers(5, 1)

	body := scrape(t, m)
	if !strings.Contains(body, "provisioner_subscribers 5") {
		t.Fatalf("gauge did not overwrite:\n%s", body)
	}
}

func TestObserveRequestRecordsDurationByRouteAndStatus(t *testing.T) {
	m := New()
	m.ObserveRequest("POST /subscribers/{imsi}/suspend", 200, 12*time.Millisecond)

	body := scrape(t, m)
	want := `provisioner_http_request_duration_seconds_count{route="POST /subscribers/{imsi}/suspend",status="200"} 1`
	if !strings.Contains(body, want) {
		t.Fatalf("missing %q in:\n%s", want, body)
	}
}

// The registry must be private to this Metrics value, not the global default —
// otherwise two servers in one test binary would panic on duplicate registration.
func TestRegistryIsNotGlobal(t *testing.T) {
	a, b := New(), New()
	if a.Registry() == b.Registry() {
		t.Fatal("two Metrics values share a registry; registration will collide")
	}
	a.RecordOperation("add", "new-customer")
	if strings.Contains(scrape(t, b), `operation="add"`) {
		t.Fatal("metrics leaked between independent registries")
	}
}

// Guards the spec's cardinality rule: IMSI must never become a label.
func TestNoIMSILabelIsExposed(t *testing.T) {
	m := New()
	m.RecordOperation("suspend", "non-payment")
	m.SetSubscribers(1, 1)
	if strings.Contains(scrape(t, m), "imsi") {
		t.Fatal("an imsi label leaked into the metrics registry")
	}
}
