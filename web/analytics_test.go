package web

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitAnalyticsInjectsSnippet(t *testing.T) {
	t.Cleanup(func() { analyticsSnippet = "" })

	dir := t.TempDir()
	path := filepath.Join(dir, "analytics.local.html")
	const snippet = "<script>gtag('config', 'G-TEST');</script>"
	if err := os.WriteFile(path, []byte(snippet), 0o644); err != nil {
		t.Fatal(err)
	}

	if got := InitAnalytics(path); got != path {
		t.Fatalf("InitAnalytics() = %q, want %q", got, path)
	}

	body := injectAnalytics("<head><!-- __ANALYTICS_LOCAL__ --></head>")
	if !strings.Contains(body, snippet) {
		t.Fatalf("expected snippet in body, got %q", body)
	}
}

func TestInitAnalyticsEmptyPlaceholderRemoved(t *testing.T) {
	t.Cleanup(func() { analyticsSnippet = "" })

	body := injectAnalytics("<head><!-- __ANALYTICS_LOCAL__ --></head>")
	if strings.Contains(body, analyticsPlaceholder) {
		t.Fatalf("placeholder should be removed when snippet empty, got %q", body)
	}
}
