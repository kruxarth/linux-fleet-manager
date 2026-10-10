package node

import "time"

const (
	Pending  = "pending"
	Online   = "online"
	Degraded = "degraded"
	Offline  = "offline"
	Revoked  = "revoked"
)

type Snapshot struct {
	State           string
	LastHeartbeatAt *time.Time
	LastError       string
	CreatedAt       time.Time
}

func Effective(s Snapshot, now time.Time, offlineAfter time.Duration) string {
	if s.State == Revoked {
		return Revoked
	}
	if s.LastHeartbeatAt == nil {
		if now.Sub(s.CreatedAt) > time.Hour {
			return Offline
		}
		return Pending
	}
	if now.Sub(*s.LastHeartbeatAt) > offlineAfter {
		return Offline
	}
	if s.LastError != "" {
		return Degraded
	}
	return Online
}

func CanDispatch(state string) bool {
	return state == Online || state == Degraded
}
