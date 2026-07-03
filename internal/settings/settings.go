// Package settings is a runtime configuration layer: admin-editable values
// stored in the DB `settings` table that override the env-based config.Config.
// Env values act as the bootstrap/fallback when a DB override isn't set.
package settings

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"sync"

	"github.com/ladderlogix/typstpad/internal/config"
	"github.com/ladderlogix/typstpad/internal/store"
)

// Managed setting keys.
const (
	KeyAllowRegistration        = "allow_registration"
	KeySignupAllowlist          = "signup_allowlist"
	KeyRequireEmailVerification = "require_email_verification"
	KeySMTPHost                 = "smtp_host"
	KeySMTPPort                 = "smtp_port"
	KeySMTPUsername             = "smtp_username"
	KeySMTPPassword             = "smtp_password"
	KeySMTPFrom                 = "smtp_from"
	KeySMTPFromName             = "smtp_from_name"
	KeyOIDCIssuer               = "oidc_issuer"
	KeyOIDCClientID             = "oidc_client_id"
	KeyOIDCClientSecret         = "oidc_client_secret"
	KeyOIDCScopes               = "oidc_scopes"
)

type Service struct {
	store *store.Store
	cfg   *config.Config
	mu    sync.RWMutex
	cache map[string]string
	// OnChange is invoked after any Set, so callers can react (e.g. re-init OIDC).
	OnChange func()
}

func New(ctx context.Context, st *store.Store, cfg *config.Config) (*Service, error) {
	s := &Service{store: st, cfg: cfg, cache: map[string]string{}}
	return s, s.Reload(ctx)
}

func (s *Service) Reload(ctx context.Context) error {
	m, err := s.store.AllSettings(ctx)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.cache = m
	s.mu.Unlock()
	return nil
}

// raw returns the DB override for key, or "" with ok=false when unset.
func (s *Service) raw(key string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.cache[key]
	return v, ok
}

// str returns the DB override or the env default.
func (s *Service) str(key, envDefault string) string {
	if v, ok := s.raw(key); ok {
		return v
	}
	return envDefault
}

func (s *Service) boolVal(key string, envDefault bool) bool {
	if v, ok := s.raw(key); ok {
		b, err := strconv.ParseBool(v)
		if err == nil {
			return b
		}
	}
	return envDefault
}

// Set upserts one setting (value stored as a jsonb string) and refreshes cache.
func (s *Service) Set(ctx context.Context, key, value string) error {
	j, _ := json.Marshal(value)
	if err := s.store.SetSetting(ctx, key, string(j)); err != nil {
		return err
	}
	s.mu.Lock()
	s.cache[key] = value
	s.mu.Unlock()
	if s.OnChange != nil {
		s.OnChange()
	}
	return nil
}

func (s *Service) Delete(ctx context.Context, key string) error {
	if err := s.store.DeleteSetting(ctx, key); err != nil {
		return err
	}
	s.mu.Lock()
	delete(s.cache, key)
	s.mu.Unlock()
	if s.OnChange != nil {
		s.OnChange()
	}
	return nil
}

// ---- typed accessors (DB override → env fallback) ----

func (s *Service) AllowRegistration() bool { return s.boolVal(KeyAllowRegistration, true) }

func (s *Service) SignupAllowlist() string { return s.str(KeySignupAllowlist, s.cfg.SignupAllowlist) }

func (s *Service) EmailAllowed(email string) bool {
	return config.EmailMatchesAllowlist(s.SignupAllowlist(), email)
}

func (s *Service) SMTPHost() string     { return s.str(KeySMTPHost, s.cfg.SMTPHost) }
func (s *Service) SMTPUsername() string { return s.str(KeySMTPUsername, s.cfg.SMTPUsername) }
func (s *Service) SMTPPassword() string { return s.str(KeySMTPPassword, s.cfg.SMTPPassword) }
func (s *Service) SMTPFrom() string     { return s.str(KeySMTPFrom, s.cfg.SMTPFrom) }
func (s *Service) SMTPFromName() string { return s.str(KeySMTPFromName, s.cfg.SMTPFromName) }

