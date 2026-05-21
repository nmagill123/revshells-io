package store

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/noahmagill/webhook-rev-shell/internal/protocol"
)

func TestCleanupSessionArtifacts(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	sessionID := "session-1"
	targetID := "target-1"

	if err := s.PutSession(&protocol.Session{
		ID:           sessionID,
		Name:         "test",
		Secret:       "secret",
		CreatedAt:    time.Now(),
		LastActivity: time.Now(),
		State:        protocol.StateKilled,
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.PutTarget(&protocol.Target{ID: targetID, SessionID: sessionID}); err != nil {
		t.Fatal(err)
	}
	if err := s.AppendTranscript(sessionID, targetID, 1, []byte("hello")); err != nil {
		t.Fatal(err)
	}
	if err := s.PutBrowserToken(&protocol.BrowserToken{
		Token:     "browser-token",
		SessionID: sessionID,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.PutOperatorToken(&OperatorToken{
		TokenHash: "operator-token",
		SessionID: sessionID,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}

	if err := s.CleanupSessionArtifacts(sessionID); err != nil {
		t.Fatal(err)
	}

	targets, err := s.ListTargets(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 0 {
		t.Fatalf("expected no targets after cleanup, got %d", len(targets))
	}
	transcript, err := s.GetTranscript(sessionID, targetID)
	if err != nil {
		t.Fatal(err)
	}
	if len(transcript) != 0 {
		t.Fatalf("expected no transcript after cleanup, got %d", len(transcript))
	}
	if _, err := s.GetBrowserToken("browser-token"); err == nil {
		t.Fatal("expected browser token to be deleted")
	}
	if _, err := s.GetOperatorToken("operator-token"); err == nil {
		t.Fatal("expected operator token to be deleted")
	}
}
