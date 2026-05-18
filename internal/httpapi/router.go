package httpapi

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/yota/nomo/backend/internal/config"
	"github.com/yota/nomo/backend/internal/supabase"
)

type Dependencies struct {
	Config   config.Config
	Logger   *slog.Logger
	Supabase *supabase.Client
}

type router struct {
	deps Dependencies
	mux  *http.ServeMux
}

func NewRouter(deps Dependencies) http.Handler {
	r := &router{deps: deps, mux: http.NewServeMux()}
	r.routes()
	return r.withCORS(r.mux)
}

func (r *router) routes() {
	r.mux.HandleFunc("GET /healthz", r.health)
	r.mux.HandleFunc("GET /v1/me/profile", r.auth(r.getProfile))
	r.mux.HandleFunc("PATCH /v1/me/profile", r.auth(r.updateProfile))
	r.mux.HandleFunc("GET /v1/friends", r.auth(r.listFriends))
	r.mux.HandleFunc("PATCH /v1/friends/{friendId}/favorite", r.auth(r.setFriendFavorite))
	r.mux.HandleFunc("GET /v1/drink-logs", r.auth(r.listDrinkLogs))
	r.mux.HandleFunc("DELETE /v1/drink-logs/{id}", r.auth(r.deleteDrinkLog))
	r.mux.HandleFunc("POST /v1/drink-logs", r.auth(r.createDrinkLog))
	r.mux.HandleFunc("PUT /v1/drink-logs/{id}/like", r.auth(r.likeDrinkLog))
	r.mux.HandleFunc("DELETE /v1/drink-logs/{id}/like", r.auth(r.unlikeDrinkLog))
	r.mux.HandleFunc("GET /v1/daily-status", r.auth(r.getDailyStatus))
	r.mux.HandleFunc("PUT /v1/daily-status", r.auth(r.upsertDailyStatus))
}

func (r *router) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "nomo-backend"})
}

