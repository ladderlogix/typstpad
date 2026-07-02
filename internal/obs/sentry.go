// Package obs wires optional error tracking (Sentry). Everything here is a
// no-op unless SENTRY_DSN is configured, so it's safe to leave enabled.
package obs

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/getsentry/sentry-go"
	sentryhttp "github.com/getsentry/sentry-go/http"
)

// InitSentry initializes error tracking if dsn is non-empty. Returns a flush
// function to defer at shutdown (a no-op when disabled).
func InitSentry(dsn, environment, release string) func() {
	if dsn == "" {
		return func() {}
	}
	err := sentry.Init(sentry.ClientOptions{
		Dsn:         dsn,
		Environment: environment,
		Release:     release,
		// Capture a small trace sample; errors are always captured.
		TracesSampleRate: 0.05,
	})
	if err != nil {
		slog.Error("sentry init failed (error tracking disabled)", "err", err)
		return func() {}
	}
	slog.Info("error tracking enabled", "environment", environment)
	return func() { sentry.Flush(2 * time.Second) }
}

// Middleware wraps a handler to report panics (and recover) to Sentry. When
// Sentry is disabled it returns the handler unchanged.
func Middleware(dsn string, h http.Handler) http.Handler {
	if dsn == "" {
		return h
	}
	return sentryhttp.New(sentryhttp.Options{Repanic: true}).Handle(h)
}
