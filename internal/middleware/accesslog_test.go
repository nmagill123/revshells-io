package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	chlog "github.com/charmbracelet/log"
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
