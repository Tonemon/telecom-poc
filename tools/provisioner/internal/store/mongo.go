package store

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/telecom-poc/provisioner/internal/provisioning"
	"github.com/telecom-poc/provisioner/internal/subscriber"
)

// MongoStore enforces suspension by STASHING: a suspended subscriber is moved
// out of the live `subscribers` collection (which the Open5GS HSS answers from)
// into `suspended_subscribers`, so the HSS returns USER_UNKNOWN and the MME
// rejects the attach. The record + keys are fully preserved and restored on
// resume — nothing is deleted. This is the enforcement path that Open5GS's MME
// actually honors: its MME ignores the Subscriber-Status/ODB flag, so the flag
// alone cannot bar an attach on this stack. See docs/provisioning.md.
type MongoStore struct {
	subs      *mongo.Collection
	suspended *mongo.Collection
	audit     *mongo.Collection
}

var _ Store = (*MongoStore)(nil)

// NewMongoStore connects to `uri` (db defaults to "open5gs" if none in the URI path).
func NewMongoStore(ctx context.Context, uri string) (*MongoStore, error) {
	cli, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		return nil, err
	}
	if err := cli.Ping(ctx, nil); err != nil {
		return nil, err
	}
	db := cli.Database("open5gs")
	return &MongoStore{
		subs:      db.Collection("subscribers"),
		suspended: db.Collection("suspended_subscribers"),
		audit:     db.Collection("provisioning_audit"),
	}, nil
}

func (m *MongoStore) Insert(ctx context.Context, s subscriber.Subscriber) error {
	// idempotent: remove any existing entry (live OR suspended) for this IMSI first
	if _, err := m.subs.DeleteOne(ctx, bson.M{"imsi": s.IMSI}); err != nil {
		return err
	}
	if _, err := m.suspended.DeleteOne(ctx, bson.M{"imsi": s.IMSI}); err != nil {
		return err
	}
	_, err := m.subs.InsertOne(ctx, s.Document())
	return err
}

func (m *MongoStore) Delete(ctx context.Context, imsi string) error {
	if _, err := m.subs.DeleteOne(ctx, bson.M{"imsi": imsi}); err != nil {
		return err
	}
	_, err := m.suspended.DeleteOne(ctx, bson.M{"imsi": imsi})
	return err
}

// subDoc is the subset we read back for a Record.
type subDoc struct {
	IMSI             string `bson:"imsi"`
	SubscriberStatus int    `bson:"subscriber_status"`
	AMBR             struct {
		Downlink subscriber.AMBR `bson:"downlink"`
		Uplink   subscriber.AMBR `bson:"uplink"`
	} `bson:"ambr"`
	Slice []struct {
		Session []struct {
			UE struct {
				IPv4 string `bson:"ipv4"`
			} `bson:"ue"`
		} `bson:"session"`
	} `bson:"slice"`
	Provisioning struct {
		LastAction string `bson:"last_action"`
		LastReason string `bson:"last_reason"`
		LastNote   string `bson:"last_note"`
	} `bson:"provisioning"`
}

func toRecord(d subDoc, barred bool) Record {
	r := Record{
		IMSI: d.IMSI, Barred: barred,
		DL: d.AMBR.Downlink, UL: d.AMBR.Uplink,
		LastAction: d.Provisioning.LastAction, LastReason: d.Provisioning.LastReason, LastNote: d.Provisioning.LastNote,
	}
	if len(d.Slice) > 0 && len(d.Slice[0].Session) > 0 {
		r.StaticIPv4 = d.Slice[0].Session[0].UE.IPv4
	}
	return r
}

