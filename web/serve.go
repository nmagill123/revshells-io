package web

import (
	"io/fs"
	"net/http"
	"strings"

	"github.com/noahmagill/webhook-rev-shell/internal/version"
)

func Version() string {
	return version.Version
}

func ServeStatic(w http.ResponseWriter, r *http.Request, name string) {
	data, err := fs.ReadFile(Static, "static/"+name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if strings.HasSuffix(name, ".css") {
		w.Header().Set("Content-Type", "text/css")
	} else if strings.HasSuffix(name, ".js") {
		w.Header().Set("Content-Type", "application/javascript")
	} else if strings.HasSuffix(name, ".svg") {
		w.Header().Set("Content-Type", "image/svg+xml")
	}
	w.Write(data)
}

func ServePage(w http.ResponseWriter, name string) {
	data, err := fs.ReadFile(Static, "static/"+name)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	body := strings.ReplaceAll(string(data), "__RSD_VERSION__", version.Version)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(body))
}