func (r *router) getProfile(w http.ResponseWriter, req *http.Request, authToken string) {
	var rows []Profile
	q := url.Values{}
	q.Set("select", "id,user_id,display_name,character_key,avatar_url")
	q.Set("id", "eq."+req.Header.Get("X-Nomo-User-ID"))
	if err := r.deps.Supabase.Get(req.Context(), authToken, "profiles", q, &rows); err != nil {
		writeSupabaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (r *router) updateProfile(w http.ResponseWriter, req *http.Request, authToken string) {
	var body map[string]any
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	allowed := map[string]any{}
	for _, key := range []string{"display_name", "character_key", "avatar_url"} {
		if value, ok := body[key]; ok {
			allowed[key] = value
		}
	}
	q := url.Values{}
	q.Set("id", "eq."+req.Header.Get("X-Nomo-User-ID"))
	var rows []Profile
	if err := r.deps.Supabase.Patch(req.Context(), authToken, "profiles", q, allowed, &rows); err != nil {
		writeSupabaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (r *router) listFriends(w http.ResponseWriter, req *http.Request, authToken string) {
	q := url.Values{}
	q.Set("select", "user_a_id,user_b_id,is_favorite,user_a:profiles!friendships_user_a_id_fkey(id,user_id,display_name,character_key,avatar_url),user_b:profiles!friendships_user_b_id_fkey(id,user_id,display_name,character_key,avatar_url)")
	q.Set("or", "(user_a_id.eq."+req.Header.Get("X-Nomo-User-ID")+",user_b_id.eq."+req.Header.Get("X-Nomo-User-ID")+")")
	q.Set("order", "created_at.desc")
	var rows []map[string]any
	if err := r.deps.Supabase.Get(req.Context(), authToken, "friendships", q, &rows); err != nil {
		writeSupabaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (r *router) setFriendFavorite(
	w http.ResponseWriter,
	req *http.Request,
	authToken string,
) {
	friendID := strings.TrimSpace(req.PathValue("friendId"))
	if friendID == "" {
		writeError(w, http.StatusBadRequest, "friend id is required")
		return
	}

	var body map[string]any
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	rawFavorite, ok := body["is_favorite"]
	if !ok {
		writeError(w, http.StatusBadRequest, "is_favorite is required")
		return
	}
	isFavorite, ok := rawFavorite.(bool)
	if !ok {
		writeError(w, http.StatusBadRequest, "is_favorite must be a boolean")
		return
	}

	userID := req.Header.Get("X-Nomo-User-ID")
	q := url.Values{}
	q.Set(
		"or",
		"(and(user_a_id.eq."+userID+",user_b_id.eq."+friendID+"),"+
			"and(user_b_id.eq."+userID+",user_a_id.eq."+friendID+"))",
	)
	payload := map[string]any{"is_favorite": isFavorite}
	var updated []map[string]any
	if err := r.deps.Supabase.Patch(
		req.Context(),
		authToken,
		"friendships",
		q,
		payload,
		&updated,
	); err != nil {
		writeSupabaseError(w, err)
		return
	}
	if len(updated) == 0 {
		writeError(w, http.StatusNotFound, "friendship not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"updated": len(updated)})
}

func (r *router) listDrinkLogs(w http.ResponseWriter, req *http.Request, authToken string) {
	userID := req.Header.Get("X-Nomo-User-ID")
	visibleUserIDs, err := r.visibleFeedUserIDs(req, authToken, userID)
	if err != nil {
		writeSupabaseError(w, err)
		return
	}
	q := url.Values{}
	q.Set("select", "id,owner_user_id,drank_at,place_name,memo,photo_path,owner:profiles!drink_logs_owner_user_id_fkey(id,user_id,display_name,character_key,avatar_url),drink_log_likes(user_id),drink_log_friends(profiles(id,user_id,display_name,character_key,avatar_url))")
	q.Set("owner_user_id", "in.("+strings.Join(visibleUserIDs, ",")+")")
	q.Set("order", "drank_at.desc")
	var rows []map[string]any
	if err := r.deps.Supabase.Get(req.Context(), authToken, "drink_logs", q, &rows); err != nil {
		writeSupabaseError(w, err)
		return
	}
	enrichDrinkLogLikes(rows, userID)
	writeJSON(w, http.StatusOK, rows)
}

func (r *router) visibleFeedUserIDs(req *http.Request, authToken string, userID string) ([]string, error) {
	q := url.Values{}
	q.Set("select", "user_a_id,user_b_id")
	q.Set("or", "(user_a_id.eq."+userID+",user_b_id.eq."+userID+")")
	var friendships []map[string]any
	if err := r.deps.Supabase.Get(req.Context(), authToken, "friendships", q, &friendships); err != nil {
		return nil, err
	}
	seen := map[string]bool{userID: true}
	ids := []string{userID}
	for _, friendship := range friendships {
		for _, key := range []string{"user_a_id", "user_b_id"} {
			id, _ := friendship[key].(string)
			if id != "" && !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
		}
	}
	return ids, nil
}

func (r *router) deleteDrinkLog(w http.ResponseWriter, req *http.Request, authToken string) {
	logID := strings.TrimSpace(req.PathValue("id"))
	if logID == "" {
		writeError(w, http.StatusBadRequest, "drink log id is required")
		return
	}
	q := url.Values{}
	q.Set("id", "eq."+logID)
	q.Set("owner_user_id", "eq."+req.Header.Get("X-Nomo-User-ID"))
	var deleted []map[string]any
	if err := r.deps.Supabase.Delete(req.Context(), authToken, "drink_logs", q, &deleted); err != nil {
		writeSupabaseError(w, err)
		return
	}
	if len(deleted) == 0 {
		writeError(w, http.StatusNotFound, "drink log not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true, "id": logID})
}

func (r *router) createDrinkLog(w http.ResponseWriter, req *http.Request, authToken string) {
	var input CreateDrinkLogRequest
	if err := json.NewDecoder(req.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	drankAt := time.Now()
	if input.DrankAt != nil {
		drankAt = *input.DrankAt
	}
	payload := map[string]any{
		"owner_user_id": req.Header.Get("X-Nomo-User-ID"),
		"drank_at":      drankAt.Format(time.RFC3339),
		"place_name":    strings.TrimSpace(input.PlaceName),
		"memo":          strings.TrimSpace(input.Memo),
		"photo_path":    strings.TrimSpace(input.PhotoPath),
	}
	var logs []DrinkLog
	if err := r.deps.Supabase.Post(req.Context(), authToken, "drink_logs", nil, payload, &logs); err != nil {
		writeSupabaseError(w, err)
		return
	}
	if len(logs) == 0 {
		writeError(w, http.StatusBadGateway, "drink log insert returned no rows")
		return
	}
	if len(input.FriendIDs) > 0 {
		links := make([]map[string]string, 0, len(input.FriendIDs))
		for _, id := range input.FriendIDs {
			if trimmed := strings.TrimSpace(id); trimmed != "" {
				links = append(links, map[string]string{"drink_log_id": logs[0].ID, "friend_user_id": trimmed})
			}
		}
		var ignored []map[string]any
		if len(links) > 0 {
			if err := r.deps.Supabase.Post(req.Context(), authToken, "drink_log_friends", nil, links, &ignored); err != nil {
				writeSupabaseError(w, err)
				return
			}
		}
	}
	writeJSON(w, http.StatusCreated, logs[0])
}

func (r *router) likeDrinkLog(w http.ResponseWriter, req *http.Request, authToken string) {
	logID := strings.TrimSpace(req.PathValue("id"))
	if logID == "" {
		writeError(w, http.StatusBadRequest, "drink log id is required")
		return
	}
	userID := req.Header.Get("X-Nomo-User-ID")
	q := url.Values{}
	q.Set("on_conflict", "drink_log_id,user_id")
	payload := map[string]any{"drink_log_id": logID, "user_id": userID}
	var ignored []map[string]any
	if err := r.deps.Supabase.PostIgnoreDuplicates(req.Context(), authToken, "drink_log_likes", q, payload, &ignored); err != nil {
		writeSupabaseError(w, err)
		return
	}
	r.writeDrinkLogLikeState(w, req, authToken, logID)
}

func (r *router) unlikeDrinkLog(w http.ResponseWriter, req *http.Request, authToken string) {
	logID := strings.TrimSpace(req.PathValue("id"))
	if logID == "" {
		writeError(w, http.StatusBadRequest, "drink log id is required")
		return
	}
	q := url.Values{}
	q.Set("drink_log_id", "eq."+logID)
	q.Set("user_id", "eq."+req.Header.Get("X-Nomo-User-ID"))
	var ignored []map[string]any
	if err := r.deps.Supabase.Delete(req.Context(), authToken, "drink_log_likes", q, &ignored); err != nil {
		writeSupabaseError(w, err)
		return
	}
	r.writeDrinkLogLikeState(w, req, authToken, logID)
}

func (r *router) writeDrinkLogLikeState(w http.ResponseWriter, req *http.Request, authToken string, logID string) {
	q := url.Values{}
	q.Set("select", "user_id")
	q.Set("drink_log_id", "eq."+logID)
	var rows []map[string]any
	if err := r.deps.Supabase.Get(req.Context(), authToken, "drink_log_likes", q, &rows); err != nil {
		writeSupabaseError(w, err)
		return
	}
	liked := false
	userID := req.Header.Get("X-Nomo-User-ID")
	for _, row := range rows {
		if row["user_id"] == userID {
			liked = true
			break
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"drink_log_id": logID, "like_count": len(rows), "liked_by_me": liked})
}

func enrichDrinkLogLikes(rows []map[string]any, userID string) {
	for _, row := range rows {
		rawLikes, _ := row["drink_log_likes"].([]any)
		liked := false
		for _, raw := range rawLikes {
			like, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			if like["user_id"] == userID {
				liked = true
			}
		}
		row["like_count"] = len(rawLikes)
		row["liked_by_me"] = liked
		delete(row, "drink_log_likes")
	}
}

func (r *router) getDailyStatus(w http.ResponseWriter, req *http.Request, authToken string) {
	date := req.URL.Query().Get("date")
	if date == "" {
		date = time.Now().Format(time.DateOnly)
	}
	q := url.Values{}
	q.Set("select", "user_id,status_date,status,updated_at")
	q.Set("user_id", "eq."+req.Header.Get("X-Nomo-User-ID"))
	q.Set("status_date", "eq."+date)
	var rows []map[string]any
	if err := r.deps.Supabase.Get(req.Context(), authToken, "daily_statuses", q, &rows); err != nil {
		writeSupabaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (r *router) upsertDailyStatus(w http.ResponseWriter, req *http.Request, authToken string) {
	var input DailyStatusRequest
	if err := json.NewDecoder(req.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if input.StatusDate == "" {
		input.StatusDate = time.Now().Format(time.DateOnly)
	}
	if !isValidDailyStatus(input.Status) {
		writeError(w, http.StatusBadRequest, "status is invalid")
		return
	}
	q := url.Values{}
	q.Set("on_conflict", "user_id,status_date")
	payload := map[string]any{"user_id": req.Header.Get("X-Nomo-User-ID"), "status_date": input.StatusDate, "status": input.Status}
	var rows []map[string]any
	if err := r.deps.Supabase.Post(req.Context(), authToken, "daily_statuses", q, payload, &rows); err != nil {
		writeSupabaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func isValidDailyStatus(status string) bool {
	switch status {
	case "unselected",
		"want_drink",
		"busy",
		"can_drink_today",
		"light_drink",
		"want_drink_hard",
		"non_alcohol",
		"liver_rest",
		"waiting_invite",
		"has_plans":
		return true
	default:
		return false
	}
}

func (r *router) auth(next func(http.ResponseWriter, *http.Request, string)) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		auth := req.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			writeError(w, http.StatusUnauthorized, "missing Bearer token")
			return
		}
		token := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
		if token == "" {
			writeError(w, http.StatusUnauthorized, "missing Bearer token")
			return
		}
		userID := strings.TrimSpace(req.Header.Get("X-Nomo-User-ID"))
		if userID == "" {
			writeError(w, http.StatusBadRequest, "X-Nomo-User-ID header is required")
			return
		}
		next(w, req, token)
	}
}

func (r *router) withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		origin := req.Header.Get("Origin")
		if allowedOrigin := r.allowedOrigin(origin); allowedOrigin != "" {
			w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
			w.Header().Set("Vary", "Origin")
		}
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Nomo-User-ID")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, OPTIONS")
		if req.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, req)
	})
}

func (r *router) allowedOrigin(origin string) string {
	for _, allowed := range r.deps.Config.AllowedOrigins {
		if allowed == "*" {
			return "*"
		}
		if origin != "" && origin == allowed {
			return origin
		}
	}
	return ""
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func writeSupabaseError(w http.ResponseWriter, err error) {
	var apiErr supabase.APIError
	if errors.As(err, &apiErr) {
		writeError(w, apiErr.StatusCode, apiErr.Body)
		return
	}
	writeError(w, http.StatusBadGateway, err.Error())
}
