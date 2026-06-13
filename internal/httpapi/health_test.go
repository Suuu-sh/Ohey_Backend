package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHealthEndpointsAreLivenessOnly(t *testing.T) {
	handler := NewRouter(Dependencies{})
	for _, path := range []string{"/health", "/healthz"} {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), `"status":"ok"`) {
				t.Fatalf("body = %s, want ok status", rec.Body.String())
			}
		})
	}
}

func TestReadyChecksDatabaseConfiguration(t *testing.T) {
	handler := NewRouter(Dependencies{})
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
	if !strings.Contains(rec.Body.String(), "not configured") {
		t.Fatalf("body = %s, want not configured", rec.Body.String())
	}
}
