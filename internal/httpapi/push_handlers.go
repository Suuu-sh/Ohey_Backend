package httpapi

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Suuu-sh/Ohey_Backend/internal/contracts"
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
	payload := map[string]any{
		"token":        token,
		"user_id":      req.Header.Get("X-Ohey-User-ID"),
		"platform":     platform,
		"updated_at":   now,
		"last_seen_at": now,
	}
	if r.deps.Postgres == nil {
		writeError(w, http.StatusServiceUnavailable, "database is not configured")
		return
	}
	row, err := upsertPostgresPushToken(req.Context(), r.deps.Postgres.Pool(), payload)
	if err != nil {
		writeError(w, http.StatusBadGateway, "database error")
		return
	}
	writeJSON(w, http.StatusOK, row)
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

	if r.deps.Postgres == nil {
		writeError(w, http.StatusServiceUnavailable, "database is not configured")
		return
	}
	count, err := deletePostgresPushToken(req.Context(), r.deps.Postgres.Pool(), token, req.Header.Get("X-Ohey-User-ID"))
	if err != nil {
		writeError(w, http.StatusBadGateway, "database error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true, "deleted_count": count})
}

func upsertPostgresPushToken(ctx context.Context, pool *pgxpool.Pool, payload map[string]any) (map[string]any, error) {
	row := pool.QueryRow(ctx, `insert into push_tokens (token,user_id,platform,updated_at,last_seen_at) values ($1,$2,$3,now(),now()) on conflict (token) do update set user_id=excluded.user_id, platform=excluded.platform, updated_at=now(), last_seen_at=now() returning token,user_id::text,platform,created_at,updated_at,last_seen_at`, payload["token"], payload["user_id"], payload["platform"])
	var token, userID, platform string
	var created, updated, lastSeen time.Time
	if err := row.Scan(&token, &userID, &platform, &created, &updated, &lastSeen); err != nil {
		return nil, err
	}
	return map[string]any{"token": token, "user_id": userID, "platform": platform, "created_at": created.UTC().Format(time.RFC3339Nano), "updated_at": updated.UTC().Format(time.RFC3339Nano), "last_seen_at": lastSeen.UTC().Format(time.RFC3339Nano)}, nil
}

func deletePostgresPushToken(ctx context.Context, pool *pgxpool.Pool, token, userID string) (int64, error) {
	tag, err := pool.Exec(ctx, `delete from push_tokens where token=$1 and user_id=$2`, token, userID)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
