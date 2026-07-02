package api

import (
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"typstpad/internal/auth"
	"typstpad/internal/settings"
	"typstpad/internal/store"
)

// Personal access tokens (for the CLI, API scripts and MCP clients).

var validScopes = []string{"read", "write", "compile", "admin"}

func (s *Server) handleListTokens(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFrom(r.Context())
	tokens, err := s.Store.ListAPITokens(r.Context(), u.ID)
	if err != nil {
		fail(w, err)
		return
	}
	if tokens == nil {
		tokens = []*store.APIToken{}
	}
	writeJSON(w, http.StatusOK, tokens)
}

func (s *Server) handleCreateToken(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFrom(r.Context())
	// Only interactive sessions may mint tokens — a PAT must not create PATs.
	if c, err := r.Cookie(auth.SessionCookie); err != nil || c.Value == "" {
		writeErr(w, http.StatusForbidden, "tokens can only be created from a browser session")
		return
	}
	var req struct {
		Name      string     `json:"name"`
		Scopes    []string   `json:"scopes"`
		ExpiresAt *time.Time `json:"expiresAt"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if req.Name == "" {
		writeErr(w, http.StatusBadRequest, "name required")
		return
	}
	if len(req.Scopes) == 0 {
		req.Scopes = []string{"read", "write", "compile"}
	}
	for _, sc := range req.Scopes {
		if !slices.Contains(validScopes, sc) {
			writeErr(w, http.StatusBadRequest, "invalid scope: "+sc)
			return
		}
		if sc == "admin" && !u.IsAdmin {
			writeErr(w, http.StatusForbidden, "admin scope requires an admin account")
			return
		}
	}
	token, hash, err := auth.NewToken("tfp_")
	if err != nil {
		fail(w, err)
		return
	}
	t, err := s.Store.CreateAPIToken(r.Context(), u.ID, req.Name, hash, req.Scopes, req.ExpiresAt)
	if err != nil {
		fail(w, err)
		return
	}
	t.Token = token // shown once
	writeJSON(w, http.StatusCreated, t)
}

func (s *Server) handleDeleteToken(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFrom(r.Context())
	if err := s.Store.DeleteAPIToken(r.Context(), chi.URLParam(r, "tokenID"), u.ID); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// Admin

func (s *Server) handleAdminListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := s.Store.ListUsers(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, users)
}

func (s *Server) handleAdminUpdateUser(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFrom(r.Context())
	targetID := chi.URLParam(r, "userID")
	var req struct {
		IsAdmin *bool `json:"isAdmin"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if req.IsAdmin != nil {
		if targetID == u.ID && !*req.IsAdmin {
			writeErr(w, http.StatusBadRequest, "cannot remove your own admin role")
			return
		}
		if err := s.Store.SetUserAdmin(r.Context(), targetID, *req.IsAdmin); err != nil {
			fail(w, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleAdminDeleteUser(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFrom(r.Context())
	targetID := chi.URLParam(r, "userID")
	if targetID == u.ID {
		writeErr(w, http.StatusBadRequest, "cannot delete your own account")
		return
	}
	if err := s.Store.DeleteUser(r.Context(), targetID); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleAdminGetSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.Settings.AdminView())
}

func (s *Server) handleAdminStats(w http.ResponseWriter, r *http.Request) {
	stats, err := s.Store.Stats(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"users":          stats.Users,
		"projects":       stats.Projects,
		"documents":      stats.Documents,
		"templates":      stats.Templates,
		"teams":          stats.Teams,
		"activeSessions": stats.ActiveSessions,
		"diskBytes":      dirSize(s.Cfg.DataDir),
	})
}

// dirSize sums file sizes under a directory (best-effort).
func dirSize(dir string) int64 {
	var total int64
	_ = filepath.Walk(dir, func(_ string, info os.FileInfo, err error) error {
		if err == nil && info != nil && !info.IsDir() {
			total += info.Size()
		}
		return nil
	})
	return total
}

// handleAdminPutSettings updates any provided setting. Secret fields (SMTP
// password, OIDC client secret) are only changed when a non-empty value is
// sent; sending "" leaves them unchanged, and null clears them.
func (s *Server) handleAdminPutSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AllowRegistration        *bool   `json:"allowRegistration"`
		SignupAllowlist          *string `json:"signupAllowlist"`
		RequireEmailVerification *bool   `json:"requireEmailVerification"`
		SMTPHost                 *string `json:"smtpHost"`
		SMTPPort                 *int    `json:"smtpPort"`
		SMTPUsername             *string `json:"smtpUsername"`
		SMTPPassword             *string `json:"smtpPassword"`
		SMTPFrom                 *string `json:"smtpFrom"`
		SMTPFromName             *string `json:"smtpFromName"`
		OIDCIssuer               *string `json:"oidcIssuer"`
		OIDCClientID             *string `json:"oidcClientId"`
		OIDCClientSecret         *string `json:"oidcClientSecret"`
		OIDCScopes               *string `json:"oidcScopes"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	ctx := r.Context()
	set := func(key, val string) bool {
		if err := s.Settings.Set(ctx, key, val); err != nil {
			fail(w, err)
			return false
		}
		return true
	}
	setBool := func(key string, v *bool) bool {
		if v == nil {
			return true
		}
		return set(key, boolStr(*v))
	}
	setStr := func(key string, v *string) bool {
		if v == nil {
			return true
		}
		return set(key, strings.TrimSpace(*v))
	}
	// Secret: only overwrite when a non-empty value is supplied.
	setSecret := func(key string, v *string) bool {
		if v == nil || *v == "" {
			return true
		}
		return set(key, *v)
	}

	oidcBefore := s.Settings.OIDCIssuer() + "|" + s.Settings.OIDCClientID() + "|" + s.Settings.OIDCClientSecret() + "|" + s.Settings.OIDCScopes()

	if req.SMTPPort != nil && !set(settings.KeySMTPPort, strconv.Itoa(*req.SMTPPort)) {
		return
	}
	if !setBool(settings.KeyAllowRegistration, req.AllowRegistration) ||
		!setStr(settings.KeySignupAllowlist, req.SignupAllowlist) ||
		!setBool(settings.KeyRequireEmailVerification, req.RequireEmailVerification) ||
		!setStr(settings.KeySMTPHost, req.SMTPHost) ||
		!setStr(settings.KeySMTPUsername, req.SMTPUsername) ||
		!setSecret(settings.KeySMTPPassword, req.SMTPPassword) ||
		!setStr(settings.KeySMTPFrom, req.SMTPFrom) ||
		!setStr(settings.KeySMTPFromName, req.SMTPFromName) ||
		!setStr(settings.KeyOIDCIssuer, req.OIDCIssuer) ||
		!setStr(settings.KeyOIDCClientID, req.OIDCClientID) ||
		!setSecret(settings.KeyOIDCClientSecret, req.OIDCClientSecret) ||
		!setStr(settings.KeyOIDCScopes, req.OIDCScopes) {
		return
	}

	// Re-initialize OIDC if any OIDC field changed. Report discovery errors.
	oidcAfter := s.Settings.OIDCIssuer() + "|" + s.Settings.OIDCClientID() + "|" + s.Settings.OIDCClientSecret() + "|" + s.Settings.OIDCScopes()
	if oidcAfter != oidcBefore {
		if err := s.SetupOIDC(ctx); err != nil {
			writeErr(w, http.StatusBadRequest, "OIDC settings saved but provider init failed: "+err.Error())
			return
		}
	}
	writeJSON(w, http.StatusOK, s.Settings.AdminView())
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
