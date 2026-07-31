package store

import (
	"context"
	"testing"

	"github.com/telecom-poc/provisioner/internal/subscriber"
)

func TestFakeStore_Lifecycle(t *testing.T) {
	ctx := context.Background()
	f := NewFake()
	s := subscriber.Subscriber{IMSI: "999700000000001", K: "AA", OPc: "BB", APN: "internet"}
	if err := f.Insert(ctx, s); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Get(ctx, s.IMSI); err != nil {
		t.Fatalf("get after insert: %v", err)
	}
	if err := f.SetStatus(ctx, s.IMSI, true); err != nil {
		t.Fatal(err)
	}
	rec, _ := f.Get(ctx, s.IMSI)
	if !rec.Barred {
		t.Fatal("expected barred after SetStatus(true)")
	}
	if _, err := f.Get(ctx, "000000000000000"); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestFakeStore_MaxMSIN(t *testing.T) {
	ctx := context.Background()
	f := NewFake()
	f.Insert(ctx, subscriber.Subscriber{IMSI: "999700000000005"})
	f.Insert(ctx, subscriber.Subscriber{IMSI: "999700000000009"})
	max, err := f.MaxMSIN(ctx, "99970")
	if err != nil || max != 9 {
		t.Fatalf("MaxMSIN = %d,%v want 9", max, err)
	}
}
