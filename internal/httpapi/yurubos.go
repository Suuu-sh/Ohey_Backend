package httpapi

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/yota/ohey/backend/internal/supabase"
)

const yuruboSelectColumns = "id,owner_user_id,title,body,category,place_text,place_lat,place_lng,time_label,starts_at,ends_at,status,visibility,expires_at,created_at,updated_at,owner:profiles!yurubos_owner_user_id_fkey(id,user_id,display_name,gender,character_key,avatar_url,is_plus),yurubo_reactions(user_id,reaction_type)"

type yuruboReactionRequest struct {
	ReactionType string `json:"reaction_type"`
}

func (r *router) listYurubos(w http.ResponseWriter, req *http.Request, authToken string) {
	userID := req.Header.Get("X-Ohey-User-ID")
	q := url.Values{}
	q.Set("select", "yurubo_id")
	q.Set("user_id", "eq."+userID)
	var hidden []map[string]any
	hiddenSet := map[string]bool{}
	if err := r.deps.Supabase.Get(req.Context(), authToken, "hidden_yurubos", q, &hidden); err == nil {
		for _, row := range hidden {
			if id, _ := row["yurubo_id"].(string); id != "" {
				hiddenSet[id] = true
			}
		}
	}

	limit := 50
	if raw := req.URL.Query().Get("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}
	q = url.Values{}
	q.Set("select", yuruboSelectColumns)
	q.Set("order", "created_at.desc")
	q.Set("limit", strconv.Itoa(limit))
	q.Set("status", "eq.open")
	var rows []map[string]any
	if err := r.deps.Supabase.Get(req.Context(), authToken, "yurubos", q, &rows); err != nil {
		writeError(w, http.StatusBadGateway, sanitizeSupabaseError(err))
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		id, _ := row["id"].(string)
		if id == "" || hiddenSet[id] {
			continue
		}
		reactions, _ := row["yurubo_reactions"].([]any)
		row["reaction_count"] = len(reactions)
		liked := false
		for _, raw := range reactions {
			if m, ok := raw.(map[string]any); ok && m["user_id"] == userID {
				liked = true
				break
			}
		}
		row["reacted_by_me"] = liked
		out = append(out, row)
	}
	writeJSON(w, http.StatusOK, out)
}

func (r *router) reactYurubo(w http.ResponseWriter, req *http.Request, authToken string) {
	id, msg := cleanUUID(req.PathValue("id"), "yurubo id")
	if msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	var body yuruboReactionRequest
	if !decodeJSONBody(w, req, &body) {
		return
	}
	reaction := strings.TrimSpace(body.ReactionType)
	if reaction == "" {
		reaction = "interested"
	}
	if reaction != "interested" && reaction != "available" && reaction != "another_day" {
		writeError(w, http.StatusBadRequest, "invalid reaction_type")
		return
	}
	payload := map[string]any{"yurubo_id": id, "user_id": req.Header.Get("X-Ohey-User-ID"), "reaction_type": reaction}
	q := url.Values{}
	q.Set("on_conflict", "yurubo_id,user_id")
	var rows []map[string]any
	if err := r.deps.Supabase.Upsert(req.Context(), authToken, "yurubo_reactions", q, payload, &rows); err != nil {
		writeError(w, http.StatusBadGateway, sanitizeSupabaseError(err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"reacted_by_me": true})
}

func (r *router) unreactYurubo(w http.ResponseWriter, req *http.Request, authToken string) {
	id, msg := cleanUUID(req.PathValue("id"), "yurubo id")
	if msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	q := url.Values{}
	q.Set("yurubo_id", "eq."+id)
	q.Set("user_id", "eq."+req.Header.Get("X-Ohey-User-ID"))
	var rows []map[string]any
	if err := r.deps.Supabase.Delete(req.Context(), authToken, "yurubo_reactions", q, &rows); err != nil {
		writeError(w, http.StatusBadGateway, sanitizeSupabaseError(err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"reacted_by_me": false})
}

func sanitizeSupabaseError(err error) string {
	var apiErr supabase.APIError
	if strings.Contains(err.Error(), "PGRST") || strings.Contains(err.Error(), "permission") || strings.Contains(err.Error(), "does not exist") {
		return "upstream database error"
	}
	_ = apiErr
	return "upstream database error"
}
