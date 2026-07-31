package store

import (
	"context"
	"errors"

	"github.com/telecom-poc/provisioner/internal/provisioning"
	"github.com/telecom-poc/provisioner/internal/subscriber"
)

var ErrNotFound = errors.New("subscriber not found")

type Record struct {
	IMSI       string
	Barred     bool
	DL, UL     subscriber.AMBR
	StaticIPv4 string
	LastAction string
	LastReason string
	LastNote   string
}

type Store interface {
	Insert(ctx context.Context, s subscriber.Subscriber) error
	Delete(ctx context.Context, imsi string) error
	Get(ctx context.Context, imsi string) (*Record, error)
	List(ctx context.Context) ([]Record, error)
	SetStatus(ctx context.Context, imsi string, barred bool) error
	SetAMBR(ctx context.Context, imsi string, dl, ul subscriber.AMBR) error
	SetStaticIP(ctx context.Context, imsi, ipv4 string) error
	MaxMSIN(ctx context.Context, plmn string) (uint64, error)
	AppendAudit(ctx context.Context, r provisioning.AuditRecord) error
	History(ctx context.Context, imsi string) ([]provisioning.AuditRecord, error)
}
