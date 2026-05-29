package middleware

import (
	"strings"
	"testing"
)

func TestRedactPathSegments(t *testing.T) {
	in := "/s/sess-uuid/secret-value/register"
	want := "/s/sess-uuid/[redacted]/register"
	if got := RedactPathSegments(in); got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if RedactPathSegments("/foo/bar") != "/foo/bar" {
		t.Fatal("expected unchanged path")
	}
}

func TestRedactRequestPathQueryToken(t *testing.T) {
	got := RedactRequestPath("/abc", "t=browser-token&foo=1")
	if !strings.Contains(got, "foo=1") {
		t.Fatalf("expected foo param preserved: %q", got)
	}
	if strings.Contains(got, "browser-token") {
		t.Fatalf("token not redacted: %q", got)
	}
}
