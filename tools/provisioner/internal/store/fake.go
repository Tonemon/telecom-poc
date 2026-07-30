package store

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/telecom-poc/provisioner/internal/provisioning"
	"github.com/telecom-poc/provisioner/internal/subscriber"
)

// Fake is an in-memory Store for unit tests.
type Fake struct {
	mu    sync.Mutex
	recs  map[string]*Record
	audit []provisioning.AuditRecord
}

func NewFake() *Fake { return &Fake{recs: map[string]*Record{}} }

func (f *Fake) Insert(_ context.Context, s subscriber.Subscriber) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.recs[s.IMSI] = &Record{IMSI: s.IMSI, DL: s.DL, UL: s.UL, StaticIPv4: s.StaticIPv4}
	return nil
}
func (f *Fake) Delete(_ context.Context, imsi string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.recs, imsi)
	return nil
}
func (f *Fake) Get(_ context.Context, imsi string) (*Record, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.recs[imsi]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *r
	return &cp, nil
}
func (f *Fake) List(_ context.Context) ([]Record, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]Record, 0, len(f.recs))
	for _, r := range f.recs {
		out = append(out, *r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].IMSI < out[j].IMSI })
	return out, nil
}
func (f *Fake) mutate(imsi string, fn func(*Record)) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.recs[imsi]
	if !ok {
		return ErrNotFound
	}
	fn(r)
	return nil
}
func (f *Fake) SetStatus(_ context.Context, imsi string, barred bool) error {
	return f.mutate(imsi, func(r *Record) { r.Barred = barred })
}
func (f *Fake) SetAMBR(_ context.Context, imsi string, dl, ul subscriber.AMBR) error {
	return f.mutate(imsi, func(r *Record) { r.DL, r.UL = dl, ul })
}
func (f *Fake) SetStaticIP(_ context.Context, imsi, ipv4 string) error {
	return f.mutate(imsi, func(r *Record) { r.StaticIPv4 = ipv4 })
}
func (f *Fake) MaxMSIN(_ context.Context, plmn string) (uint64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var max uint64
	for imsi := range f.recs {
		if !strings.HasPrefix(imsi, plmn) {
			continue
		}
		if n, err := strconv.ParseUint(imsi[len(plmn):], 10, 64); err == nil && n > max {
			max = n
		}
	}
	return max, nil
}
func (f *Fake) AppendAudit(_ context.Context, r provisioning.AuditRecord) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.audit = append(f.audit, r)
	return nil
}
func (f *Fake) History(_ context.Context, imsi string) ([]provisioning.AuditRecord, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []provisioning.AuditRecord
	for _, a := range f.audit {
		if a.IMSI == imsi {
			out = append(out, a)
		}
	}
	return out, nil
}
