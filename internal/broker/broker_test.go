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
