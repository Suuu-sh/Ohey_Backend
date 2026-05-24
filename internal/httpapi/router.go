package httpapi

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/yota/nomo/backend/internal/config"
	"github.com/yota/nomo/backend/internal/supabase"
)

type Dependencies struct {
	Config        config.Config
	Logger        *slog.Logger
	Supabase      *supabase.Client
	AdminSupabase *supabase.Client
	FCM           *fcmSender
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
	r.mux.HandleFunc("PUT /v1/me/profile", r.auth(r.upsertProfile))
	r.mux.HandleFunc("PATCH /v1/me/profile", r.auth(r.updateProfile))
	r.mux.HandleFunc("GET /v1/profiles/by-user-id/{user_id}", r.auth(r.getProfileByUserID))
	r.mux.HandleFunc("GET /v1/friends", r.auth(r.listFriends))
	r.mux.HandleFunc("POST /v1/friends", r.auth(r.createFriendship))
	r.mux.HandleFunc("PUT /v1/friends/{id}/favorite", r.auth(r.updateFriendFavorite))
	r.mux.HandleFunc("GET /v1/friend-requests/status", r.auth(r.getFriendRequestStatus))
	r.mux.HandleFunc("POST /v1/friend-requests", r.auth(r.createFriendRequest))
	r.mux.HandleFunc("PATCH /v1/friend-requests/{id}", r.auth(r.updateFriendRequest))
	r.mux.HandleFunc("GET /v1/drink-logs", r.auth(r.listDrinkLogs))
	r.mux.HandleFunc("POST /v1/drink-logs", r.auth(r.createDrinkLog))
	r.mux.HandleFunc("DELETE /v1/drink-logs/{id}", r.auth(r.deleteDrinkLog))
	r.mux.HandleFunc("PUT /v1/drink-logs/{id}/like", r.auth(r.likeDrinkLog))
	r.mux.HandleFunc("DELETE /v1/drink-logs/{id}/like", r.auth(r.unlikeDrinkLog))
	r.mux.HandleFunc("POST /v1/drink-logs/{id}/report", r.auth(r.reportDrinkLog))
	r.mux.HandleFunc("GET /v1/notifications", r.auth(r.listNotifications))
	r.mux.HandleFunc("PATCH /v1/notifications/read-all", r.auth(r.markNotificationsRead))
	r.mux.HandleFunc("PUT /v1/me/push-token", r.auth(r.registerPushToken))
	r.mux.HandleFunc("GET /v1/daily-status", r.auth(r.getDailyStatus))
	r.mux.HandleFunc("PUT /v1/daily-status", r.auth(r.upsertDailyStatus))
	r.mux.HandleFunc("GET /v1/drink-invites/today-reservations", r.auth(r.listTodayReservations))
	r.mux.HandleFunc("GET /v1/drink-invites/incoming-pending", r.auth(r.listIncomingPendingInvites))
	r.mux.HandleFunc("POST /v1/drink-invites", r.auth(r.createDrinkInvite))
	r.mux.HandleFunc("PATCH /v1/drink-invites/{id}", r.auth(r.updateDrinkInvite))
	r.mux.HandleFunc("GET /v1/admin/me", r.admin(r.adminMe))
	r.mux.HandleFunc("GET /v1/admin/users", r.admin(r.adminListUsers))
	r.mux.HandleFunc("POST /v1/admin/users", r.admin(r.adminCreateUser))
	r.mux.HandleFunc("PATCH /v1/admin/users/{id}", r.admin(r.adminUpdateUser))
	r.mux.HandleFunc("DELETE /v1/admin/users/{id}", r.admin(r.adminDeleteUser))
	r.mux.HandleFunc("GET /v1/admin/drink-logs", r.admin(r.adminListDrinkLogs))
	r.mux.HandleFunc("POST /v1/admin/drink-logs", r.admin(r.adminCreateDrinkLog))
	r.mux.HandleFunc("PATCH /v1/admin/drink-logs/{id}", r.admin(r.adminUpdateDrinkLog))
	r.mux.HandleFunc("DELETE /v1/admin/drink-logs/{id}", r.admin(r.adminDeleteDrinkLog))
	r.mux.HandleFunc("POST /v1/admin/notifications", r.admin(r.adminCreateNotification))
}

