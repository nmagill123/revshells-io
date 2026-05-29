package middleware

import (
	"bufio"
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	chlog "github.com/charmbracelet/log"
	"github.com/go-chi/chi/v5"
)

func TestAccessLogEmitsOnVerbose(t *testing.T) {
	prev := chlog.Default()
	buf := &logBuffer{}
	chlog.SetDefault(chlog.NewWithOptions(buf, chlog.Options{Level: chlog.DebugLevel}))
	defer chlog.SetDefault(prev)

	handler := AccessLog(true)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/s/sess/secret/register", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if !stringsContains(buf.String(), "http request") {
		t.Fatalf("expected access log line, got %q", buf.String())
	}
	if stringsContains(buf.String(), "secret") {
		t.Fatalf("secret should be redacted: %q", buf.String())
	}
}

func TestAccessLogSkipsWebSocketAndPoll(t *testing.T) {
	prev := chlog.Default()
	buf := &logBuffer{}
	chlog.SetDefault(chlog.NewWithOptions(buf, chlog.Options{Level: chlog.DebugLevel}))
	defer chlog.SetDefault(prev)

	handler := AccessLog(true)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	cases := []struct {
		name string
		req  *http.Request
	}{
		{
			name: "websocket upgrade",
			req: func() *http.Request {
				req := httptest.NewRequest(http.MethodGet, "/sess/attach", nil)
				req.Header.Set("Upgrade", "websocket")
				return req
			}(),
		},
		{
			name: "target poll",
			req: func() *http.Request {
				req := httptest.NewRequest(http.MethodGet, "/s/sess/secret/poll", nil)
				rctx := chi.NewRouteContext()
				rctx.RoutePatterns = []string{"/s/{id}/{secret}/poll"}
				return req.WithContext(contextWithChiRoute(req.Context(), rctx))
			}(),
		},
	}

	for _, tc := range cases {
		buf.data = nil
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, tc.req)
		if stringsContains(buf.String(), "http request") {
			t.Fatalf("%s: expected no access log, got %q", tc.name, buf.String())
		}
	}
}

func TestAccessLogPreservesHijacker(t *testing.T) {
	handler := AccessLog(true)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := w.(http.Hijacker); !ok {
			t.Fatal("ResponseWriter should implement Hijacker through access log wrapper")
		}
	}))

	req := httptest.NewRequest(http.MethodGet, "/sess/attach", nil)
	req.Header.Set("Upgrade", "websocket")
	rr := &hijackRecorder{ResponseRecorder: httptest.NewRecorder()}
	handler.ServeHTTP(rr, req)
}

type hijackRecorder struct {
	*httptest.ResponseRecorder
}

func (h *hijackRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return nil, nil, nil
}

func contextWithChiRoute(ctx context.Context, rctx *chi.Context) context.Context {
	return context.WithValue(ctx, chi.RouteCtxKey, rctx)
}

type logBuffer struct {
	data []byte
}

func (b *logBuffer) Write(p []byte) (int, error) {
	b.data = append(b.data, p...)
	return len(p), nil
}

func (b *logBuffer) String() string {
	return string(b.data)
}

func stringsContains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
