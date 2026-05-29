package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/noahmagill/webhook-rev-shell/internal/broker"
	"github.com/noahmagill/webhook-rev-shell/internal/protocol"
	"github.com/noahmagill/webhook-rev-shell/internal/store"
)

type stubPresigner struct {
	url string
}

func (s stubPresigner) PresignGetURL(_ context.Context, platform string) (string, time.Time, error) {
	return s.url + "/" + platform, time.Now().Add(10 * time.Minute), nil
}

func TestAgentPresignHandler(t *testing.T) {
	st, err := store.Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	sess := &protocol.Session{
		ID:     "sess-1",
		Secret: "secret-1",
		State:  protocol.StateWaiting,
	}
	if err := st.PutSession(sess); err != nil {
		t.Fatal(err)
	}

	h := &AgentPresignHandler{
		B:         broker.New(st),
		Presigner: stubPresigner{url: "https://s3.example/presigned"},
	}

	body := bytes.NewBufferString(`{"platform":"linux-amd64"}`)
	req := httptest.NewRequest(http.MethodPost, "/s/sess-1/secret-1/agent-url", body)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "sess-1")
	rctx.URLParams.Add("secret", "secret-1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	h.AgentURL(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rr.Code, rr.Body.String())
	}

	var resp agentPresignResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.Platform != "linux-amd64" {
		t.Fatalf("platform = %q", resp.Platform)
	}
	if resp.URL != "https://s3.example/presigned/linux-amd64" {
		t.Fatalf("url = %q", resp.URL)
	}
}

func TestAgentPresignResponseDoesNotEscapeAmpersands(t *testing.T) {
	st, err := store.Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	if err := st.PutSession(&protocol.Session{ID: "sess-1", Secret: "secret-1", State: protocol.StateWaiting}); err != nil {
		t.Fatal(err)
	}

	h := &AgentPresignHandler{
		B: broker.New(st),
		Presigner: stubPresigner{url: "https://s3.example/obj?a=1&b=2"},
	}

	body := bytes.NewBufferString(`{"platform":"linux-amd64"}`)
	req := httptest.NewRequest(http.MethodPost, "/s/sess-1/secret-1/agent-url", body)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "sess-1")
	rctx.URLParams.Add("secret", "secret-1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	h.AgentURL(rr, req)
	out := rr.Body.String()
	if strings.Contains(out, `\u0026`) {
		t.Fatalf("json escaped ampersands in presigned url: %s", out)
	}
	if !strings.Contains(out, "a=1&b=2") {
		t.Fatalf("expected raw query string in json: %s", out)
	}
}

func TestAgentPresignHandlerBadSecret(t *testing.T) {
	st, err := store.Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	if err := st.PutSession(&protocol.Session{ID: "sess-1", Secret: "secret-1", State: protocol.StateWaiting}); err != nil {
		t.Fatal(err)
	}

	h := &AgentPresignHandler{B: broker.New(st), Presigner: stubPresigner{url: "https://s3.example"}}
	body := bytes.NewBufferString(`{"platform":"linux-amd64"}`)
	req := httptest.NewRequest(http.MethodPost, "/s/sess-1/wrong/agent-url", body)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "sess-1")
	rctx.URLParams.Add("secret", "wrong")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	h.AgentURL(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rr.Code)
	}
}

func TestAgentPresignHandlerInvalidPlatform(t *testing.T) {
	st, err := store.Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	if err := st.PutSession(&protocol.Session{ID: "sess-1", Secret: "secret-1", State: protocol.StateWaiting}); err != nil {
		t.Fatal(err)
	}

	h := &AgentPresignHandler{B: broker.New(st), Presigner: stubPresigner{url: "https://s3.example"}}
	body := bytes.NewBufferString(`{"platform":"../../etc/passwd"}`)
	req := httptest.NewRequest(http.MethodPost, "/s/sess-1/secret-1/agent-url", body)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "sess-1")
	rctx.URLParams.Add("secret", "secret-1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	h.AgentURL(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rr.Code)
	}
}
