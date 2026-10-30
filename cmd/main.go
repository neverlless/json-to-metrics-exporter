package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"syscall"
	"time"

	"github.com/neverlless/json-to-metrics-exporter/pkg/collector"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))

	if err := run(); err != nil {
		slog.Error("Exporter stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	healthy, err := regexp.Compile(getEnv("HEALTHY_REGEX", "OK|success"))
	if err != nil {
		return err
	}
	allowed, err := compileAllowedTargets(os.Getenv("ALLOWED_TARGETS"))
	if err != nil {
		return err
	}
	if allowed == nil {
		slog.Warn("ALLOWED_TARGETS is not set: anyone who can reach this port can make the exporter request any URL")
	}
	insecureTLS, err := strconv.ParseBool(getEnv("TLS_INSECURE", "false"))
	if err != nil {
		return err
	}

	server := &http.Server{
		Addr:              net.JoinHostPort(getEnv("HOST", "0.0.0.0"), getEnv("PORT", "9908")),
		Handler:           newHandler(healthy, allowed, collector.NewClient(insecureTLS, allowed)),
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	serveErr := make(chan error, 1)
	go func() {
		slog.Info("Starting server", "addr", server.Addr)
		serveErr <- server.ListenAndServe()
	}()

	select {
	case err := <-serveErr:
		return err
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		return err
	}
	if err := <-serveErr; !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func newHandler(healthy, allowed *regexp.Regexp, client *http.Client) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte("<html><body><h1>Welcome to JSON to Metrics Exporter</h1><p>Use '/metrics?target=YOUR_JSON_ENDPOINT' to collect metrics.</p></body></html>"))
	})

	mux.HandleFunc("GET /metrics", func(w http.ResponseWriter, r *http.Request) {
		target := r.URL.Query().Get("target")
		if target == "" {
			slog.Warn("Missing target parameter in request")
			http.Error(w, "Missing target parameter", http.StatusBadRequest)
			return
		}
		if allowed != nil && !allowed.MatchString(target) {
			slog.Warn("Target is not allowed", "target", target)
			http.Error(w, "Target is not allowed", http.StatusForbidden)
			return
		}

		registry := prometheus.NewRegistry()
		registry.MustRegister(collector.NewJsonCollector(r.Context(), target, healthy, client))

		slog.Info("Serving metrics", "target", target)
		promhttp.HandlerFor(registry, promhttp.HandlerOpts{}).ServeHTTP(w, r)
	})

	return mux
}

// compileAllowedTargets returns nil when pattern is empty. The pattern must
// match the whole URL, so that a query string cannot satisfy it.
func compileAllowedTargets(pattern string) (*regexp.Regexp, error) {
	if pattern == "" {
		return nil, nil
	}
	return regexp.Compile("^(?:" + pattern + ")$")
}

// getEnv fetches an environment variable or returns a default value.
func getEnv(key, defaultVal string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultVal
}
