package httpapi

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/yota/ohey/backend/internal/contracts"
)

const wishItemSelectColumns = "id,owner_user_id,title,note,category,place_text,place_url,visibility,status,created_at,updated_at"

type wishItemCreateRequest struct {
	Title      string `json:"title"`
	Note       string `json:"note"`
	Category   string `json:"category"`
	PlaceText  string `json:"place_text"`
	PlaceURL   string `json:"place_url"`
	Visibility string `json:"visibility"`
}

func (r *router) listWishItems(w http.ResponseWriter, req *http.Request, authToken string) {
	userID := req.Header.Get("X-Ohey-User-ID")
	limit := 50
	if raw := req.URL.Query().Get("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}
	q := url.Values{}
	q.Set("select", wishItemSelectColumns)
	q.Set("owner_user_id", "eq."+userID)
	q.Set("status", "eq."+contracts.StatusActive)
	q.Set("order", "created_at.desc")
	q.Set("limit", strconv.Itoa(limit))
	var rows []map[string]any
	if err := r.deps.Supabase.Get(req.Context(), authToken, "wish_items", q, &rows); err != nil {
		writeError(w, http.StatusBadGateway, sanitizeSupabaseError(err))
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (r *router) listProfileWishItems(w http.ResponseWriter, req *http.Request, authToken string) {
	profileID, msg := cleanUUID(req.PathValue("id"), "profile id")
	if msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	limit := 30
	if raw := req.URL.Query().Get("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}
	q := url.Values{}
	q.Set("select", wishItemSelectColumns)
	q.Set("owner_user_id", "eq."+profileID)
	q.Set("visibility", "eq."+contracts.VisibilityFriends)
	q.Set("status", "eq."+contracts.StatusActive)
	q.Set("order", "created_at.desc")
	q.Set("limit", strconv.Itoa(limit))
	var rows []map[string]any
	if err := r.deps.Supabase.Get(req.Context(), authToken, "wish_items", q, &rows); err != nil {
		writeError(w, http.StatusBadGateway, sanitizeSupabaseError(err))
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (r *router) createWishItem(w http.ResponseWriter, req *http.Request, authToken string) {
	var body wishItemCreateRequest
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
	category := strings.TrimSpace(body.Category)
	if category == "" {
		category = contracts.CategoryOther
	}
	visibility := strings.TrimSpace(body.Visibility)
	if visibility == "" {
		visibility = contracts.VisibilityPrivate
	}
	if visibility != contracts.VisibilityPrivate && visibility != contracts.VisibilityFriends {
		writeError(w, http.StatusBadRequest, "invalid visibility")
		return
	}
	payload := map[string]any{
		"owner_user_id": req.Header.Get("X-Ohey-User-ID"),
		"title":         title,
		"note":          strings.TrimSpace(body.Note),
		"category":      category,
		"place_text":    strings.TrimSpace(body.PlaceText),
		"place_url":     strings.TrimSpace(body.PlaceURL),
		"visibility":    visibility,
	}
	var rows []map[string]any
	if err := r.deps.Supabase.Post(req.Context(), authToken, "wish_items", nil, payload, &rows); err != nil {
		writeError(w, http.StatusBadGateway, sanitizeSupabaseError(err))
		return
	}
	if len(rows) == 0 {
		writeError(w, http.StatusBadGateway, "wish item insert returned no rows")
		return
	}
	writeJSON(w, http.StatusCreated, rows[0])
}
