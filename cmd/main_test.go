package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/neverlless/json-to-metrics-exporter/pkg/collector"
)

func TestMetricsHandlerTargetChecks(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status":"OK"}`))
	}))
	defer target.Close()

	allowed, err := compileAllowedTargets(regexp.QuoteMeta(target.URL) + "/.*")
	if err != nil {
		t.Fatal(err)
	}
	handler := newHandler(regexp.MustCompile("OK"), allowed, collector.NewClient(false, allowed))

	tests := []struct {
		name     string
		query    string
		expected int
	}{
		{"missing target", "", http.StatusBadRequest},
		{"allowed target", "?target=" + url.QueryEscape(target.URL+"/health"), http.StatusOK},
		{"other host", "?target=" + url.QueryEscape("http://169.254.169.254/"), http.StatusForbidden},
		{"allowed URL only in query", "?target=" + url.QueryEscape("http://evil.test/?x="+target.URL+"/health"), http.StatusForbidden},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/metrics"+test.query, nil))
			if recorder.Code != test.expected {
				t.Errorf("Expected status %d, got %d: %s", test.expected, recorder.Code, recorder.Body)
			}
		})
	}
}

func TestRedirectToForbiddenTargetIsNotFollowed(t *testing.T) {
	var internalHit bool
	internal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		internalHit = true
	}))
	defer internal.Close()

	public := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, internal.URL, http.StatusFound)
	}))
	defer public.Close()

	allowed, err := compileAllowedTargets(regexp.QuoteMeta(public.URL))
	if err != nil {
		t.Fatal(err)
	}
	handler := newHandler(regexp.MustCompile("OK"), allowed, collector.NewClient(false, allowed))

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/metrics?target="+url.QueryEscape(public.URL), nil))

	if internalHit {
		t.Error("Redirect reached a target outside ALLOWED_TARGETS")
	}
	if !strings.Contains(recorder.Body.String(), "probe_success") {
		t.Errorf("Expected probe_success in response, got %s", recorder.Body)
	}
}

func TestUnknownPathIsNotFound(t *testing.T) {
	handler := newHandler(regexp.MustCompile("OK"), nil, http.DefaultClient)

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/nope", nil))

	if recorder.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", recorder.Code)
	}
}
