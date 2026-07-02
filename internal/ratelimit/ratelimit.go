// Package ratelimit is a small in-memory fixed-window rate limiter for a
// single-instance deployment. Keys are arbitrary strings (client IP, user id).
package ratelimit

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

type window struct {
	count int
	reset time.Time
}

type Limiter struct {
	mu      sync.Mutex
	windows map[string]*window
	limit   int
	period  time.Duration
	now     func() time.Time
}

// New creates a limiter allowing `limit` requests per `period` per key.
func New(limit int, period time.Duration) *Limiter {
	l := &Limiter{windows: map[string]*window{}, limit: limit, period: period, now: time.Now}
	go l.gc()
	return l
}

// Allow reports whether a request under key may proceed.
func (l *Limiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	w := l.windows[key]
	if w == nil || now.After(w.reset) {
		l.windows[key] = &window{count: 1, reset: now.Add(l.period)}
		return true
	}
	if w.count >= l.limit {
		return false
	}
	w.count++
	return true
}

func (l *Limiter) gc() {
	t := time.NewTicker(5 * time.Minute)
	for range t.C {
		l.mu.Lock()
		now := l.now()
		for k, w := range l.windows {
			if now.After(w.reset) {
				delete(l.windows, k)
			}
		}
		l.mu.Unlock()
	}
}

// Middleware rate-limits by a key derived from the request. On limit it
// responds 429.
func (l *Limiter) Middleware(keyFn func(*http.Request) string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !l.Allow(keyFn(r)) {
				w.Header().Set("Retry-After", "60")
				http.Error(w, `{"error":"too many requests, please slow down"}`, http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// ClientIP extracts the real client IP, honoring Cloudflare's CF-Connecting-IP
// and X-Forwarded-For (the app sits behind a Cloudflare tunnel / reverse proxy).
func ClientIP(r *http.Request) string {
	if ip := r.Header.Get("CF-Connecting-IP"); ip != "" {
		return ip
	}
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.IndexByte(xff, ','); i >= 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