func (m *MongoStore) Get(ctx context.Context, imsi string) (*Record, error) {
	var d subDoc
	err := m.subs.FindOne(ctx, bson.M{"imsi": imsi}).Decode(&d)
	if err == nil {
		rec := toRecord(d, false)
		return &rec, nil
	}
	if err != mongo.ErrNoDocuments {
		return nil, err
	}
	// not live — check the suspended holding collection
	err = m.suspended.FindOne(ctx, bson.M{"imsi": imsi}).Decode(&d)
	if err == mongo.ErrNoDocuments {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	rec := toRecord(d, true)
	return &rec, nil
}

func (m *MongoStore) collect(ctx context.Context, coll *mongo.Collection, barred bool, out *[]Record) error {
	cur, err := coll.Find(ctx, bson.M{}, options.Find().SetSort(bson.M{"imsi": 1}))
	if err != nil {
		return err
	}
	defer cur.Close(ctx)
	for cur.Next(ctx) {
		var d subDoc
		if err := cur.Decode(&d); err != nil {
			return err
		}
		*out = append(*out, toRecord(d, barred))
	}
	return cur.Err()
}

func (m *MongoStore) List(ctx context.Context) ([]Record, error) {
	var out []Record
	if err := m.collect(ctx, m.subs, false, &out); err != nil {
		return nil, err
	}
	if err := m.collect(ctx, m.suspended, true, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// SetStatus stashes (barred=true) or restores (barred=false) the subscriber by
// moving its document between the live and suspended collections. Idempotent.
func (m *MongoStore) SetStatus(ctx context.Context, imsi string, barred bool) error {
	if barred {
		return m.move(ctx, m.subs, m.suspended, imsi, 1)
	}
	return m.move(ctx, m.suspended, m.subs, imsi, 0)
}

// move relocates a subscriber doc from `from` to `to`, stamping subscriber_status.
func (m *MongoStore) move(ctx context.Context, from, to *mongo.Collection, imsi string, status int) error {
	var raw bson.M
	err := from.FindOne(ctx, bson.M{"imsi": imsi}).Decode(&raw)
	if err == mongo.ErrNoDocuments {
		// already in the target collection? -> idempotent success
		cnt, cerr := to.CountDocuments(ctx, bson.M{"imsi": imsi})
		if cerr != nil {
			return cerr
		}
		if cnt > 0 {
			return nil
		}
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	raw["subscriber_status"] = status
	if _, err := to.InsertOne(ctx, raw); err != nil {
		return err
	}
	_, err = from.DeleteOne(ctx, bson.M{"imsi": imsi})
	return err
}

// setEither applies a $set to whichever collection currently holds the imsi.
func (m *MongoStore) setEither(ctx context.Context, imsi string, set bson.M) error {
	res, err := m.subs.UpdateOne(ctx, bson.M{"imsi": imsi}, bson.M{"$set": set})
	if err != nil {
		return err
	}
	if res.MatchedCount > 0 {
		return nil
	}
	res, err = m.suspended.UpdateOne(ctx, bson.M{"imsi": imsi}, bson.M{"$set": set})
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}

func (m *MongoStore) SetAMBR(ctx context.Context, imsi string, dl, ul subscriber.AMBR) error {
	a := bson.M{
		"downlink": bson.M{"value": dl.Value, "unit": dl.Unit},
		"uplink":   bson.M{"value": ul.Value, "unit": ul.Unit},
	}
	// set both the UE-AMBR and the default session AMBR
	return m.setEither(ctx, imsi, bson.M{"ambr": a, "slice.0.session.0.ambr": a})
}

func (m *MongoStore) SetStaticIP(ctx context.Context, imsi, ipv4 string) error {
	if ipv4 == "" {
		if _, err := m.subs.UpdateOne(ctx, bson.M{"imsi": imsi}, bson.M{"$unset": bson.M{"slice.0.session.0.ue": ""}}); err != nil {
			return err
		}
		_, err := m.suspended.UpdateOne(ctx, bson.M{"imsi": imsi}, bson.M{"$unset": bson.M{"slice.0.session.0.ue": ""}})
		return err
	}
	return m.setEither(ctx, imsi, bson.M{"slice.0.session.0.ue.ipv4": ipv4})
}

func (m *MongoStore) MaxMSIN(ctx context.Context, plmn string) (uint64, error) {
	var max uint64
	for _, coll := range []*mongo.Collection{m.subs, m.suspended} {
		cur, err := coll.Find(ctx, bson.M{"imsi": bson.M{"$regex": "^" + plmn}}, options.Find().SetProjection(bson.M{"imsi": 1}))
		if err != nil {
			return 0, err
		}
		for cur.Next(ctx) {
			var d struct {
				IMSI string `bson:"imsi"`
			}
			if err := cur.Decode(&d); err != nil {
				cur.Close(ctx)
				return 0, err
			}
			if len(d.IMSI) > len(plmn) {
				if n := parseUint(d.IMSI[len(plmn):]); n > max {
					max = n
				}
			}
		}
		if err := cur.Err(); err != nil {
			cur.Close(ctx)
			return 0, err
		}
		cur.Close(ctx)
	}
	return max, nil
}

func parseUint(s string) uint64 {
	var n uint64
	for _, c := range s {
		if c < '0' || c > '9' {
			return n
		}
		n = n*10 + uint64(c-'0')
	}
	return n
}

func (m *MongoStore) AppendAudit(ctx context.Context, r provisioning.AuditRecord) error {
	if r.At.IsZero() {
		r.At = time.Now().UTC()
	}
	_, err := m.audit.InsertOne(ctx, r)
	if err != nil {
		return err
	}
	// mirror the latest action onto the subscriber doc wherever it lives
	// (Open5GS-ignored namespaced field) — best effort.
	mirror := bson.M{"$set": bson.M{
		"provisioning": bson.M{"last_action": r.Action, "last_reason": r.Reason, "last_note": r.Note, "updated_at": r.At},
	}}
	_, _ = m.subs.UpdateOne(ctx, bson.M{"imsi": r.IMSI}, mirror)
	_, _ = m.suspended.UpdateOne(ctx, bson.M{"imsi": r.IMSI}, mirror)
	return nil
}

func (m *MongoStore) History(ctx context.Context, imsi string) ([]provisioning.AuditRecord, error) {
	cur, err := m.audit.Find(ctx, bson.M{"imsi": imsi}, options.Find().SetSort(bson.M{"at": 1}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var out []provisioning.AuditRecord
	if err := cur.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}
