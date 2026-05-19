package auth

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/noahmagill/webhook-rev-shell/internal/config"
	"github.com/noahmagill/webhook-rev-shell/internal/store"
)

type Kind string

const (
	KindWorkspaceBrowser Kind = "workspace_browser"
	KindWorkspaceCLI     Kind = "workspace_cli"
	KindSessionBrowser   Kind = "session_browser"
	KindSessionCLI       Kind = "session_cli"
)

type Principal struct {
	Kind        Kind
	WorkspaceID string
	SessionID   string // set for session-scoped principals
}

type ctxKey struct{}

func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, ctxKey{}, p)
}

func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(ctxKey{}).(Principal)
	return p, ok
}

// ResolveBearer loads a principal from Authorization: Bearer.
func ResolveBearer(s *store.Store, bearer string) (Principal, error) {
	if bearer == "" {
		return Principal{}, fmt.Errorf("no bearer")
	}
	ot, err := s.GetOperatorToken(HashToken(bearer))
	if err != nil {
		return Principal{}, fmt.Errorf("invalid bearer")
	}
	if time.Now().After(ot.ExpiresAt) {
		return Principal{}, fmt.Errorf("bearer expired")
	}
	if ot.SessionID != "" {
		return Principal{
			Kind:        KindSessionCLI,
			WorkspaceID: ot.WorkspaceID,
			SessionID:   ot.SessionID,
		}, nil
	}
	return Principal{
		Kind:        KindWorkspaceCLI,
		WorkspaceID: ot.WorkspaceID,
	}, nil
}

func WorkspaceIDFromRequest(s *store.Store, r *http.Request) (string, error) {
	if c, err := r.Cookie(CookieWorkspace); err == nil && c.Value != "" {
		bt, err := s.GetWorkspaceBrowserToken(c.Value)
		if err != nil {
			return "", err
		}
		if time.Now().After(bt.ExpiresAt) {
			return "", fmt.Errorf("workspace cookie expired")
		}
		return bt.WorkspaceID, nil
	}
	return "", fmt.Errorf("no workspace cookie")
}

func RequireWorkspace(p Principal) error {
	switch p.Kind {
	case KindWorkspaceBrowser, KindWorkspaceCLI:
		if p.WorkspaceID == "" {
			return fmt.Errorf("no workspace")
		}
		return nil
	default:
		return fmt.Errorf("workspace access required")
	}
}

func RequireSessionAccess(s *store.Store, p Principal, sessionID string) error {
	if sessionID == "" {
		return fmt.Errorf("no session id")
	}
	switch p.Kind {
	case KindSessionBrowser, KindSessionCLI:
		if p.SessionID != sessionID {
			return fmt.Errorf("session mismatch")
		}
		return nil
	case KindWorkspaceBrowser, KindWorkspaceCLI:
		ok, err := s.SessionOwnedBy(p.WorkspaceID, sessionID)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("session not in workspace")
		}
		return nil
	default:
		return fmt.Errorf("unauthorized")
	}
}

func CanCreateSession(s *store.Store, cfg config.Auth, workspaceID string) error {
	if workspaceID == "" {
		return fmt.Errorf("no workspace")
	}
	n, err := s.CountActiveSessionsForWorkspace(workspaceID)
	if err != nil {
		return err
	}
	if n >= cfg.MaxSessionsPerWorkspace {
		return fmt.Errorf("session limit reached (%d)", cfg.MaxSessionsPerWorkspace)
	}
	return nil
}

// MiddlewareWorkspaceBearer requires a valid workspace CLI bearer and sets principal on context.
func MiddlewareWorkspaceBearer(s *store.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			auth := r.Header.Get("Authorization")
			if !strings.HasPrefix(auth, "Bearer ") {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			p, err := ResolveBearer(s, strings.TrimPrefix(auth, "Bearer "))
			if err != nil || p.Kind != KindWorkspaceCLI {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r.WithContext(WithPrincipal(r.Context(), p)))
		})
	}
}

// ResolveAttachPrincipal accepts workspace CLI, session CLI, or session browser cookie/?t=.
func ResolveAttachPrincipal(s *store.Store, r *http.Request, sessionID string) (Principal, error) {
	if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		p, err := ResolveBearer(s, strings.TrimPrefix(auth, "Bearer "))
		if err != nil {
			return Principal{}, err
		}
		if err := RequireSessionAccess(s, p, sessionID); err != nil {
			return Principal{}, err
		}
		return p, nil
	}
	if err := ValidateBrowserTokenForSession(s, r, sessionID); err == nil {
		bt, _ := s.GetBrowserToken(browserTokenValue(r))
		ws := ""
		if bt != nil {
			sess, _ := s.GetSession(sessionID)
			if sess != nil {
				ws = sess.OwnerWorkspaceID
			}
		}
		return Principal{Kind: KindSessionBrowser, WorkspaceID: ws, SessionID: sessionID}, nil
	}
	return Principal{}, fmt.Errorf("unauthorized")
}