func (r *router) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "nomo-backend"})
}

func (r *router) getProfile(w http.ResponseWriter, req *http.Request, authToken string) {
	var rows []Profile
	q := url.Values{}
	q.Set("select", "id,user_id,display_name,gender,character_key,avatar_url,is_plus")
	q.Set("id", "eq."+req.Header.Get("X-Nomo-User-ID"))
	if err := r.deps.Supabase.Get(req.Context(), authToken, "profiles", q, &rows); err != nil {
		writeSupabaseError(w, err)
		return
	}
	if len(rows) == 0 {
		writeError(w, http.StatusNotFound, "profile not found")
		return
	}
	writeJSON(w, http.StatusOK, rows[0])
}

func (r *router) updateProfile(w http.ResponseWriter, req *http.Request, authToken string) {
	var body map[string]any
	if !decodeJSONBody(w, req, &body) {
		return
	}
	allowed := map[string]any{}
	for _, key := range []string{"display_name", "character_key", "avatar_url"} {
		if value, ok := body[key]; ok {
			allowed[key] = value
		}
	}
	if _, ok := body["gender"]; ok {
		writeError(w, http.StatusBadRequest, "gender cannot be changed")
		return
	}
	if value, ok := body["user_id"]; ok {
		allowed["user_id"] = value
	}
	if errMessage := validateProfilePayload(req, authToken, allowed); errMessage != "" {
		writeError(w, http.StatusBadRequest, errMessage)
		return
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
	q.Set(
		"select",
		"user_a_id,user_b_id,is_favorite,user_a:profiles!friendships_user_a_id_fkey(id,user_id,display_name,gender,character_key,avatar_url,is_plus),user_b:profiles!friendships_user_b_id_fkey(id,user_id,display_name,gender,character_key,avatar_url,is_plus)",
	)
	q.Set("or", "(user_a_id.eq."+req.Header.Get("X-Nomo-User-ID")+",user_b_id.eq."+req.Header.Get("X-Nomo-User-ID")+")")
	q.Set("order", "created_at.desc")
	var rows []map[string]any
	if err := r.deps.Supabase.Get(req.Context(), authToken, "friendships", q, &rows); err != nil {
		writeSupabaseError(w, err)
		return
	}
	if err := r.attachTodayStatuses(req, authToken, rows); err != nil {
		writeSupabaseError(w, err)
		return
	}
	if err := r.attachFriendDrinkStats(req, authToken, rows); err != nil {
		if r.deps.Logger != nil {
			r.deps.Logger.Warn("failed to attach friend drink stats", "error", err)
		}
	}
	writeJSON(w, http.StatusOK, rows)
}

func (r *router) updateFriendFavorite(w http.ResponseWriter, req *http.Request, authToken string) {
	friendID, errMessage := cleanUUID(req.PathValue("id"), "friend id")
	if errMessage != "" {
		writeError(w, http.StatusBadRequest, errMessage)
		return
	}
	var input FriendFavoriteRequest
	if !decodeJSONBody(w, req, &input) {
		return
	}
	userID := req.Header.Get("X-Nomo-User-ID")
	q := url.Values{}
	q.Set("or", "(and(user_a_id.eq."+userID+",user_b_id.eq."+friendID+"),and(user_a_id.eq."+friendID+",user_b_id.eq."+userID+"))")
	var rows []map[string]any
	if err := r.deps.Supabase.Patch(req.Context(), authToken, "friendships", q, map[string]any{"is_favorite": input.IsFavorite}, &rows); err != nil {
		writeSupabaseError(w, err)
		return
	}
	if len(rows) == 0 {
		writeError(w, http.StatusNotFound, "friendship not found")
		return
	}
	writeJSON(w, http.StatusOK, rows[0])
}

func (r *router) listDrinkLogs(w http.ResponseWriter, req *http.Request, authToken string) {
	userID := req.Header.Get("X-Nomo-User-ID")
	visibleUserIDs, err := r.visibleFeedUserIDs(req, authToken, userID)
	if err != nil {
		writeSupabaseError(w, err)
		return
	}

	selectColumns := "id,owner_user_id,drank_at,place_name,memo,photo_path,link_url,marker_rarity,is_official,owner:profiles!drink_logs_owner_user_id_fkey(id,user_id,display_name,gender,character_key,avatar_url,is_plus),drink_log_likes(user_id),drink_log_friends(profiles(id,user_id,display_name,gender,character_key,avatar_url,is_plus))"
	q := url.Values{}
	q.Set("select", selectColumns)
	q.Set("owner_user_id", "in.("+strings.Join(visibleUserIDs, ",")+")")
	q.Set("order", "drank_at.desc")
	var rows []map[string]any
	if err := r.deps.Supabase.Get(req.Context(), authToken, "drink_logs", q, &rows); err != nil {
		writeSupabaseError(w, err)
		return
	}

	officialQ := url.Values{}
	officialQ.Set("select", selectColumns)
	officialQ.Set("is_official", "eq.true")
	officialQ.Set("order", "drank_at.desc")
	var officialRows []map[string]any
	if err := r.deps.Supabase.Get(req.Context(), authToken, "drink_logs", officialQ, &officialRows); err != nil {
		writeSupabaseError(w, err)
		return
	}
	rows = appendUniqueDrinkLogRows(rows, officialRows...)

	for _, row := range rows {
		rawLikes, _ := row["drink_log_likes"].([]any)
		row["like_count"] = len(rawLikes)
		likedByMe := false
		for _, rawLike := range rawLikes {
			like, ok := rawLike.(map[string]any)
			if ok && like["user_id"] == userID {
				likedByMe = true
				break
			}
		}
		row["liked_by_me"] = likedByMe
	}
	sortDrinkLogRowsByDrankAtDesc(rows)
	writeJSON(w, http.StatusOK, rows)
}

func (r *router) visibleFeedUserIDs(req *http.Request, authToken, userID string) ([]string, error) {
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
			id, ok := friendship[key].(string)
			if ok && id != "" && !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
		}
	}
	return ids, nil
}

