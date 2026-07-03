package api

import (
	"io/fs"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/ladderlogix/typstpad/internal/auth"
	"github.com/ladderlogix/typstpad/internal/blob"
	"github.com/ladderlogix/typstpad/internal/collab"
	"github.com/ladderlogix/typstpad/internal/compile"
	"github.com/ladderlogix/typstpad/internal/config"
	"github.com/ladderlogix/typstpad/internal/mail"
	mcpsrv "github.com/ladderlogix/typstpad/internal/mcp"
	"github.com/ladderlogix/typstpad/internal/metrics"
	"github.com/ladderlogix/typstpad/internal/ratelimit"
	"github.com/ladderlogix/typstpad/internal/settings"
	"github.com/ladderlogix/typstpad/internal/store"
	"github.com/ladderlogix/typstpad/internal/versions"
)

// selfURL derives the loopback URL the MCP proxy uses to reach this server.
func selfURL(addr string) string {
	if strings.HasPrefix(addr, ":") {
		return "http://127.0.0.1" + addr
	}
	return "http://" + addr
}

type Server struct {
	Cfg      *config.Config
	Store    *store.Store
	Auth     *auth.Auth
	Blob     *blob.Store
	Hub      *Hub
	Collab   *collab.Client
	Compiler *compile.Compiler
	Versions *versions.Snapshotter
	Mailer   *mail.Mailer
	Settings *settings.Service
	SPA      fs.FS
	// OnDocStored is invoked whenever the collab sidecar persists a doc;
	// the snapshotter uses it to mark projects dirty.
	OnDocStored func(projectID string)
	// OnFirstUser runs after the bootstrap admin registers (seeds templates).
	OnFirstUser func()
}

func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(metrics.Middleware)
	r.Use(middleware.Timeout(120 * time.Second))
	r.Use(s.Auth.Authenticate)

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := s.Store.Pool.Ping(r.Context()); err != nil {
			writeErr(w, http.StatusServiceUnavailable, "db unreachable")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	// Prometheus scrape endpoint (bearer-token gated; internal only).
	r.Handle("/metrics", metrics.Handler(s.Cfg.MetricsToken))

	r.Route("/api", func(r chi.Router) {
		r.Route("/auth", func(r chi.Router) {
			// Per-IP rate limits: modest on credential checks, strict on the
			// endpoints that send email (to prevent mail-bombing / enumeration).
			ipKey := ratelimit.ClientIP
			authLimit := ratelimit.New(20, time.Minute).Middleware(ipKey)
			emailLimit := ratelimit.New(6, time.Minute).Middleware(ipKey)

			r.With(authLimit).Post("/register", s.handleRegister)
			r.With(authLimit).Post("/login", s.handleLogin)
			r.Post("/logout", s.handleLogout)
			r.Get("/me", s.handleMe)
			r.Get("/config", s.handleAuthConfig)
			r.Get("/oidc/login", s.handleOIDCLogin)
			r.Get("/oidc/callback", s.handleOIDCCallback)
			r.Get("/verify-email", s.handleVerifyEmail)
			r.With(emailLimit).Post("/resend-verification", s.handleResendVerification)
			r.With(emailLimit).Post("/forgot-password", s.handleForgotPassword)
			r.With(authLimit).Post("/reset-password", s.handleResetPassword)
			r.With(auth.RequireUser).Patch("/me", s.handleUpdateProfile)
			r.With(auth.RequireUser, authLimit).Post("/change-password", s.handleChangePassword)
		})
		s.mountAuthedRoutes(r)

		// MCP streamable-HTTP endpoint for remote AI agents (PAT bearer auth;
		// tools proxy to this same server so authz is uniform).
		r.Handle("/mcp", mcpsrv.HTTPHandler(selfURL(s.Cfg.Addr)))
	})

	s.mountInternalRoutes(r)

	// Websocket sync traffic is reverse-proxied to the collab sidecar; auth
	// happens inside the sidecar via the JWT from /collab-token.
	if proxy, err := s.Collab.WSProxy(); err == nil {
		r.Handle("/collab", proxy)
		r.Handle("/collab/*", proxy)
	}

	s.mountSPA(r)
	return r
}

// mountSPA serves the embedded React build, falling back to index.html for
// client-side routes.
func (s *Server) mountSPA(r chi.Router) {
	fileServer := http.FileServer(http.FS(s.SPA))
	r.NotFound(func(w http.ResponseWriter, req *http.Request) {
		if strings.HasPrefix(req.URL.Path, "/api/") {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		path := strings.TrimPrefix(req.URL.Path, "/")
		if path != "" {
			if f, err := s.SPA.Open(path); err == nil {
				f.Close()
				fileServer.ServeHTTP(w, req)
				return
			}
		}
		// SPA fallback
		req.URL.Path = "/"
		fileServer.ServeHTTP(w, req)
	})
}
