package web

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestServeArticleKnownSlug(t *testing.T) {
	rr := httptest.NewRecorder()
	ServeArticle(rr, "bash-reverse-shell")
	if rr.Code != 200 {
		t.Fatalf("status = %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "Bash reverse shell") {
		t.Fatal("expected article body")
	}
	if !strings.Contains(rr.Body.String(), "SESSION-ID") {
		t.Fatal("expected session placeholder in examples")
	}
}

func TestServeArticleUnknownSlug(t *testing.T) {
	rr := httptest.NewRecorder()
	ServeArticle(rr, "not-a-real-slug")
	if rr.Code != 404 {
		t.Fatalf("status = %d, want 404", rr.Code)
	}
}

func TestServeArticlesIndex(t *testing.T) {
	rr := httptest.NewRecorder()
	ServeArticlesIndex(rr)
	if rr.Code != 200 {
		t.Fatalf("status = %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "msfvenom-alternative") {
		t.Fatal("expected index to link msfvenom guide")
	}
}

func TestServeSitemap(t *testing.T) {
	rr := httptest.NewRecorder()
	ServeSitemap(rr, "https://revshells.io")
	body := rr.Body.String()
	if !strings.Contains(body, "https://revshells.io/articles/bash-reverse-shell") {
		t.Fatalf("sitemap missing article url: %s", body)
	}
	if !strings.Contains(body, "<loc>https://revshells.io/articles</loc>") {
		t.Fatalf("sitemap missing articles index: %s", body)
	}
}

func TestServeRobots(t *testing.T) {
	rr := httptest.NewRecorder()
	ServeRobots(rr, "https://revshells.io")
	body := rr.Body.String()
	if !strings.Contains(body, "Sitemap: https://revshells.io/sitemap.xml") {
		t.Fatalf("robots.txt missing sitemap: %s", body)
	}
	if !strings.Contains(body, "Allow: /") {
		t.Fatal("robots.txt should allow crawlers")
	}
}
