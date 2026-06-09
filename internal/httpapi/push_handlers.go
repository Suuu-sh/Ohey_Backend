package httpapi

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

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
	if r.deps.Config.DataStore == "postgres" || r.deps.Config.DataStore == "neon" {
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
		return
	}
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
	if r.deps.Config.DataStore == "postgres" || r.deps.Config.DataStore == "neon" {
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
		return
	}
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
