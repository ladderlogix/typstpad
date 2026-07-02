// Package metrics exposes Prometheus instrumentation: HTTP traffic, compile
// activity, and business gauges, plus the standard Go runtime/process metrics.
package metrics

import (
	"crypto/subtle"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	httpRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "typstpad_http_requests_total",
		Help: "HTTP requests by route, method and status.",
	}, []string{"method", "route", "status"})

	httpDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "typstpad_http_request_duration_seconds",
		Help:    "HTTP request latency by route.",
		Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
	}, []string{"method", "route"})

	compileTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "typstpad_compile_total",
		Help: "Typst compilations by result (ok|error).",
	}, []string{"result"})

	compileDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "typstpad_compile_duration_seconds",
		Help:    "Typst compilation duration.",
		Buckets: []float64{.05, .1, .25, .5, 1, 2, 5, 10, 20, 30},
	})

	gauges = map[string]prometheus.Gauge{}
)

func gauge(name, help string) prometheus.Gauge {
	g := promauto.NewGauge(prometheus.GaugeOpts{Name: name, Help: help})
	gauges[name] = g
	return g
}

var (
	usersGauge    = gauge("typstpad_users", "Total registered users.")
	projectsGauge = gauge("typstpad_projects", "Live (non-template) projects.")
	docsGauge     = gauge("typstpad_documents", "Text documents across projects.")
	teamsGauge    = gauge("typstpad_teams", "Teams.")
	sessionsGauge = gauge("typstpad_active_sessions", "Active (unexpired) sessions.")
)

// SetBusiness updates the business gauges (called periodically).
func SetBusiness(users, projects, documents, teams, sessions int) {
	usersGauge.Set(float64(users))
	projectsGauge.Set(float64(projects))
	docsGauge.Set(float64(documents))
	teamsGauge.Set(float64(teams))
	sessionsGauge.Set(float64(sessions))
}

// RecordCompile records a compilation result and duration.
func RecordCompile(ok bool, d time.Duration) {
	result := "error"
	if ok {
		result = "ok"
	}
	compileTotal.WithLabelValues(result).Inc()
	compileDuration.Observe(d.Seconds())
}

// Middleware records request counts and latency, labelled by the chi route
// pattern (bounded cardinality) rather than the raw path.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := chimw.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		route := chi.RouteContext(r.Context()).RoutePattern()
		if route == "" {
			route = "unmatched"
		}
		httpRequests.WithLabelValues(r.Method, route, strconv.Itoa(ww.Status())).Inc()
		httpDuration.WithLabelValues(r.Method, route).Observe(time.Since(start).Seconds())
	})
}

// Handler serves /metrics, gated by a bearer token (Prometheus scrapes with it)
// so the endpoint isn't public. Returns 404 when no token is configured.
func Handler(token string) http.Handler {
	prom := promhttp.Handler()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if token == "" {
			http.NotFound(w, r)
			return
		}
		got := r.Header.Get("Authorization")
		const p = "Bearer "
		if len(got) <= len(p) || subtle.ConstantTimeCompare([]byte(got[len(p):]), []byte(token)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		prom.ServeHTTP(w, r)
	})
}
