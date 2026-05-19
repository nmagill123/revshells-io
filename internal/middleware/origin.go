package middleware

import (
	"net/http"
	"net/url"
	"strings"
)

// RequireSameOrigin rejects cross-site cookie-authenticated mutating requests.
func RequireSameOrigin(allowedHosts []string) func(http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(allowedHosts))
	for _, h := range allowedHosts {
		h = strings.ToLower(strings.TrimSpace(h))
		if h != "" {
			allowed[h] = struct{}{}
		}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
				next.ServeHTTP(w, r)
				return
			}
			host := requestHost(r)
			if host == "" {
				http.Error(w, "missing origin", http.StatusForbidden)
				return
			}
			if _, ok := allowed[strings.ToLower(host)]; !ok {
				http.Error(w, "origin not allowed", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func requestHost(r *http.Request) string {
	if o := r.Header.Get("Origin"); o != "" {
		if u, err := url.Parse(o); err == nil && u.Host != "" {
			return u.Host
		}
	}
	if ref := r.Header.Get("Referer"); ref != "" {
		if u, err := url.Parse(ref); err == nil && u.Host != "" {
			return u.Host
		}
	}
	return r.Host
}
