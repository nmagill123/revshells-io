package auth

import (
	"time"

	"github.com/google/uuid"
	"github.com/noahmagill/webhook-rev-shell/internal/config"
	"github.com/noahmagill/webhook-rev-shell/internal/protocol"
	"github.com/noahmagill/webhook-rev-shell/internal/store"
)

// BootstrapWorkspace creates a workspace and HttpOnly browser credential.
func BootstrapWorkspace(s *store.Store, cfg config.Auth) (workspaceID, browserToken string, expiresAt time.Time, err error) {
	workspaceID = uuid.New().String()
	now := time.Now()
	expiresAt = now.Add(cfg.WorkspaceBrowserTokenTTL)

	browserToken, err = GenerateToken()
	if err != nil {
		return "", "", time.Time{}, err
	}

	ws := &protocol.Workspace{ID: workspaceID, CreatedAt: now}
	bt := &protocol.WorkspaceBrowserToken{
		Token:       browserToken,
		WorkspaceID: workspaceID,
		CreatedAt:   now,
		ExpiresAt:   expiresAt,
	}
	if err := s.CreateWorkspaceWithToken(ws, bt); err != nil {
		return "", "", time.Time{}, err
	}
	return workspaceID, browserToken, expiresAt, nil
}

// IssueWorkspaceCLIToken mints a workspace-scoped rsctl bearer (requires valid workspace browser cookie proof server-side).
func IssueWorkspaceCLIToken(s *store.Store, workspaceID string, cfg config.Auth) (token string, expiresAt time.Time, err error) {
	token, err = GenerateToken()
	if err != nil {
		return "", time.Time{}, err
	}
	now := time.Now()
	expiresAt = now.Add(cfg.WorkspaceCLITokenTTL)
	err = s.PutOperatorToken(&store.OperatorToken{
		TokenHash:   HashToken(token),
		WorkspaceID: workspaceID,
		CreatedAt:   now,
		ExpiresAt:   expiresAt,
	})
	if err != nil {
		return "", time.Time{}, err
	}
	return token, expiresAt, nil
}

// IssueSessionCLIToken mints attach/kill token for one session.
func IssueSessionCLIToken(s *store.Store, workspaceID, sessionID string, cfg config.Auth) (token string, expiresAt time.Time, err error) {
	token, err = GenerateToken()
	if err != nil {
		return "", time.Time{}, err
	}
	now := time.Now()
	expiresAt = now.Add(cfg.SessionCLITokenTTL)
	err = s.PutOperatorToken(&store.OperatorToken{
		TokenHash:   HashToken(token),
		WorkspaceID: workspaceID,
		SessionID:   sessionID,
		CreatedAt:   now,
		ExpiresAt:   expiresAt,
	})
	if err != nil {
		return "", time.Time{}, err
	}
	return token, expiresAt, nil
}
