package broker

import (
	"errors"
	"time"
)

// Must exceed HTTP long-poll block (30s) so live poll beacons are not unclaimed mid-wait.
const ClaimStaleTimeout = 40 * time.Second

var ErrSessionHasActiveBeacon = errors.New("session has active beacon")

type beaconClaim struct {
	targetID string
	lastSeen time.Time
}

func (r *Room) TryClaim(targetID string) error {
	r.claimMu.Lock()
	defer r.claimMu.Unlock()
	r.pruneClaimLocked()
	if r.claimed != nil && r.claimed.targetID != targetID {
		return ErrSessionHasActiveBeacon
	}
	r.claimed = &beaconClaim{targetID: targetID, lastSeen: time.Now()}
	return nil
}

func (r *Room) TouchClaim(targetID string) {
	r.claimMu.Lock()
	defer r.claimMu.Unlock()
	if r.claimed != nil && r.claimed.targetID == targetID {
		r.claimed.lastSeen = time.Now()
	}
}

func (r *Room) ReleaseClaim(targetID string) {
	r.claimMu.Lock()
	defer r.claimMu.Unlock()
	if r.claimed != nil && r.claimed.targetID == targetID {
		r.claimed = nil
	}
}

func (r *Room) WasClaimedBy(targetID string) bool {
	r.claimMu.Lock()
	defer r.claimMu.Unlock()
	r.pruneClaimLocked()
	return r.claimed != nil && r.claimed.targetID == targetID
}

func (r *Room) ClaimedTargetID() string {
	r.claimMu.Lock()
	defer r.claimMu.Unlock()
	r.pruneClaimLocked()
	if r.claimed == nil {
		return ""
	}
	return r.claimed.targetID
}

func (r *Room) pruneClaimLocked() {
	if r.claimed != nil && time.Since(r.claimed.lastSeen) > ClaimStaleTimeout {
		r.claimed = nil
	}
}

func (r *Room) SendToClaimed(data []byte) bool {
	id := r.ClaimedTargetID()
	if id == "" {
		return r.SendToTarget(r.FirstTargetID(), data)
	}
	return r.SendToTarget(id, data)
}

// DisconnectOperators closes operator attach sessions when the active beacon leaves.
func (r *Room) DisconnectOperators() {
	msg := []byte("\r\n\x1b[33m[beacon disconnected]\x1b[0m\r\n")
	r.BroadcastToOperators(msg)
	r.Operators.Range(func(_, val any) bool {
		op := val.(*OperatorConn)
		op.Cancel()
		return true
	})
}
