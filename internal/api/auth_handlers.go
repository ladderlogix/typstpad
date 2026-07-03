package api

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/ladderlogix/typstpad/internal/auth"
	"github.com/ladderlogix/typstpad/internal/store"
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
		"oidcEnabled":       s.Settings.OIDCEnabled(),
		"allowRegistration": s.Settings.AllowRegistration(),
		"emailVerification": s.Settings.EmailVerificationRequired(),
		"signupAllowlist":   s.Settings.SignupAllowlist(),
	})
}

const emailVerificationTTL = 24 * time.Hour

// sendVerification generates a token and emails a verification link.
func (s *Server) sendVerification(r *http.Request, user *store.User) error {
	token, hash, err := auth.NewToken("")
	if err != nil {
		return err
	}
	if err := s.Store.CreateEmailVerification(r.Context(), hash, user.ID, time.Now().Add(emailVerificationTTL)); err != nil {
		return err
	}
	link := strings.TrimRight(s.Cfg.PublicURL, "/") + "/api/auth/verify-email?token=" + token
	return s.Mailer.SendVerification(user.Email, user.Name, link)
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
	if !s.Settings.EmailAllowed(req.Email) {
		writeErr(w, http.StatusForbidden, "this email address is not permitted to sign up here")
		return
	}
	// Registration can be disabled by an admin, but the very first user (the
	// bootstrap admin) can always register.
	n, err := s.Store.CountUsers(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	if n > 0 && !s.Settings.AllowRegistration() {
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
	if user.IsAdmin && s.OnFirstUser != nil {
		s.OnFirstUser()
	}

	// When email verification is on, don't log the user in — send a link and
	// tell the client to prompt for verification.
	if s.Settings.EmailVerificationRequired() {
		if err := s.sendVerification(r, user); err != nil {
			slog.Error("send verification email failed", "err", err)
			writeErr(w, http.StatusBadGateway, "could not send verification email; contact the administrator")
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{
			"emailVerificationRequired": true,
			"email":                     user.Email,
		})
		return
	}

	if err := s.Auth.SetSessionCookie(w, r, user.ID); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, user)
}

func (s *Server) handleVerifyEmail(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Redirect(w, r, "/login?verify=invalid", http.StatusFound)
		return
	}
	userID, err := s.Store.ConsumeEmailVerification(r.Context(), auth.HashToken(token))
	if err != nil {
		http.Redirect(w, r, "/login?verify=invalid", http.StatusFound)
		return
	}
	if err := s.Store.MarkEmailVerified(r.Context(), userID); err != nil {
		fail(w, err)
		return
	}
	http.Redirect(w, r, "/login?verify=success", http.StatusFound)
}

func (s *Server) handleResendVerification(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	// Always return ok so this doesn't reveal which emails are registered.
	if s.Settings.EmailVerificationRequired() {
		if user, err := s.Store.UserByEmail(r.Context(), strings.TrimSpace(strings.ToLower(req.Email))); err == nil && !user.EmailVerified {
			if err := s.sendVerification(r, user); err != nil {
				slog.Error("resend verification failed", "err", err)
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
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
	if s.Settings.EmailVerificationRequired() && !user.EmailVerified {
		writeJSON(w, http.StatusForbidden, map[string]any{
			"error":              "please verify your email before signing in",
			"needsVerification":  true,
			"email":              user.Email,
		})
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
	// Tell the client whether this is a local (password) account so the UI can
	// show the change-password form.
	writeJSON(w, http.StatusOK, meResponse{User: u, HasPassword: u.PasswordHash != nil})
}

type meResponse struct {
	*store.User
	HasPassword bool `json:"hasPassword"`
}

// handleUpdateProfile lets a user change their own display name and color.
func (s *Server) handleUpdateProfile(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFrom(r.Context())
	var req struct {
		Name  string `json:"name"`
		Color string `json:"color"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = u.Name
	}
	color := u.Color
	if strings.HasPrefix(req.Color, "#") && len(req.Color) == 7 {
		color = req.Color
	}
	if err := s.Store.UpdateUser(r.Context(), u.ID, name, color); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleChangePassword changes the caller's password after verifying the
// current one (local accounts only).
func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFrom(r.Context())
	var req struct {
		CurrentPassword string `json:"currentPassword"`
		NewPassword     string `json:"newPassword"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if u.PasswordHash == nil {
		writeErr(w, http.StatusBadRequest, "this account signs in with SSO and has no password")
		return
	}
	if !auth.VerifyPassword(req.CurrentPassword, *u.PasswordHash) {
		writeErr(w, http.StatusUnauthorized, "current password is incorrect")
		return
	}
	if len(req.NewPassword) < 8 {
		writeErr(w, http.StatusBadRequest, "new password must be at least 8 characters")
		return
	}
	hash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		fail(w, err)
		return
	}
	if err := s.Store.SetPassword(r.Context(), u.ID, hash); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

const passwordResetTTL = time.Hour

func (s *Server) handleForgotPassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	// Always return ok — never reveal whether an email is registered.
	if s.Settings.SMTPEnabled() {
		user, err := s.Store.UserByEmail(r.Context(), strings.TrimSpace(strings.ToLower(req.Email)))
		if err == nil && user.PasswordHash != nil {
			token, hash, terr := auth.NewToken("")
			if terr == nil {
				if err := s.Store.CreatePasswordReset(r.Context(), hash, user.ID, time.Now().Add(passwordResetTTL)); err == nil {
					link := strings.TrimRight(s.Cfg.PublicURL, "/") + "/reset-password?token=" + token
					if err := s.Mailer.SendPasswordReset(user.Email, user.Name, link); err != nil {
						slog.Error("send password reset failed", "err", err)
					}
				}
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleResetPassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if len(req.Password) < 8 {
		writeErr(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}
	userID, err := s.Store.ConsumePasswordReset(r.Context(), auth.HashToken(req.Token))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "this reset link is invalid or has expired")
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		fail(w, err)
		return
	}
	if err := s.Store.SetPassword(r.Context(), userID, hash); err != nil {
		fail(w, err)
		return
	}
	// Reset also verifies the email (they proved control of it) and logs out
	// other sessions.
	_ = s.Store.MarkEmailVerified(r.Context(), userID)
	_ = s.Store.DeleteUserSessions(r.Context(), userID)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
