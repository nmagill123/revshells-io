package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRateLimitByKey(t *testing.T) {
	key := "session-1"
	limit := RateLimitByKey(func(r *http.Request) string {
		return r.Header.Get("X-Test-Key")
	}, 1, 1)

	handler := limit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Test-Key", key)

	rr1 := httptest.NewRecorder()
	handler.ServeHTTP(rr1, req)
	if rr1.Code != http.StatusOK {
		t.Fatalf("first request status = %d", rr1.Code)
	}

	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req)
	if rr2.Code != http.StatusTooManyRequests {
		t.Fatalf("second request status = %d, want 429", rr2.Code)
	}
}
