package web

import (
	"fmt"
	"io/fs"
	"net/http"
	"strings"
)

var articleSlugs = map[string]string{
	"bash-reverse-shell":      "articles/bash-reverse-shell.html",
	"php-reverse-shell":       "articles/php-reverse-shell.html",
	"python-reverse-shell":    "articles/python-reverse-shell.html",
	"javascript-reverse-shell": "articles/javascript-reverse-shell.html",
	"msfvenom-alternative":    "articles/msfvenom-alternative.html",
}

// ServeArticle serves an embedded SEO guide page by slug.
func ServeArticle(w http.ResponseWriter, slug string) {
	path, ok := articleSlugs[slug]
	if !ok {
		http.NotFound(w, nil)
		return
	}
	ServePage(w, path)
}

// ServeArticlesIndex lists available guides.
func ServeArticlesIndex(w http.ResponseWriter) {
	ServePage(w, "articles/index.html")
}

// ArticlePaths returns slugs for sitemap or tests.
func ArticlePaths() []string {
	out := make([]string, 0, len(articleSlugs))
	for slug := range articleSlugs {
		out = append(out, slug)
	}
	return out
}

// ServeSitemap writes a minimal sitemap including article URLs.
func ServeSitemap(w http.ResponseWriter, baseURL string) {
	baseURL = strings.TrimRight(baseURL, "/")
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">` + "\n")
	writeSitemapURL(&b, baseURL+"/")
	writeSitemapURL(&b, baseURL+"/articles")
	for _, slug := range ArticlePaths() {
		writeSitemapURL(&b, fmt.Sprintf("%s/articles/%s", baseURL, slug))
	}
	b.WriteString("</urlset>\n")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Write([]byte(b.String()))
}

// ServeRobots writes robots.txt with a sitemap reference.
func ServeRobots(w http.ResponseWriter, baseURL string) {
	baseURL = strings.TrimRight(baseURL, "/")
	body := "User-agent: *\nAllow: /\n\nSitemap: " + baseURL + "/sitemap.xml\n"
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte(body))
}

func writeSitemapURL(b *strings.Builder, loc string) {
	fmt.Fprintf(b, "  <url><loc>%s</loc></url>\n", loc)
}

// ArticleExists reports whether a slug is registered.
func ArticleExists(slug string) bool {
	_, ok := articleSlugs[slug]
	return ok
}

func init() {
	for _, path := range articleSlugs {
		if _, err := fs.Stat(Static, "static/"+path); err != nil {
			panic("missing article: static/" + path)
		}
	}
	if _, err := fs.Stat(Static, "static/articles/index.html"); err != nil {
		panic("missing article index: static/articles/index.html")
	}
}
