package node

import (
	"testing"
	"time"
)

func TestEffective(t *testing.T) {
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	offline := 90 * time.Second
	hb := now.Add(-10 * time.Second)

	if got := Effective(Snapshot{State: Revoked, CreatedAt: now}, now, offline); got != Revoked {
		t.Fatalf("revoked: %s", got)
	}
	if got := Effective(Snapshot{State: Pending, CreatedAt: now}, now, offline); got != Pending {
		t.Fatalf("pending: %s", got)
	}
	if got := Effective(Snapshot{State: Pending, CreatedAt: now.Add(-2 * time.Hour)}, now, offline); got != Offline {
		t.Fatalf("expired pending: %s", got)
	}
	if got := Effective(Snapshot{State: Online, LastHeartbeatAt: &hb, CreatedAt: now.Add(-time.Hour)}, now, offline); got != Online {
		t.Fatalf("online: %s", got)
	}
	stale := now.Add(-2 * time.Minute)
	if got := Effective(Snapshot{State: Online, LastHeartbeatAt: &stale, CreatedAt: now.Add(-time.Hour)}, now, offline); got != Offline {
		t.Fatalf("stale: %s", got)
	}
	if got := Effective(Snapshot{State: Online, LastHeartbeatAt: &hb, LastError: "failed", CreatedAt: now}, now, offline); got != Degraded {
		t.Fatalf("degraded: %s", got)
	}
	if CanDispatch(Offline) || CanDispatch(Pending) || CanDispatch(Revoked) {
		t.Fatal("should not dispatch")
	}
	if !CanDispatch(Online) || !CanDispatch(Degraded) {
		t.Fatal("should dispatch")
	}
}
