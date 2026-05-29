package middleware

import (
	"net/url"
	"strings"
)

// RedactRequestPath returns a log-safe path; secrets and browser tokens are masked.
func RedactRequestPath(path, rawQuery string) string {
	path = RedactPathSegments(path)
	if rawQuery == "" {
		return path
	}
	q, err := url.ParseQuery(rawQuery)
	if err != nil {
		return path + "?[redacted]"
	}
	for key := range q {
		switch strings.ToLower(key) {
		case "t", "token":
			q.Set(key, "[redacted]")
		}
	}
	return path + "?" + q.Encode()
}

// RedactPathSegments masks the session secret in /s/{id}/{secret}/... URLs.
func RedactPathSegments(path string) string {
	parts := strings.Split(path, "/")
	if len(parts) >= 4 && parts[1] == "s" && parts[3] != "" {
		parts[3] = "[redacted]"
	}
	return strings.Join(parts, "/")
}
