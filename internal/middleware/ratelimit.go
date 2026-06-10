package middleware

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

type visitor struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// RateLimit per client IP (RealIP) with configurable RPS and burst.
//
// WARNING: IP is extracted from X-Real-IP and X-Forwarded-For headers.
// Only deploy behind a trusted reverse proxy that strips/sets these headers;
// otherwise an attacker can spoof the header to bypass rate limiting.
func RateLimit(ctx context.Context, rps float64, burst int) func(http.Handler) http.Handler {
	var mu sync.Mutex
	visitors := make(map[string]*visitor)

	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				mu.Lock()
				for ip, v := range visitors {
					if time.Since(v.lastSeen) > 3*time.Minute {
						delete(visitors, ip)
					}
				}
				mu.Unlock()
			}
		}
	}()

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := r.RemoteAddr
			if x := r.Header.Get("X-Real-IP"); x != "" {
				ip = x
			} else if x := r.Header.Get("X-Forwarded-For"); x != "" {
				ip = strings.TrimSpace(strings.Split(x, ",")[0])
			}

			mu.Lock()
			v, exists := visitors[ip]
			if !exists {
				v = &visitor{limiter: rate.NewLimiter(rate.Limit(rps), burst)}
				visitors[ip] = v
			}
			v.lastSeen = time.Now()
			allow := v.limiter.Allow()
			mu.Unlock()

			if !allow {
				http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