func appendUniqueDrinkLogRows(rows []map[string]any, extraRows ...map[string]any) []map[string]any {
	seen := make(map[string]bool, len(rows)+len(extraRows))
	for _, row := range rows {
		if id, _ := row["id"].(string); id != "" {
			seen[id] = true
		}
	}
	for _, row := range extraRows {
		id, _ := row["id"].(string)
		if id != "" && seen[id] {
			continue
		}
		if id != "" {
			seen[id] = true
		}
		rows = append(rows, row)
	}
	return rows
}

func sortDrinkLogRowsByDrankAtDesc(rows []map[string]any) {
	sort.SliceStable(rows, func(i, j int) bool {
		return drinkLogRowTime(rows[i]).After(drinkLogRowTime(rows[j]))
	})
}

func drinkLogRowTime(row map[string]any) time.Time {
	value, _ := row["drank_at"].(string)
	parsed, err := time.Parse(time.RFC3339, value)
	if err == nil {
		return parsed
	}
	return time.Time{}
}

func cleanDrinkLogMarkerRarity(value string) string {
	switch strings.TrimSpace(value) {
	case "uncommon", "rare", "super_rare", "ultra_rare", "secret":
		return strings.TrimSpace(value)
	default:
		return "normal"
	}
}

func (r *router) createDrinkLog(w http.ResponseWriter, req *http.Request, authToken string) {
	var input CreateDrinkLogRequest
	if !decodeJSONBody(w, req, &input) {
		return
	}
	friendIDs, errMessage := cleanUUIDs(input.FriendIDs, "friend id")
	if errMessage != "" {
		writeError(w, http.StatusBadRequest, errMessage)
		return
	}
	ownerUserID := req.Header.Get("X-Nomo-User-ID")
	validFriends, err := r.validateDrinkLogFriendIDs(req, authToken, ownerUserID, friendIDs)
	if err != nil {
		writeSupabaseError(w, err)
		return
	}
	if !validFriends {
		writeError(w, http.StatusForbidden, "friend_ids must be existing friends")
		return
	}
	drankAt := time.Now()
	if input.DrankAt != nil {
		drankAt = *input.DrankAt
	}
	available, errMessage, err := r.dailyDrinkLogAvailable(req, authToken, ownerUserID, input, drankAt)
	if err != nil {
		writeSupabaseError(w, err)
		return
	}
	if errMessage != "" {
		writeError(w, http.StatusBadRequest, errMessage)
		return
	}
	if !available {
		writeError(w, http.StatusConflict, "投稿は1日1回までです")
		return
	}
	payload := map[string]any{
		"owner_user_id": ownerUserID,
		"drank_at":      drankAt.Format(time.RFC3339),
		"place_name":    strings.TrimSpace(input.PlaceName),
		"memo":          strings.TrimSpace(input.Memo),
		"photo_path":    strings.TrimSpace(input.PhotoPath),
		"marker_rarity": cleanDrinkLogMarkerRarity(input.MarkerRarity),
		"is_official":   false,
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
	if len(friendIDs) > 0 {
		links := drinkLogFriendLinks(logs[0].ID, friendIDs)
		var ignored []map[string]any
		if err := r.deps.Supabase.Post(req.Context(), authToken, "drink_log_friends", nil, links, &ignored); err != nil {
			writeSupabaseError(w, err)
			return
		}
	}
	r.createDrinkLogTaggedNotifications(req, authToken, logs[0].ID, ownerUserID, friendIDs)
	writeJSON(w, http.StatusCreated, logs[0])
}

func (r *router) dailyDrinkLogAvailable(req *http.Request, authToken, ownerUserID string, input CreateDrinkLogRequest, drankAt time.Time) (bool, string, error) {
	start, end, errMessage := drinkLogDayWindow(input, drankAt)
	if errMessage != "" {
		return false, errMessage, nil
	}
	q := url.Values{}
	q.Set("select", "id")
	q.Set("owner_user_id", "eq."+ownerUserID)
	q.Set("is_official", "eq.false")
	q.Add("drank_at", "gte."+start.Format(time.RFC3339))
	q.Add("drank_at", "lt."+end.Format(time.RFC3339))
	q.Set("limit", "1")
	var rows []map[string]any
	if err := r.deps.Supabase.Get(req.Context(), authToken, "drink_logs", q, &rows); err != nil {
		return false, "", err
	}
	return len(rows) == 0, "", nil
}

func drinkLogDayWindow(input CreateDrinkLogRequest, drankAt time.Time) (time.Time, time.Time, string) {
	drankOn := strings.TrimSpace(input.DrankOn)
	if drankOn == "" {
		utc := drankAt.UTC()
		start := time.Date(utc.Year(), utc.Month(), utc.Day(), 0, 0, 0, 0, time.UTC)
		return start, start.AddDate(0, 0, 1), ""
	}

	day, err := time.Parse(time.DateOnly, drankOn)
	if err != nil {
		return time.Time{}, time.Time{}, "drank_on must be YYYY-MM-DD"
	}
	offsetMinutes := 0
	if input.TimezoneOffsetMinutes != nil {
		offsetMinutes = *input.TimezoneOffsetMinutes
	}
	if offsetMinutes < -14*60 || offsetMinutes > 14*60 {
		return time.Time{}, time.Time{}, "timezone_offset_minutes is out of range"
	}
	location := time.FixedZone("client", offsetMinutes*60)
	start := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, location)
	end := start.AddDate(0, 0, 1)
	return start.UTC(), end.UTC(), ""
}

