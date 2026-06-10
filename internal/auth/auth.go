package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"net/http"
	"time"

	"github.com/noahmagill/webhook-rev-shell/internal/store"
)

func GenerateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func HashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

func browserTokenValue(r *http.Request) string {
	if t := r.URL.Query().Get("t"); t != "" {
		return t
	}
	if c, err := r.Cookie(CookieSession); err == nil {
		return c.Value
	}
	return ""
}

// ValidateBrowserTokenForSession checks query ?t= or rs_token cookie grants access to sessionID.
func ValidateBrowserTokenForSession(s *store.Store, r *http.Request, sessionID string) error {
	token := browserTokenValue(r)
	if token == "" {
		return fmt.Errorf("no token")
	}
	bt, err := s.GetBrowserToken(token)
	if err != nil {
		return fmt.Errorf("invalid token")
	}
	if time.Now().After(bt.ExpiresAt) {
		return fmt.Errorf("expired")
	}
	if subtle.ConstantTimeCompare([]byte(bt.SessionID), []byte(sessionID)) != 1 {
		return fmt.Errorf("wrong session")
	}
	return nil
}
