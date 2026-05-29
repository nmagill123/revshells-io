package transport

import (
	"encoding/json"
	"net/http"
	"time"

	chlog "github.com/charmbracelet/log"
	"github.com/go-chi/chi/v5"
	"github.com/noahmagill/webhook-rev-shell/internal/agents"
	"github.com/noahmagill/webhook-rev-shell/internal/broker"
	"github.com/noahmagill/webhook-rev-shell/internal/protocol"
)

type AgentPresignHandler struct {
	B         *broker.Broker
	Presigner agents.Presigner
}

type agentPresignRequest struct {
	Platform string `json:"platform"`
}

type agentPresignResponse struct {
	URL       string    `json:"url"`
	ExpiresAt time.Time `json:"expires_at"`
	Platform  string    `json:"platform"`
}

func (h *AgentPresignHandler) AgentURL(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	sessionID := chi.URLParam(r, "id")
	secret := chi.URLParam(r, "secret")

	sess, err := h.B.Store().GetSession(sessionID)
	if err != nil || sess.Secret != secret {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if sess.State == protocol.StateExpired || sess.State == protocol.StateKilled {
		http.Error(w, "session closed", http.StatusGone)
		return
	}

	var body agentPresignRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if !agents.ValidPlatform(body.Platform) {
		http.Error(w, "invalid platform", http.StatusBadRequest)
		return
	}

	url, expires, err := h.Presigner.PresignGetURL(r.Context(), body.Platform)
	if err != nil {
		http.Error(w, "presign failed", http.StatusInternalServerError)
		return
	}

	h.B.Touch(sessionID)

	chlog.Debug("agent presign issued",
		"session_id", sessionID,
		"platform", body.Platform,
		"expires_at", expires.UTC(),
		"remote_ip", requestIP(r),
	)

	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(agentPresignResponse{
		URL:       url,
		ExpiresAt: expires.UTC(),
		Platform:  body.Platform,
	})
}
