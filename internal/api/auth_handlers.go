package api

import (
	"errors"
	"net/http"
	"strings"

	"typstpad/internal/auth"
	"typstpad/internal/store"
)

var userColors = []string{
	"#e11d48", "#ea580c", "#ca8a04", "#16a34a", "#0d9488",
	"#0284c7", "#4f46e5", "#9333ea", "#c026d3", "#db2777",
}

func pickColor(seed string) string {
	var sum int
	for _, c := range seed {
		sum += int(c)
	}
	return userColors[sum%len(userColors)]
}

func (s *Server) handleAuthConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"oidcEnabled":       s.Cfg.OIDCEnabled(),
		"allowRegistration": s.Store.SettingBool(r.Context(), "allow_registration", true),
	})
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Name     string `json:"name"`
		Password string `json:"password"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	req.Name = strings.TrimSpace(req.Name)
	if req.Email == "" || !strings.Contains(req.Email, "@") {
		writeErr(w, http.StatusBadRequest, "valid email required")
		return
	}
	if req.Name == "" {
		req.Name = strings.Split(req.Email, "@")[0]
	}
	if len(req.Password) < 8 {
		writeErr(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}
	// Registration can be disabled by an admin, but the very first user (the
	// bootstrap admin) can always register.
	n, err := s.Store.CountUsers(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	if n > 0 && !s.Store.SettingBool(r.Context(), "allow_registration", true) {
		writeErr(w, http.StatusForbidden, "registration is disabled")
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		fail(w, err)
		return
	}
	user, err := s.Store.CreateUser(r.Context(), req.Email, req.Name, &hash, pickColor(req.Email))
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			writeErr(w, http.StatusConflict, "an account with this email already exists")
			return
		}
		fail(w, err)
		return
	}
	if err := s.Auth.SetSessionCookie(w, r, user.ID); err != nil {
		fail(w, err)
		return
	}
	if user.IsAdmin && s.OnFirstUser != nil {
		s.OnFirstUser()
	}
	writeJSON(w, http.StatusCreated, user)
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	user, err := s.Store.UserByEmail(r.Context(), strings.TrimSpace(strings.ToLower(req.Email)))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusUnauthorized, "invalid email or password")
			return
		}
		fail(w, err)
		return
	}
	if user.PasswordHash == nil || !auth.VerifyPassword(req.Password, *user.PasswordHash) {
		writeErr(w, http.StatusUnauthorized, "invalid email or password")
		return
	}
	if err := s.Auth.SetSessionCookie(w, r, user.ID); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, user)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	s.Auth.ClearSession(w, r)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFrom(r.Context())
	if u == nil {
		writeErr(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	writeJSON(w, http.StatusOK, u)
}
