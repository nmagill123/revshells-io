package middleware

import (
	"context"
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// RateLimitByKey applies a token bucket per key returned by keyFunc.
//
// WARNING: If keyFunc derives the key from request headers (e.g. X-Real-IP),
// only deploy behind a trusted reverse proxy that strips/sets those headers.
func RateLimitByKey(ctx context.Context, keyFunc func(*http.Request) string, rps float64, burst int) func(http.Handler) http.Handler {
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
				for key, v := range visitors {
					if time.Since(v.lastSeen) > 3*time.Minute {
						delete(visitors, key)
					}
				}
				mu.Unlock()
			}
		}
	}()

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := keyFunc(r)
			if key == "" {
				next.ServeHTTP(w, r)
				return
			}

			mu.Lock()
			v, exists := visitors[key]
			if !exists {
				v = &visitor{limiter: rate.NewLimiter(rate.Limit(rps), burst)}
				visitors[key] = v
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
