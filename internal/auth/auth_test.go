package auth

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/noahmagill/webhook-rev-shell/internal/config"
	"github.com/noahmagill/webhook-rev-shell/internal/protocol"
	"github.com/noahmagill/webhook-rev-shell/internal/store"
)

func TestWorkspaceSessionOwnership(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	cfg := config.DefaultAuth("http://localhost:8080")
	wsID, _, _, err := BootstrapWorkspace(s, cfg)
	if err != nil {
		t.Fatal(err)
	}

	sess := &protocol.Session{
		ID:               "550e8400-e29b-41d4-a716-446655440000",
		Name:             "test",
		Secret:           "secret",
		OwnerWorkspaceID: wsID,
		CreatedAt:        time.Now(),
		LastActivity:     time.Now(),
		State:            protocol.StateWaiting,
	}
	if err := s.PutSession(sess); err != nil {
		t.Fatal(err)
	}

	ok, err := s.SessionOwnedBy(wsID, sess.ID)
	if err != nil || !ok {
		t.Fatalf("ownership: ok=%v err=%v", ok, err)
	}
	ok, err = s.SessionOwnedBy("other-workspace", sess.ID)
	if err != nil || ok {
		t.Fatalf("expected no ownership for other workspace")
	}

	n, err := s.CountActiveSessionsForWorkspace(wsID)
	if err != nil || n != 1 {
		t.Fatalf("count=%d err=%v", n, err)
	}
}

func TestIssueWorkspaceCLIToken(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	cfg := config.DefaultAuth("http://localhost:8080")
	wsID, _, _, err := BootstrapWorkspace(s, cfg)
	if err != nil {
		t.Fatal(err)
	}
	tok, _, err := IssueWorkspaceCLIToken(s, wsID, cfg)
	if err != nil {
		t.Fatal(err)
	}
	p, err := ResolveBearer(s, tok)
	if err != nil || p.Kind != KindWorkspaceCLI || p.WorkspaceID != wsID {
		t.Fatalf("principal %+v err=%v", p, err)
	}
}
