package web

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestServeStaticSetsCacheHeaders(t *testing.T) {
	req := httptest.NewRequest("GET", "/static/favicon.svg", nil)
	rr := httptest.NewRecorder()
	ServeStatic(rr, req, "favicon.svg")

	if got := rr.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Fatalf("unexpected cache-control: %q", got)
	}
	if got := rr.Header().Get("Content-Type"); got != "image/svg+xml" {
		t.Fatalf("unexpected content-type: %q", got)
	}
}

func TestServePageSetsNoStore(t *testing.T) {
	rr := httptest.NewRecorder()
	ServePage(rr, "hub.html")

	if got := rr.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("unexpected cache-control: %q", got)
	}
	if !strings.Contains(rr.Body.String(), "<!DOCTYPE html>") {
		t.Fatal("expected html body")
	}
}