func (s *Service) SMTPPort() int {
	if v, ok := s.raw(KeySMTPPort); ok {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n > 0 {
			return n
		}
	}
	return s.cfg.SMTPPort
}

func (s *Service) SMTPEnabled() bool { return s.SMTPHost() != "" && s.SMTPFrom() != "" }

func (s *Service) EmailVerificationRequired() bool {
	if v, ok := s.raw(KeyRequireEmailVerification); ok {
		if b, err := strconv.ParseBool(v); err == nil {
			return b && s.SMTPEnabled()
		}
	}
	// no DB override: fall back to env behavior (default = on when SMTP set)
	if s.cfg.EmailVerificationRequired() && !dbOverridesSMTP(s) {
		return true
	}
	return s.SMTPEnabled()
}

// dbOverridesSMTP reports whether SMTP came from the DB (so env's
// EmailVerificationRequired shouldn't be the sole authority).
func dbOverridesSMTP(s *Service) bool {
	_, ok := s.raw(KeySMTPHost)
	return ok
}

func (s *Service) OIDCIssuer() string       { return s.str(KeyOIDCIssuer, s.cfg.OIDCIssuer) }
func (s *Service) OIDCClientID() string     { return s.str(KeyOIDCClientID, s.cfg.OIDCClientID) }
func (s *Service) OIDCClientSecret() string { return s.str(KeyOIDCClientSecret, s.cfg.OIDCClientSecret) }
func (s *Service) OIDCScopes() string       { return s.str(KeyOIDCScopes, s.cfg.OIDCScopes) }
func (s *Service) OIDCEnabled() bool {
	return s.OIDCIssuer() != "" && s.OIDCClientID() != "" && s.OIDCClientSecret() != ""
}

// AdminView is the settings snapshot returned to the admin UI. Secrets are
// masked to a boolean "isSet" so they're never sent to the browser.
type AdminView struct {
	AllowRegistration         bool   `json:"allowRegistration"`
	SignupAllowlist           string `json:"signupAllowlist"`
	RequireEmailVerification  bool   `json:"requireEmailVerification"`
	SMTPHost                  string `json:"smtpHost"`
	SMTPPort                  int    `json:"smtpPort"`
	SMTPUsername              string `json:"smtpUsername"`
	SMTPPasswordSet           bool   `json:"smtpPasswordSet"`
	SMTPFrom                  string `json:"smtpFrom"`
	SMTPFromName              string `json:"smtpFromName"`
	OIDCIssuer                string `json:"oidcIssuer"`
	OIDCClientID              string `json:"oidcClientId"`
	OIDCClientSecretSet       bool   `json:"oidcClientSecretSet"`
	OIDCScopes                string `json:"oidcScopes"`
	EmailVerificationActive   bool   `json:"emailVerificationActive"`
	OIDCActive                bool   `json:"oidcActive"`
}

func (s *Service) AdminView() AdminView {
	return AdminView{
		AllowRegistration:        s.AllowRegistration(),
		SignupAllowlist:          s.SignupAllowlist(),
		RequireEmailVerification: s.EmailVerificationRequired(),
		SMTPHost:                 s.SMTPHost(),
		SMTPPort:                 s.SMTPPort(),
		SMTPUsername:             s.SMTPUsername(),
		SMTPPasswordSet:          s.SMTPPassword() != "",
		SMTPFrom:                 s.SMTPFrom(),
		SMTPFromName:             s.SMTPFromName(),
		OIDCIssuer:               s.OIDCIssuer(),
		OIDCClientID:             s.OIDCClientID(),
		OIDCClientSecretSet:      s.OIDCClientSecret() != "",
		OIDCScopes:               s.OIDCScopes(),
		EmailVerificationActive:  s.EmailVerificationRequired(),
		OIDCActive:               s.OIDCEnabled(),
	}
}
