package config

import (
	"net/url"
	"time"
)

// Auth holds authorization-related settings (flags + optional file overrides).
type Auth struct {
	WorkspaceBrowserTokenTTL time.Duration
	WorkspaceCLITokenTTL     time.Duration
	SessionBrowserTokenTTL   time.Duration
	SessionCLITokenTTL       time.Duration
	MaxSessionsPerWorkspace  int
	AllowedOrigins           []string
}

func DefaultAuth(publicURL string) Auth {
	c := Auth{
		WorkspaceBrowserTokenTTL: 24 * time.Hour,
		WorkspaceCLITokenTTL:     24 * time.Hour,
		SessionBrowserTokenTTL:   24 * time.Hour,
		SessionCLITokenTTL:       6 * time.Hour,
		MaxSessionsPerWorkspace:  12,
	}
	if u, err := url.Parse(publicURL); err == nil && u.Host != "" {
		c.AllowedOrigins = []string{u.Host}
	}
	return c
}
