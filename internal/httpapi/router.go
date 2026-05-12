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
	r.mux.HandleFunc("GET /v1/drink-logs", r.auth(r.listDrinkLogs))
	r.mux.HandleFunc("POST /v1/drink-logs", r.auth(r.createDrinkLog))
	r.mux.HandleFunc("GET /v1/daily-status", r.auth(r.getDailyStatus))
	r.mux.HandleFunc("PUT /v1/daily-status", r.auth(r.upsertDailyStatus))
}

func (r *router) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "nomo-backend"})
}

func (r *router) getProfile(w http.ResponseWriter, req *http.Request, authToken string) {
	var rows []Profile
	q := url.Values{}
	q.Set("select", "id,user_id,display_name,character_key,avatar_url,is_plus")
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
	q.Set("select", "user_a_id,user_b_id,user_a:profiles!friendships_user_a_id_fkey(id,user_id,display_name,character_key,avatar_url,is_plus),user_b:profiles!friendships_user_b_id_fkey(id,user_id,display_name,character_key,avatar_url,is_plus)")
	q.Set("or", "(user_a_id.eq."+req.Header.Get("X-Nomo-User-ID")+",user_b_id.eq."+req.Header.Get("X-Nomo-User-ID")+")")
	q.Set("order", "created_at.desc")
	var rows []map[string]any
	if err := r.deps.Supabase.Get(req.Context(), authToken, "friendships", q, &rows); err != nil {
		writeSupabaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (r *router) listDrinkLogs(w http.ResponseWriter, req *http.Request, authToken string) {
	q := url.Values{}
	q.Set("select", "id,drank_at,place_name,memo,photo_path,drink_log_friends(profiles(id,user_id,display_name,character_key,avatar_url,is_plus))")
	q.Set("owner_user_id", "eq."+req.Header.Get("X-Nomo-User-ID"))
	q.Set("order", "drank_at.desc")
	var rows []map[string]any
	if err := r.deps.Supabase.Get(req.Context(), authToken, "drink_logs", q, &rows); err != nil {
		writeSupabaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
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
	if input.Status != "unselected" && input.Status != "want_drink" && input.Status != "busy" {
		writeError(w, http.StatusBadRequest, "status must be unselected, want_drink, or busy")
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
