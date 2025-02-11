package main

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
	"github.com/neverlless/json-to-metrics-exporter/pkg/collector"
	"net/http"
	"os"
)

func main() {
	logrus.SetFormatter(&logrus.JSONFormatter{})

	http.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		target := r.URL.Query().Get("target")
		if target == "" {
			logrus.Warning("Missing target parameter in request")
			http.Error(w, "Missing target parameter", http.StatusBadRequest)
			return
		}

		colec := collector.NewJsonCollector(target)

		registry := prometheus.NewRegistry()
		registry.MustRegister(colec)

		logrus.WithField("target", target).Info("Serving metrics")
		promhttp.HandlerFor(registry, promhttp.HandlerOpts{}).ServeHTTP(w, r)
	})

	ip := getEnv("HOST", "0.0.0.0")
	port := getEnv("PORT", "9908")

	logrus.Infof("Starting server at %s:%s", ip, port)
	logrus.Fatal(http.ListenAndServe(ip+":"+port, nil))
}

// getEnv fetches an environment variable or returns a default value.
func getEnv(key, defaultVal string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultVal
}