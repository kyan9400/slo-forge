package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"

	"github.com/kyan9400/slo-forge/internal/compiler"
	"github.com/kyan9400/slo-forge/internal/spec"
)

const maxRequestBytes = 1 << 20

type API struct {
	version        string
	requests       atomic.Uint64
	renders        atomic.Uint64
	renderFailures atomic.Uint64
}

func New(version string) *API {
	return &API{version: version}
}

func (a *API) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", requireMethod(http.MethodGet, a.handleIndex))
	mux.HandleFunc("/healthz", requireMethod(http.MethodGet, a.handleHealth))
	mux.HandleFunc("/readyz", requireMethod(http.MethodGet, a.handleHealth))
	mux.HandleFunc("/metrics", requireMethod(http.MethodGet, a.handleMetrics))
	mux.HandleFunc("/v1/render", requireMethod(http.MethodPost, a.handleRender))
	return a.withRequestMetrics(mux)
}

func requireMethod(method string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != method {
			w.Header().Set("Allow", method)
			writeProblem(w, http.StatusMethodNotAllowed, "method not allowed", "use "+method)
			return
		}
		next(w, r)
	}
}

func (a *API) withRequestMetrics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a.requests.Add(1)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		next.ServeHTTP(w, r)
	})
}

func (a *API) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"name":    "slo-forge",
		"version": a.version,
		"endpoints": map[string]string{
			"render":  "POST /v1/render",
			"health":  "GET /healthz",
			"metrics": "GET /metrics",
		},
	})
}

func (a *API) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *API) handleMetrics(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	fmt.Fprintf(w, "# HELP slo_forge_http_requests_total Total HTTP requests.\n")
	fmt.Fprintf(w, "# TYPE slo_forge_http_requests_total counter\n")
	fmt.Fprintf(w, "slo_forge_http_requests_total %d\n", a.requests.Load())
	fmt.Fprintf(w, "# HELP slo_forge_renders_total Successful SLO renders.\n")
	fmt.Fprintf(w, "# TYPE slo_forge_renders_total counter\n")
	fmt.Fprintf(w, "slo_forge_renders_total %d\n", a.renders.Load())
	fmt.Fprintf(w, "# HELP slo_forge_render_failures_total Failed SLO renders.\n")
	fmt.Fprintf(w, "# TYPE slo_forge_render_failures_total counter\n")
	fmt.Fprintf(w, "slo_forge_render_failures_total %d\n", a.renderFailures.Load())
}

func (a *API) handleRender(w http.ResponseWriter, r *http.Request) {
	if contentType := r.Header.Get("Content-Type"); contentType != "" && !strings.Contains(contentType, "yaml") && !strings.Contains(contentType, "json") && !strings.Contains(contentType, "text/plain") {
		a.renderFailures.Add(1)
		writeProblem(w, http.StatusUnsupportedMediaType, "unsupported media type", "send YAML using application/yaml")
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxRequestBytes))
	if err != nil {
		a.renderFailures.Add(1)
		writeProblem(w, http.StatusRequestEntityTooLarge, "request too large", "SLO documents are limited to 1 MiB")
		return
	}
	document, err := spec.Load(body)
	if err != nil {
		a.renderFailures.Add(1)
		status := http.StatusUnprocessableEntity
		var syntaxError *json.SyntaxError
		if errors.As(err, &syntaxError) {
			status = http.StatusBadRequest
		}
		writeProblem(w, status, "invalid SLO", err.Error())
		return
	}
	output, err := compiler.Compile(document)
	if err != nil {
		a.renderFailures.Add(1)
		writeProblem(w, http.StatusInternalServerError, "render failed", err.Error())
		return
	}
	a.renders.Add(1)
	writeJSON(w, http.StatusOK, map[string]string{
		"prometheusRules":  string(output.PrometheusRules),
		"prometheusRule":   string(output.PrometheusRule),
		"grafanaDashboard": string(output.GrafanaDashboard),
		"explanation":      string(output.Explanation),
	})
}

func writeProblem(w http.ResponseWriter, status int, title, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	writeJSON(w, status, map[string]any{"status": status, "title": title, "detail": detail})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	if w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
