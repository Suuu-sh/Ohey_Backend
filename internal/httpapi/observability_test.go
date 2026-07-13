package httpapi

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Suuu-sh/Ohey_Backend/internal/config"
)

func TestRequestObservabilityPreservesValidRequestID(t *testing.T) {
	handler := NewRouter(Dependencies{Config: config.Config{AllowedOrigins: []string{"*"}}})
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set(requestIDHeader, "edge-request_123")
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	if got := res.Header().Get(requestIDHeader); got != "edge-request_123" {
		t.Fatalf("request ID = %q, want preserved value", got)
	}
	if got := res.Header().Get("Strict-Transport-Security"); got == "" {
		t.Fatal("Strict-Transport-Security is missing")
	}
	if got := res.Header().Get("Content-Security-Policy"); got == "" {
		t.Fatal("Content-Security-Policy is missing")
	}
}

func TestRequestObservabilityReplacesUnsafeRequestID(t *testing.T) {
	handler := NewRouter(Dependencies{Config: config.Config{AllowedOrigins: []string{"*"}}})
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set(requestIDHeader, strings.Repeat("x", 200))
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	got := res.Header().Get(requestIDHeader)
	if !validRequestIDPattern.MatchString(got) || got == req.Header.Get(requestIDHeader) {
		t.Fatalf("request ID = %q, want generated safe value", got)
	}
}

func TestRequestObservabilityRecoversPanicWithoutLeakingDetails(t *testing.T) {
	var logs bytes.Buffer
	r := &router{deps: Dependencies{Logger: slog.New(slog.NewJSONHandler(&logs, nil))}}
	handler := r.withRequestObservability(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("sensitive panic detail")
	}))
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/v1/test", nil))

	if res.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusInternalServerError)
	}
	var body map[string]string
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["error"] != "internal server error" {
		t.Fatalf("error = %q, want generic message", body["error"])
	}
	if strings.Contains(res.Body.String(), "sensitive") {
		t.Fatalf("panic details leaked in response: %s", res.Body.String())
	}
}

func TestRequestObservabilityLogsRoutePatternInsteadOfUserIdentifier(t *testing.T) {
	var logs bytes.Buffer
	handler := NewRouter(Dependencies{
		Config: config.Config{AllowedOrigins: []string{"*"}},
		Logger: slog.New(slog.NewJSONHandler(&logs, nil)),
	})
	userID := "110e8400-e29b-41d4-a716-446655440000"
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/v1/profiles/by-user-id/"+userID, nil))

	if got := res.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
	if strings.Contains(logs.String(), userID) {
		t.Fatalf("request log leaked path identifier: %s", logs.String())
	}
	if !strings.Contains(logs.String(), "/v1/profiles/by-user-id/{user_id}") {
		t.Fatalf("request log is missing route pattern: %s", logs.String())
	}
}
