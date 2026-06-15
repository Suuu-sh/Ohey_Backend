package httpapi

import (
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Suuu-sh/Ohey_Backend/internal/features/webhooks"
)

func (r *router) handleClerkEmailWebhook(w http.ResponseWriter, req *http.Request) {
	body, err := io.ReadAll(io.LimitReader(req.Body, 1024*1024))
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read webhook body")
		return
	}
	result, err := webhooks.ProcessClerkEmailWebhook(req.Header, body, &r.deps.Config, time.Now())
	if err != nil {
		msg := strings.ToLower(err.Error())
		if strings.Contains(msg, "signature") || strings.Contains(msg, "timestamp") || strings.Contains(msg, "svix") || strings.Contains(msg, "secret") {
			writeError(w, http.StatusUnauthorized, "Authentication required")
			return
		}
		if strings.Contains(msg, "invalid webhook json") || strings.Contains(msg, "invalid email event data") {
			writeError(w, http.StatusBadRequest, "invalid webhook payload")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to deliver email")
		return
	}
	status := http.StatusAccepted
	if result.Ignored {
		status = http.StatusOK
	}
	writeJSON(w, status, result)
}
