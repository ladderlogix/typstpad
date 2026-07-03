package api

import (
	"context"
	"sync"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/http"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/ladderlogix/typstpad/internal/store"
)

type oidcState struct {
	provider *oidc.Provider
	verifier *oidc.IDTokenVerifier
	oauth    *oauth2.Config
}

var (
	oidcHandlers *oidcState
	oidcMu       sync.RWMutex
)

func currentOIDC() *oidcState {
	oidcMu.RLock()
	defer oidcMu.RUnlock()
	return oidcHandlers
}

// SetupOIDC (re)initializes the OIDC provider from current settings. Safe to
// call at startup and again whenever an admin updates OIDC settings. Clears
// the provider when OIDC is not configured.
func (s *Server) SetupOIDC(ctx context.Context) error {
	if !s.Settings.OIDCEnabled() {
		oidcMu.Lock()
		oidcHandlers = nil
		oidcMu.Unlock()
		return nil
	}
	provider, err := oidc.NewProvider(ctx, s.Settings.OIDCIssuer())
	if err != nil {
		return err
	}
	clientID := s.Settings.OIDCClientID()
	scopes := []string{oidc.ScopeOpenID}
	for _, sc := range splitScopes(s.Settings.OIDCScopes()) {
		if sc != oidc.ScopeOpenID {
			scopes = append(scopes, sc)
		}
	}
	oidcMu.Lock()
	oidcHandlers = &oidcState{
		provider: provider,
		verifier: provider.Verifier(&oidc.Config{ClientID: clientID}),
		oauth: &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: s.Settings.OIDCClientSecret(),
			Endpoint:     provider.Endpoint(),
			RedirectURL:  s.Cfg.PublicURL + "/api/auth/oidc/callback",
			Scopes:       scopes,
		},
	}
	oidcMu.Unlock()
	return nil
}

func splitScopes(s string) []string {
	var out []string
	cur := ""
	for _, c := range s {
		if c == ' ' || c == ',' {
			if cur != "" {
				out = append(out, cur)
				cur = ""
			}
			continue
		}
		cur += string(c)
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

func randB64(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func (s *Server) oidcCookie(w http.ResponseWriter, name, value string) {
	http.SetCookie(w, &http.Cookie{
		Name: name, Value: value, Path: "/api/auth/oidc",
		MaxAge: 600, HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: !s.Cfg.DevHTTP,
	})
}

func (s *Server) handleOIDCLogin(w http.ResponseWriter, r *http.Request) {
	oh := currentOIDC()
	if oh == nil {
		writeErr(w, http.StatusNotFound, "SSO not configured")
		return
	}
	state := randB64(24)
	verifier := oauth2.GenerateVerifier()
	s.oidcCookie(w, "oidc_state", state)
	s.oidcCookie(w, "oidc_verifier", verifier)
	http.Redirect(w, r, oh.oauth.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier)), http.StatusFound)
}

func (s *Server) handleOIDCCallback(w http.ResponseWriter, r *http.Request) {
	oh := currentOIDC()
	if oh == nil {
		writeErr(w, http.StatusNotFound, "SSO not configured")
		return
	}
	stateCookie, err1 := r.Cookie("oidc_state")
	verifierCookie, err2 := r.Cookie("oidc_verifier")
	if err1 != nil || err2 != nil || r.URL.Query().Get("state") != stateCookie.Value {
		writeErr(w, http.StatusBadRequest, "invalid OIDC state")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	token, err := oh.oauth.Exchange(ctx, r.URL.Query().Get("code"),
		oauth2.VerifierOption(verifierCookie.Value))
	if err != nil {
		writeErr(w, http.StatusBadGateway, "token exchange failed: "+err.Error())
		return
	}
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		writeErr(w, http.StatusBadGateway, "no id_token in response")
		return
	}
	idToken, err := oh.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "invalid id_token: "+err.Error())
		return
	}
	var claims struct {
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
		Name          string `json:"name"`
	}
	if err := idToken.Claims(&claims); err != nil {
		fail(w, err)
		return
	}

	user, err := s.Store.UserByOIDC(ctx, idToken.Issuer, idToken.Subject)
	if errors.Is(err, store.ErrNotFound) {
		user = nil
		// Link by verified email to an existing local account.
		if claims.Email != "" && claims.EmailVerified {
			if existing, eerr := s.Store.UserByEmail(ctx, claims.Email); eerr == nil {
				user = existing
			}
		}
		if user == nil {
			if claims.Email == "" {
				writeErr(w, http.StatusBadRequest, "identity provider returned no email")
				return
			}
			name := claims.Name
			if name == "" {
				name = claims.Email
			}
			user, err = s.Store.CreateUser(ctx, claims.Email, name, nil, pickColor(claims.Email))
			if err != nil {
				fail(w, err)
				return
			}
			// The identity provider already verified this email.
			_ = s.Store.MarkEmailVerified(ctx, user.ID)
			if user.IsAdmin && s.OnFirstUser != nil {
				s.OnFirstUser()
			}
		}
		if err := s.Store.LinkOIDC(ctx, user.ID, idToken.Issuer, idToken.Subject); err != nil {
			fail(w, err)
			return
		}
	} else if err != nil {
		fail(w, err)
		return
	}

	if err := s.Auth.SetSessionCookie(w, r, user.ID); err != nil {
		fail(w, err)
		return
	}
	http.Redirect(w, r, "/", http.StatusFound)
}
