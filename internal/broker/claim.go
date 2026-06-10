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
	r.claimMu.Lock()
	r.pruneClaimLocked()
	id := ""
	if r.claimed != nil {
		id = r.claimed.targetID
	}
	r.claimMu.Unlock()
	if id == "" {
		id = r.FirstTargetID()
	}
	if id == "" {
		return false
	}
	return r.SendToTarget(id, data)
}

func (r *Room) DisconnectOperators() {
	msg := []byte("\r\n\x1b[33m[beacon disconnected]\x1b[0m\r\n")
	var ops []*OperatorConn
	r.Operators.Range(func(_, val any) bool {
		op := val.(*OperatorConn)
		r.Operators.Delete(op.ID)
		ops = append(ops, op)
		return true
	})
	for _, op := range ops {
		select {
		case op.Send <- msg:
		default:
		}
	}
	for _, op := range ops {
		op.Cancel()
	}
}
