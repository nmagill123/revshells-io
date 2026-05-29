package middleware

import (
	"bufio"
	"net"
	"net/http"
	"strings"
	"time"

	chlog "github.com/charmbracelet/log"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type responseRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (r *responseRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	n, err := r.ResponseWriter.Write(b)
	r.bytes += n
	return n, err
}

func (r *responseRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := r.ResponseWriter.(http.Hijacker); ok {
		return h.Hijack()
	}
	return nil, nil, http.ErrNotSupported
}

func (r *responseRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// AccessLog emits a debug line per HTTP request when enabled (use with --verbose).
func AccessLog(enabled bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if !enabled {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if shouldSkipAccessLog(r) {
				next.ServeHTTP(w, r)
				return
			}

			start := time.Now()
			rec := &responseRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rec, r)

			reqID := middleware.GetReqID(r.Context())
			path := RedactRequestPath(r.URL.Path, r.URL.RawQuery)
			route := ""
			if rc := chi.RouteContext(r.Context()); rc != nil {
				if p := rc.RoutePattern(); p != "" {
					route = p
				}
			}
			if route != "" {
				path = path + " route=" + route
			}

			fields := []any{
				"request_id", reqID,
				"method", r.Method,
				"path", path,
				"status", rec.status,
				"duration_ms", time.Since(start).Milliseconds(),
				"bytes", rec.bytes,
				"remote_ip", clientIP(r),
			}
			if ua := r.Header.Get("User-Agent"); ua != "" {
				fields = append(fields, "user_agent", ua)
			}
			if ref := r.Header.Get("Referer"); ref != "" {
				fields = append(fields, "referer", ref)
			}
			if r.TLS != nil {
				fields = append(fields, "tls", true)
			}
			if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
				fields = append(fields, "x_forwarded_for", fwd)
			}
			if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
				fields = append(fields, "x_forwarded_proto", proto)
			}
			if sess := sessionIDFromRequest(r); sess != "" {
				fields = append(fields, "session_id", sess)
			}

			chlog.Debug("http request", fields...)
		})
	}
}

func shouldSkipAccessLog(r *http.Request) bool {
	if strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		return true
	}
	if rc := chi.RouteContext(r.Context()); rc != nil {
		switch rc.RoutePattern() {
		case "/s/{id}/{secret}/poll":
			return true
		}
	}
	return false
}

func sessionIDFromRequest(r *http.Request) string {
	if id := r.URL.Query().Get("session_id"); id != "" {
		return id
	}
	parts := splitPath(r.URL.Path)
	if len(parts) >= 2 && parts[0] == "s" {
		return parts[1]
	}
	if len(parts) >= 1 && parts[0] != "" {
		switch parts[0] {
		case "web", "api", "static", "articles", "sitemap.xml", "robots.txt":
		default:
			return parts[0]
		}
	}
	if len(parts) >= 3 && parts[0] == "web" && parts[1] == "sessions" {
		return parts[2]
	}
	return ""
}

func splitPath(path string) []string {
	path = strings.Trim(path, "/")
	if path == "" {
		return nil
	}
	return strings.Split(path, "/")
}

func clientIP(r *http.Request) string {
	if x := r.Header.Get("X-Real-IP"); x != "" {
		return x
	}
	if x := r.Header.Get("X-Forwarded-For"); x != "" {
		return strings.TrimSpace(strings.Split(x, ",")[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}
