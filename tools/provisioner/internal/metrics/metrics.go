// Package metrics holds the provisioner's Prometheus instrumentation.
//
// The registry is per-Metrics rather than the global default so tests can build
// independent instances, and so nothing registers by import side effect.
//
// Deliberately absent: any label carrying an IMSI. Subscriber identity has
// unbounded cardinality and does not belong in a metrics store (spec §8);
// per-subscriber history already lives in the provisioning audit log.
package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics is the provisioner's instrumentation, and owns its own registry.
type Metrics struct {
	reg *prometheus.Registry

	operations  *prometheus.CounterVec
	subscribers prometheus.Gauge
	suspended   prometheus.Gauge
	requests    *prometheus.HistogramVec
}

// New builds a Metrics with everything registered and ready to serve.
func New() *Metrics {
	m := &Metrics{
		reg: prometheus.NewRegistry(),
		operations: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "provisioner_operations_total",
			Help: "Completed provisioning operations by operation and reason code.",
		}, []string{"operation", "reason_code"}),
		subscribers: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "provisioner_subscribers",
			Help: "Subscribers currently provisioned.",
		}),
		suspended: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "provisioner_subscribers_suspended",
			Help: "Subscribers currently suspended (barred).",
		}),
		requests: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "provisioner_http_request_duration_seconds",
			Help:    "Provisioner API request duration by route and status.",
			Buckets: prometheus.DefBuckets,
		}, []string{"route", "status"}),
	}
	m.reg.MustRegister(m.operations, m.subscribers, m.suspended, m.requests)
	return m
}

// Registry exposes the private registry, for tests and for the handler.
func (m *Metrics) Registry() *prometheus.Registry { return m.reg }

// RecordOperation counts one *completed* provisioning operation. Failures never
// reach this path (see the package doc on where it is called from) and show up
// as non-2xx in provisioner_http_request_duration_seconds instead.
func (m *Metrics) RecordOperation(operation, reasonCode string) {
	m.operations.WithLabelValues(operation, reasonCode).Inc()
}

// SetSubscribers publishes the current subscriber counts. Both are absolute
// values, not deltas.
func (m *Metrics) SetSubscribers(total, suspended int) {
	m.subscribers.Set(float64(total))
	m.suspended.Set(float64(suspended))
}

// ObserveRequest records one API request's duration.
func (m *Metrics) ObserveRequest(route string, status int, d time.Duration) {
	m.requests.WithLabelValues(route, strconv.Itoa(status)).Observe(d.Seconds())
}

// Handler serves the exposition format for this registry.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.reg, promhttp.HandlerOpts{})
}
