package httpapi

import (
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/yota/ohey/backend/internal/contracts"
)

type PushTokenRequest struct {
	Token    string `json:"token"`
	Platform string `json:"platform"`
}

func (r *router) registerPushToken(w http.ResponseWriter, req *http.Request, authToken string) {
	var input PushTokenRequest
	if !decodeJSONBody(w, req, &input) {
		return
	}
	token := strings.TrimSpace(input.Token)
	platform := strings.ToLower(strings.TrimSpace(input.Platform))
	if token == "" {
		writeError(w, http.StatusBadRequest, "token is required")
		return
	}
	if len(token) > 4096 {
		writeError(w, http.StatusBadRequest, "token is too long")
		return
	}
	if platform == "" {
		platform = contracts.PushPlatformIOS
	}
	if platform != contracts.PushPlatformIOS && platform != contracts.PushPlatformAndroid {
		writeError(w, http.StatusBadRequest, "platform must be ios or android")
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	q := make(map[string][]string)
	q["on_conflict"] = []string{"token"}
	payload := map[string]any{
		"token":        token,
		"user_id":      req.Header.Get("X-Ohey-User-ID"),
		"platform":     platform,
		"updated_at":   now,
		"last_seen_at": now,
	}
	var rows []map[string]any
	if err := r.deps.Supabase.Upsert(req.Context(), authToken, "push_tokens", q, payload, &rows); err != nil {
		writeSupabaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, firstMap(rows, payload))
}

func (r *router) unregisterPushToken(w http.ResponseWriter, req *http.Request, authToken string) {
	token := strings.TrimSpace(req.URL.Query().Get("token"))
	if token == "" && req.Body != nil {
		var input PushTokenRequest
		if !decodeJSONBody(w, req, &input) {
			return
		}
		token = strings.TrimSpace(input.Token)
	}
	if token == "" {
		writeError(w, http.StatusBadRequest, "token is required")
		return
	}
	if len(token) > 4096 {
		writeError(w, http.StatusBadRequest, "token is too long")
		return
	}

	q := url.Values{}
	q.Set("token", "eq."+token)
	q.Set("user_id", "eq."+req.Header.Get("X-Ohey-User-ID"))
	var rows []map[string]any
	if r.deps.AdminSupabase != nil && r.deps.Config.SupabaseServiceRoleKey != "" {
		if err := r.deps.AdminSupabase.Delete(req.Context(), r.deps.Config.SupabaseServiceRoleKey, "push_tokens", q, &rows); err != nil {
			writeSupabaseError(w, err)
			return
		}
	} else if err := r.deps.Supabase.Delete(req.Context(), authToken, "push_tokens", q, &rows); err != nil {
		writeSupabaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true, "deleted_count": len(rows)})
}
