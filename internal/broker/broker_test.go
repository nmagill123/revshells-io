package broker

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/noahmagill/webhook-rev-shell/internal/protocol"
	"github.com/noahmagill/webhook-rev-shell/internal/store"
)

func TestGetOrLoadRoomReturnsClosedError(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	b := New(s)
	sess := &protocol.Session{
		ID:           "session-1",
		Name:         "test",
		Secret:       "secret",
		CreatedAt:    time.Now(),
		LastActivity: time.Now(),
		State:        protocol.StateKilled,
	}
	if err := s.PutSession(sess); err != nil {
		t.Fatal(err)
	}

	room, err := b.GetOrLoadRoom(sess.ID)
	if err != ErrSessionClosed {
		t.Fatalf("expected ErrSessionClosed, got room=%v err=%v", room, err)
	}
}

func TestPruneStaleTargetsUsesTransportSpecificTimeouts(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	b := New(s)
	sess := &protocol.Session{
		ID:           "session-1",
		Name:         "test",
		Secret:       "secret",
		CreatedAt:    time.Now(),
		LastActivity: time.Now(),
		State:        protocol.StateActive,
	}
	if err := s.PutSession(sess); err != nil {
		t.Fatal(err)
	}
	room, err := b.GetOrLoadRoom(sess.ID)
	if err != nil {
		t.Fatal(err)
	}

	wsTL := &TargetLink{
		ID: "ws-target",
		Info: &protocol.Target{
			ID:        "ws-target",
			SessionID: sess.ID,
			Transport: "websocket",
		},
		CmdQueue: make(chan []byte, 1),
		Done:     make(chan struct{}),
	}
	wsTL.Touch()
	wsTL.seenMu.Lock()
	wsTL.lastSeen = time.Now().Add(-2 * time.Minute)
	wsTL.seenMu.Unlock()

	pollTL := &TargetLink{
		ID: "poll-target",
		Info: &protocol.Target{
			ID:        "poll-target",
			SessionID: sess.ID,
			Transport: "poll",
		},
		CmdQueue: make(chan []byte, 1),
		Done:     make(chan struct{}),
	}
	pollTL.Touch()
	pollTL.seenMu.Lock()
	pollTL.lastSeen = time.Now().Add(-2 * time.Minute)
	pollTL.seenMu.Unlock()

	room.Targets.Store(wsTL.ID, wsTL)
	room.Targets.Store(pollTL.ID, pollTL)

	b.PruneStaleTargets()

	if _, ok := room.Targets.Load(wsTL.ID); !ok {
		t.Fatal("websocket target pruned too early")
	}
	if _, ok := room.Targets.Load(pollTL.ID); ok {
		t.Fatal("poll target should have been pruned")
	}
}
