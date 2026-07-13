package httpapi

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"regexp"
	"runtime/debug"
	"strings"
	"time"
)

const requestIDHeader = "X-Request-ID"

var validRequestIDPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (w *statusRecorder) WriteHeader(status int) {
	if w.wroteHeader {
		return
	}
	w.status = status
	w.wroteHeader = true
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusRecorder) Write(body []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(body)
}

func (w *statusRecorder) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func (r *router) withRequestObservability(next http.Handler) http.Handler {
	logger := r.deps.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		startedAt := timeNow()
		requestID := normalizedRequestID(req.Header.Get(requestIDHeader))
		w.Header().Set(requestIDHeader, requestID)
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

		defer func() {
			if recovered := recover(); recovered != nil {
				logger.Error(
					"http panic recovered",
					"request_id", requestID,
					"method", req.Method,
					"path", requestLogPath(req),
					"stack", string(debug.Stack()),
				)
				if !recorder.wroteHeader {
					writeError(recorder, http.StatusInternalServerError, "internal server error")
				}
			}

			if !isRoutineHealthRequest(req.URL.Path, recorder.status) {
				logger.Info(
					"http request",
					"request_id", requestID,
					"method", req.Method,
					"path", requestLogPath(req),
					"status", recorder.status,
					"duration_ms", timeNow().Sub(startedAt).Milliseconds(),
				)
			}
		}()

		next.ServeHTTP(recorder, req)
	})
}

func requestLogPath(req *http.Request) string {
	pattern := strings.TrimSpace(req.Pattern)
	if pattern == "" {
		return "<unmatched>"
	}
	return strings.TrimSpace(strings.TrimPrefix(pattern, req.Method+" "))
}

func normalizedRequestID(value string) string {
	value = strings.TrimSpace(value)
	if validRequestIDPattern.MatchString(value) {
		return value
	}
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err == nil {
		return hex.EncodeToString(bytes[:])
	}
	return hex.EncodeToString([]byte(timeNow().UTC().Format(time.RFC3339Nano)))[:32]
}

func isRoutineHealthRequest(path string, status int) bool {
	return (path == "/health" || path == "/healthz") && status < http.StatusBadRequest
}
