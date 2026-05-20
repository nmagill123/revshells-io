package transport

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/noahmagill/webhook-rev-shell/internal/broker"
	"github.com/noahmagill/webhook-rev-shell/internal/operatorinput"
	"nhooyr.io/websocket"
)

type WSOperatorHandler struct {
	B              *broker.Broker
	OriginPatterns []string
}

func (h *WSOperatorHandler) Attach(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "id")

	op, room, err := h.B.AddOperator(sessionID)
	if err != nil {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: h.OriginPatterns,
	})
	if err != nil {
		h.B.RemoveOperator(sessionID, op.ID)
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "")
	defer h.B.RemoveOperator(sessionID, op.ID)

	ctx, cancel := context.WithCancel(op.Ctx)
	defer cancel()
	go func() {
		select {
		case <-op.Done:
			cancel()
		case <-ctx.Done():
		}
	}()

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
			}
		}
	}()

	for _, chunk := range room.Transcript.Snapshot() {
		if err := conn.Write(ctx, websocket.MessageBinary, chunk); err != nil {
			return
		}
	}

	go func() {
		defer cancel()
		for {
			typ, data, err := conn.Read(ctx)
			if err != nil {
				return
			}
			if typ == websocket.MessageText && !operatorinput.IsResizeMessage(data) {
				// Browser operators should only send resize control frames as text.
				// Drop any other text frames so they never reach the target shell.
				continue
			}
			room.SendToClaimed(data)
		}
	}()

	for {
		select {
		case data := <-op.Send:
			if err := conn.Write(ctx, websocket.MessageBinary, data); err != nil {
				return
			}
		case <-ctx.Done():
			return
		}
	}
}
