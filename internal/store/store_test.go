package store

import (
	"context"
	"io/fs"
	"testing"
	"time"

	"fleetmgr/internal/auth"
	nodestate "fleetmgr/internal/node"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	sqlFS, err := fs.Sub(SQLFS, "sql")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Migrate(sqlFS); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestEnrollTokenMaxUses(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	tok, _ := auth.NewToken()
	if err := s.CreateEnrollToken(ctx, "t1", auth.HashToken(tok), `{}`, now.Add(time.Hour), 2, now); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ConsumeEnrollToken(ctx, auth.HashToken(tok), now); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ConsumeEnrollToken(ctx, auth.HashToken(tok), now); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ConsumeEnrollToken(ctx, auth.HashToken(tok), now); err != ErrUnauthorized {
		t.Fatalf("want unauthorized, got %v", err)
	}
}

func TestNodeHeartbeatRoundTrip(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	tok, _ := auth.NewToken()
	row := NodeRow{
		ID:         "n1",
		Hostname:   "box",
		State:      nodestate.Pending,
		TokenHash:  auth.HashToken(tok),
		LabelsJSON: "{}",
		CreatedAt:  rfc(now),
		UpdatedAt:  rfc(now),
	}
	if err := s.InsertNode(ctx, row); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetNodeByTokenHash(ctx, auth.HashToken(tok))
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "n1" {
		t.Fatalf("id %s", got.ID)
	}
	if err := s.Heartbeat(ctx, "n1", "box", "linux", "6.1", "amd64", "0.1", "10.0.0.1:9100", now); err != nil {
		t.Fatal(err)
	}
	n, err := s.GetNode(ctx, "n1")
	if err != nil {
		t.Fatal(err)
	}
	p := n.ToProto(now, 90*time.Second)
	if p.State != nodestate.Online {
		t.Fatalf("state %s", p.State)
	}
}
