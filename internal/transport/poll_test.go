package transport

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/noahmagill/webhook-rev-shell/internal/broker"
	"github.com/noahmagill/webhook-rev-shell/internal/protocol"
	"github.com/noahmagill/webhook-rev-shell/internal/store"
)

func TestPushRejectsUnknownTargetID(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	sessionID := "session-1"
	secret := "secret"
	if err := s.PutSession(&protocol.Session{
		ID:           sessionID,
		Name:         "test",
		Secret:       secret,
		CreatedAt:    time.Now(),
		LastActivity: time.Now(),
		State:        protocol.StateActive,
	}); err != nil {
		t.Fatal(err)
	}

	b := broker.New(s)
	room, err := b.GetOrLoadRoom(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	room.Targets.Store("real-target", &broker.TargetLink{ID: "real-target", Done: make(chan struct{}), CmdQueue: make(chan []byte, 1)})

	req := httptest.NewRequest(http.MethodPost, "/s/"+sessionID+"/"+secret+"/push?target_id=spoofed", http.NoBody)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", sessionID)
	rctx.URLParams.Add("secret", secret)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()

	(&PollHandler{B: b}).Push(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d (%s)", rr.Code, rr.Body.String())
	}
}

func TestPollRejectsUnknownTargetID(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	sessionID := "session-1"
	secret := "secret"
	if err := s.PutSession(&protocol.Session{
		ID:           sessionID,
		Name:         "test",
		Secret:       secret,
		CreatedAt:    time.Now(),
		LastActivity: time.Now(),
		State:        protocol.StateActive,
	}); err != nil {
		t.Fatal(err)
	}

	b := broker.New(s)
	if _, err := b.GetOrLoadRoom(sessionID); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/s/"+sessionID+"/"+secret+"/poll?target_id=spoofed", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", sessionID)
	rctx.URLParams.Add("secret", secret)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()

	(&PollHandler{B: b}).Poll(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d (%s)", rr.Code, rr.Body.String())
	}
}
