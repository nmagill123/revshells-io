package protocol

import "time"

// SessionPublic is returned to operators and the web UI (no target secret).
type SessionPublic struct {
	ID               string          `json:"id"`
	Name             string          `json:"name"`
	OwnerWorkspaceID string          `json:"owner_workspace_id,omitempty"`
	CreatedAt        time.Time       `json:"created_at"`
	LastActivity     time.Time       `json:"last_activity"`
	State            SessionState    `json:"state"`
	LastTarget       *TargetSnapshot `json:"last_target,omitempty"`
}

func SessionToPublic(s *Session) SessionPublic {
	if s == nil {
		return SessionPublic{}
	}
	return SessionPublic{
		ID:               s.ID,
		Name:             s.Name,
		OwnerWorkspaceID: s.OwnerWorkspaceID,
		CreatedAt:        s.CreatedAt,
		LastActivity:     s.LastActivity,
		State:            s.State,
		LastTarget:       s.LastTarget,
	}
}
