package api

import (
	"io/fs"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"typstpad/internal/auth"
	"typstpad/internal/blob"
	"typstpad/internal/collab"
	"typstpad/internal/compile"
	"typstpad/internal/config"
	"typstpad/internal/mail"
	mcpsrv "typstpad/internal/mcp"
	"typstpad/internal/settings"
	"typstpad/internal/store"
	"typstpad/internal/versions"
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
	r.Use(middleware.Timeout(120 * time.Second))
	r.Use(s.Auth.Authenticate)

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := s.Store.Pool.Ping(r.Context()); err != nil {
			writeErr(w, http.StatusServiceUnavailable, "db unreachable")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	r.Route("/api", func(r chi.Router) {
		r.Route("/auth", func(r chi.Router) {
			r.Post("/register", s.handleRegister)
			r.Post("/login", s.handleLogin)
			r.Post("/logout", s.handleLogout)
			r.Get("/me", s.handleMe)
			r.Get("/config", s.handleAuthConfig)
			r.Get("/oidc/login", s.handleOIDCLogin)
			r.Get("/oidc/callback", s.handleOIDCCallback)
			r.Get("/verify-email", s.handleVerifyEmail)
			r.Post("/resend-verification", s.handleResendVerification)
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
