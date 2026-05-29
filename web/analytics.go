package web

import (
	"os"
	"strings"
)

const analyticsPlaceholder = "<!-- __ANALYTICS_LOCAL__ -->"

var analyticsSnippet string

// InitAnalytics loads an optional HTML snippet injected into pages at analyticsPlaceholder.
// explicitPath is used when set; otherwise common local/prod paths are tried silently.
func InitAnalytics(explicitPath string) string {
	if explicitPath != "" {
		if err := loadAnalyticsFile(explicitPath); err == nil {
			return explicitPath
		}
		return ""
	}
	for _, path := range []string{
		"web/static/analytics.local.html",
		"/data/analytics.local.html",
	} {
		if err := loadAnalyticsFile(path); err == nil {
			return path
		}
	}
	return ""
}

func loadAnalyticsFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	snippet := strings.TrimSpace(string(data))
	if snippet == "" {
		analyticsSnippet = ""
		return nil
	}
	analyticsSnippet = snippet + "\n"
	return nil
}

func injectAnalytics(body string) string {
	if analyticsSnippet == "" {
		return strings.ReplaceAll(body, analyticsPlaceholder, "")
	}
	return strings.ReplaceAll(body, analyticsPlaceholder, analyticsSnippet)
}
