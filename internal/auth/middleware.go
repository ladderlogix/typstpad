package auth

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"time"

	"typstpad/internal/store"
)

type ctxKey int

const (
	userKey ctxKey = iota
	scopesKey
)

const SessionCookie = "typstpad_session"
const sessionTTL = 30 * 24 * time.Hour

// AllScopes marks a session-authenticated (non-PAT) request.
var AllScopes = []string{"read", "write", "compile", "admin"}

type Auth struct {
	Store   *store.Store
	DevHTTP bool
}

func (a *Auth) SetSessionCookie(w http.ResponseWriter, r *http.Request, userID string) error {
	token, hash, err := NewToken("")
	if err != nil {
		return err
	}
	expires := time.Now().Add(sessionTTL)
	if err := a.Store.CreateSession(r.Context(), hash, userID, expires, r.UserAgent()); err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookie,
		Value:    token,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   !a.DevHTTP,
	})
	return nil
}

func (a *Auth) ClearSession(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(SessionCookie); err == nil {
		_ = a.Store.DeleteSession(r.Context(), HashToken(c.Value))
	}
	http.SetCookie(w, &http.Cookie{
		Name: SessionCookie, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: !a.DevHTTP,
	})
}

// Authenticate resolves the request's user from a PAT bearer token or session
// cookie and stores it on the context. Anonymous requests pass through.
func (a *Auth) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The Prometheus /metrics endpoint carries its own bearer token that is
		// not a PAT; let its handler validate it instead of failing here.
		if r.URL.Path == "/metrics" {
			next.ServeHTTP(w, r)
			return
		}
		if bearer := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "); bearer != r.Header.Get("Authorization") && bearer != "" {
			user, scopes, err := a.Store.APITokenUser(r.Context(), HashToken(bearer))
			if err == nil {
				next.ServeHTTP(w, r.WithContext(withUser(r.Context(), user, scopes)))
				return
			}
			http.Error(w, `{"error":"invalid token"}`, http.StatusUnauthorized)
			return
		}
		if c, err := r.Cookie(SessionCookie); err == nil && c.Value != "" {
			user, err := a.Store.SessionUser(r.Context(), HashToken(c.Value), time.Now().Add(sessionTTL))
			if err == nil {
				next.ServeHTTP(w, r.WithContext(withUser(r.Context(), user, AllScopes)))
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func withUser(ctx context.Context, u *store.User, scopes []string) context.Context {
	ctx = context.WithValue(ctx, userKey, u)
	return context.WithValue(ctx, scopesKey, scopes)
}

func UserFrom(ctx context.Context) *store.User {
	u, _ := ctx.Value(userKey).(*store.User)
	return u
}

func ScopesFrom(ctx context.Context) []string {
	s, _ := ctx.Value(scopesKey).([]string)
	return s
}

func HasScope(ctx context.Context, scope string) bool {
	return slices.Contains(ScopesFrom(ctx), scope)
}

func RequireUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if UserFrom(r.Context()) == nil {
			http.Error(w, `{"error":"authentication required"}`, http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := UserFrom(r.Context())
		if u == nil || !u.IsAdmin {
			http.Error(w, `{"error":"admin required"}`, http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func RequireScope(scope string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !HasScope(r.Context(), scope) {
				http.Error(w, `{"error":"insufficient token scope"}`, http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
