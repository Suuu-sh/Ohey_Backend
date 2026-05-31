package httpapi

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/yota/ohey/backend/internal/supabase"
)

const yuruboSelectColumns = "id,wish_item_id,owner_user_id,title,body,category,place_text,place_lat,place_lng,time_label,starts_at,ends_at,status,visibility,expires_at,created_at,updated_at,owner:profiles!yurubos_owner_user_id_fkey(id,user_id,display_name,gender,character_key,avatar_url,is_plus)"

type yuruboReactionRequest struct {
	ReactionType string `json:"reaction_type"`
}

type yuruboCreateRequest struct {
	Title      string `json:"title"`
	Body       string `json:"body"`
	Category   string `json:"category"`
	PlaceText  string `json:"place_text"`
	TimeLabel  string `json:"time_label"`
	Visibility string `json:"visibility"`
	GroupID    string `json:"group_id"`
	WishItemID string `json:"wish_item_id"`
}

type yuruboUpdateRequest struct {
	Title     string `json:"title"`
	Body      string `json:"body"`
	PlaceText string `json:"place_text"`
	TimeLabel string `json:"time_label"`
}

func (r *router) createYurubo(w http.ResponseWriter, req *http.Request, authToken string) {
	var body yuruboCreateRequest
	if !decodeJSONBody(w, req, &body) {
		return
	}
	title := strings.TrimSpace(body.Title)
	if title == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}
	if len([]rune(title)) > 80 {
		writeError(w, http.StatusBadRequest, "title is too long")
		return
	}
	visibility := strings.TrimSpace(body.Visibility)
	if visibility == "" {
		visibility = "friends"
	}
	if visibility != "friends" && visibility != "group" {
		writeError(w, http.StatusBadRequest, "invalid visibility")
		return
	}
	var groupID string
	if visibility == "group" {
		var msg string
		groupID, msg = cleanUUID(body.GroupID, "group id")
		if msg != "" {
			writeError(w, http.StatusBadRequest, msg)
			return
		}
	}
	category := strings.TrimSpace(body.Category)
	if category == "" {
		category = "other"
	}
	payload := map[string]any{
		"owner_user_id": req.Header.Get("X-Ohey-User-ID"),
		"title":         title,
		"body":          strings.TrimSpace(body.Body),
		"category":      category,
		"place_text":    strings.TrimSpace(body.PlaceText),
		"time_label":    strings.TrimSpace(body.TimeLabel),
		"visibility":    visibility,
	}
	if strings.TrimSpace(body.WishItemID) != "" {
		wishItemID, msg := cleanUUID(body.WishItemID, "wish item id")
		if msg != "" {
			writeError(w, http.StatusBadRequest, msg)
			return
		}
		q := url.Values{}
		q.Set("select", "id")
		q.Set("id", "eq."+wishItemID)
		q.Set("owner_user_id", "eq."+req.Header.Get("X-Ohey-User-ID"))
		q.Set("limit", "1")
		var wishRows []map[string]any
		if err := r.deps.Supabase.Get(req.Context(), authToken, "wish_items", q, &wishRows); err != nil {
			writeError(w, http.StatusBadGateway, sanitizeSupabaseError(err))
			return
		}
		if len(wishRows) == 0 {
			writeError(w, http.StatusBadRequest, "wish item not found")
			return
		}
		payload["wish_item_id"] = wishItemID
	}
	var rows []map[string]any
	if err := r.deps.Supabase.Post(req.Context(), authToken, "yurubos", nil, payload, &rows); err != nil {
		writeError(w, http.StatusBadGateway, sanitizeSupabaseError(err))
		return
	}
	if len(rows) == 0 {
		writeError(w, http.StatusBadGateway, "yurubo insert returned no rows")
		return
	}
	yuruboID, _ := rows[0]["id"].(string)
	if visibility == "group" {
		var ignored []map[string]any
		if err := r.deps.Supabase.Post(req.Context(), authToken, "yurubo_visibility_groups", nil, map[string]any{"yurubo_id": yuruboID, "group_id": groupID}, &ignored); err != nil {
			writeError(w, http.StatusBadGateway, sanitizeSupabaseError(err))
			return
		}
	}
	writeJSON(w, http.StatusCreated, rows[0])
}