func (r *router) validateDrinkLogFriendIDs(req *http.Request, authToken, ownerUserID string, friendIDs []string) (bool, error) {
	for _, friendID := range friendIDs {
		if friendID == ownerUserID {
			return false, nil
		}
		ok, err := r.friendshipExists(req, authToken, ownerUserID, friendID)
		if err != nil {
			return false, err
		}
		if !ok {
			return false, nil
		}
	}
	return true, nil
}

func (r *router) deleteDrinkLog(w http.ResponseWriter, req *http.Request, authToken string) {
	logID, errMessage := cleanUUID(req.PathValue("id"), "drink log id")
	if errMessage != "" {
		writeError(w, http.StatusBadRequest, errMessage)
		return
	}
	q := url.Values{}
	q.Set("id", "eq."+logID)
	q.Set("owner_user_id", "eq."+req.Header.Get("X-Nomo-User-ID"))
	var rows []DrinkLog
	if err := r.deps.Supabase.Delete(req.Context(), authToken, "drink_logs", q, &rows); err != nil {
		writeSupabaseError(w, err)
		return
	}
	if len(rows) == 0 {
		writeError(w, http.StatusNotFound, "drink log not found")
		return
	}
	writeJSON(w, http.StatusOK, rows[0])
}

func (r *router) getDailyStatus(w http.ResponseWriter, req *http.Request, authToken string) {
	date, errMessage := cleanDateOnlyOrToday(req.URL.Query().Get("date"), "date")
	if errMessage != "" {
		writeError(w, http.StatusBadRequest, errMessage)
		return
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
	if !decodeJSONBody(w, req, &input) {
		return
	}
	statusDate, errMessage := cleanDateOnlyOrToday(input.StatusDate, "status_date")
	if errMessage != "" {
		writeError(w, http.StatusBadRequest, errMessage)
		return
	}
	input.Status = strings.TrimSpace(input.Status)
	if !isValidDailyStatus(input.Status) {
		writeError(w, http.StatusBadRequest, "status is invalid")
		return
	}
	q := url.Values{}
	q.Set("on_conflict", "user_id,status_date")
	payload := map[string]any{"user_id": req.Header.Get("X-Nomo-User-ID"), "status_date": statusDate, "status": input.Status}
	var rows []map[string]any
	if err := r.deps.Supabase.Upsert(req.Context(), authToken, "daily_statuses", q, payload, &rows); err != nil {
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
		cleanUserID, errMessage := cleanUUID(userID, "X-Nomo-User-ID")
		if errMessage != "" {
			writeError(w, http.StatusBadRequest, errMessage)
			return
		}
		var authUser AuthUser
		if err := r.deps.Supabase.GetAuthUser(req.Context(), token, &authUser); err != nil {
			writeSupabaseError(w, err)
			return
		}
		authUserID, errMessage := cleanUUID(authUser.ID, "auth user id")
		if errMessage != "" {
			writeError(w, http.StatusUnauthorized, "invalid auth user")
			return
		}
		if cleanUserID != authUserID {
			writeError(w, http.StatusForbidden, "auth user mismatch")
			return
		}
		req.Header.Set("X-Nomo-User-ID", authUserID)
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
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
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
		writeError(w, apiErr.StatusCode, safeSupabaseErrorMessage(apiErr.StatusCode))
		return
	}
	writeError(w, http.StatusBadGateway, "upstream service error")
}

func safeSupabaseErrorMessage(status int) string {
	switch {
	case status == http.StatusUnauthorized:
		return "authentication failed"
	case status == http.StatusForbidden:
		return "access denied"
	case status == http.StatusNotFound:
		return "resource not found"
	case status == http.StatusConflict:
		return "request conflicts with existing data"
	case status == http.StatusTooManyRequests:
		return "too many requests"
	case status >= 500:
		return "upstream service error"
	default:
		return "request rejected by upstream service"
	}
}