func (r *router) updateYurubo(w http.ResponseWriter, req *http.Request, authToken string) {
	id, msg := cleanUUID(req.PathValue("id"), "yurubo id")
	if msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	var body yuruboUpdateRequest
	if !decodeJSONBody(w, req, &body) {
		return
	}
	title := strings.TrimSpace(body.Title)
	if title == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}
	if len([]rune(title)) > 80 {
		writeError(w, http.StatusBadRequest, "title is too long")
		return
	}
	q := url.Values{}
	q.Set("id", "eq."+id)
	q.Set("owner_user_id", "eq."+req.Header.Get("X-Ohey-User-ID"))
	payload := map[string]any{
		"title":      title,
		"body":       strings.TrimSpace(body.Body),
		"place_text": strings.TrimSpace(body.PlaceText),
		"time_label": strings.TrimSpace(body.TimeLabel),
	}
	var rows []map[string]any
	if err := r.deps.Supabase.Patch(req.Context(), authToken, "yurubos", q, payload, &rows); err != nil {
		writeError(w, http.StatusBadGateway, sanitizeSupabaseError(err))
		return
	}
	if len(rows) == 0 {
		writeError(w, http.StatusNotFound, "yurubo not found")
		return
	}
	writeJSON(w, http.StatusOK, rows[0])
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
	ids := make([]string, 0, len(rows))
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		id, _ := row["id"].(string)
		if id == "" || hiddenSet[id] {
			continue
		}
		ids = append(ids, id)
		out = append(out, row)
	}
	reactionCounts, reactedByMe, participants := r.yuruboReactionSummaries(req, authToken, ids, userID)
	visibilityLabels := r.yuruboVisibilityLabels(req, authToken, out)
	for _, row := range out {
		id, _ := row["id"].(string)
		row["reaction_count"] = reactionCounts[id]
		row["reacted_by_me"] = reactedByMe[id]
		row["participants"] = participants[id]
		if label := visibilityLabels[id]; label != "" {
			row["visibility_label"] = label
		} else {
			row["visibility_label"] = "全フレンズ"
		}
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

func (r *router) yuruboReactionSummaries(req *http.Request, authToken string, ids []string, userID string) (map[string]int, map[string]bool, map[string][]map[string]any) {
	counts := map[string]int{}
	reactedByMe := map[string]bool{}
	participants := map[string][]map[string]any{}
	if len(ids) == 0 {
		return counts, reactedByMe, participants
	}
	q := url.Values{}
	q.Set("select", "yurubo_id,user_id")
	q.Set("yurubo_id", "in.("+strings.Join(ids, ",")+")")
	var rows []map[string]any
	if err := r.deps.Supabase.Get(req.Context(), authToken, "yurubo_reactions", q, &rows); err != nil {
		return counts, reactedByMe, participants
	}
	userIDs := []string{}
	seenUsers := map[string]bool{}
	for _, row := range rows {
		id, _ := row["yurubo_id"].(string)
		if id == "" {
			continue
		}
		counts[id]++
		actor, _ := row["user_id"].(string)
		if actor == userID {
			reactedByMe[id] = true
		}
		if actor != "" && !seenUsers[actor] {
			seenUsers[actor] = true
			userIDs = append(userIDs, actor)
		}
	}
	profilesByID := r.yuruboParticipantProfiles(req, authToken, userIDs)
	for _, row := range rows {
		id, _ := row["yurubo_id"].(string)
		actor, _ := row["user_id"].(string)
		if id == "" || actor == "" {
			continue
		}
		profile := profilesByID[actor]
		if profile == nil {
			profile = map[string]any{"id": actor}
		}
		participants[id] = append(participants[id], profile)
	}
	return counts, reactedByMe, participants
}

func (r *router) yuruboParticipantProfiles(req *http.Request, authToken string, userIDs []string) map[string]map[string]any {
	profilesByID := map[string]map[string]any{}
	if len(userIDs) == 0 {
		return profilesByID
	}
	q := url.Values{}
	q.Set("select", "id,user_id,display_name,avatar_url")
	q.Set("id", "in.("+strings.Join(userIDs, ",")+")")
	var profiles []map[string]any
	if err := r.deps.Supabase.Get(req.Context(), authToken, "profiles", q, &profiles); err != nil {
		return profilesByID
	}
	for _, profile := range profiles {
		id, _ := profile["id"].(string)
		if id != "" {
			profilesByID[id] = profile
		}
	}
	return profilesByID
}

func (r *router) yuruboVisibilityLabels(req *http.Request, authToken string, rows []map[string]any) map[string]string {
	labels := map[string]string{}
	groupIDs := []string{}
	for _, row := range rows {
		id, _ := row["id"].(string)
		visibility, _ := row["visibility"].(string)
		if visibility != "group" {
			labels[id] = "全フレンズ"
			continue
		}
		groupIDs = append(groupIDs, id)
	}
	if len(groupIDs) == 0 {
		return labels
	}
	q := url.Values{}
	q.Set("select", "yurubo_id,friend_groups(name)")
	q.Set("yurubo_id", "in.("+strings.Join(groupIDs, ",")+")")
	var links []map[string]any
	if err := r.deps.Supabase.Get(req.Context(), authToken, "yurubo_visibility_groups", q, &links); err != nil {
		for _, id := range groupIDs {
			labels[id] = "グループ"
		}
		return labels
	}
	for _, link := range links {
		id, _ := link["yurubo_id"].(string)
		group, _ := link["friend_groups"].(map[string]any)
		name, _ := group["name"].(string)
		if strings.TrimSpace(name) == "" {
			name = "グループ"
		}
		labels[id] = strings.TrimSpace(name)
	}
	return labels
}

func yuruboVisibilityLabel(row map[string]any) string {
	visibility, _ := row["visibility"].(string)
	if visibility != "group" {
		return "全フレンズ"
	}
	rawGroups, _ := row["yurubo_visibility_groups"].([]any)
	for _, raw := range rawGroups {
		link, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		group, ok := link["friend_groups"].(map[string]any)
		if !ok {
			continue
		}
		if name, _ := group["name"].(string); strings.TrimSpace(name) != "" {
			return strings.TrimSpace(name)
		}
	}
	return "グループ"
}

func sanitizeSupabaseError(err error) string {
	var apiErr supabase.APIError
	if strings.Contains(err.Error(), "PGRST") || strings.Contains(err.Error(), "permission") || strings.Contains(err.Error(), "does not exist") {
		return "upstream database error"
	}
	_ = apiErr
	return "upstream database error"
}
