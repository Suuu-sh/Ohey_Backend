package httpapi

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/yota/nomo/backend/internal/config"
	"github.com/yota/nomo/backend/internal/features/drinklogs"
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
	r.mux.HandleFunc("POST /v1/media/upload-url", r.auth(r.createMediaUploadURL))
	r.mux.HandleFunc("GET /v1/notifications", r.auth(r.listNotifications))
	r.mux.HandleFunc("PATCH /v1/notifications/read-all", r.auth(r.markNotificationsRead))
	r.mux.HandleFunc("PUT /v1/me/push-token", r.auth(r.registerPushToken))
	r.mux.HandleFunc("GET /v1/daily-status", r.auth(r.getDailyStatus))
	r.mux.HandleFunc("PUT /v1/daily-status", r.auth(r.upsertDailyStatus))
	r.mux.HandleFunc("GET /v1/drink-invites/today-reservations", r.auth(r.listTodayReservations))
	r.mux.HandleFunc("GET /v1/drink-invites/incoming-pending", r.auth(r.listIncomingPendingInvites))
	r.mux.HandleFunc("GET /v1/drink-invites/outgoing-active", r.auth(r.listOutgoingActiveInvites))
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
	rows, err := r.drinkLogUsecase(req).ListDrinkLogs(req.Context(), drinklogs.ListInput{
		AuthToken: authToken,
		UserID:    req.Header.Get("X-Nomo-User-ID"),
	})
	if err != nil {
		writeDrinkLogError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func cleanDrinkLogMarkerRarity(value string) string {
	switch strings.TrimSpace(value) {
	case "uncommon", "rare", "super_rare", "ultra_rare", "secret":
		return strings.TrimSpace(value)
	default:
		return "normal"
	}
}

func cleanDrinkLogCaptionY(value *float64) float64 {
	if value == nil {
		return 0.5
	}
	if *value < 0 {
		return 0
	}
	if *value > 1 {
		return 1
	}
	return *value
}

func (r *router) createDrinkLog(w http.ResponseWriter, req *http.Request, authToken string) {
	var input CreateDrinkLogRequest
	if !decodeJSONBody(w, req, &input) {
		return
	}
	row, err := r.drinkLogUsecase(req).CreateDrinkLog(req.Context(), drinklogs.CreateInput{
		AuthToken:             authToken,
		OwnerUserID:           req.Header.Get("X-Nomo-User-ID"),
		DrankAt:               input.DrankAt,
		DrankOn:               input.DrankOn,
		TimezoneOffsetMinutes: input.TimezoneOffsetMinutes,
		PlaceName:             input.PlaceName,
		PlaceLat:              input.PlaceLat,
		PlaceLng:              input.PlaceLng,
		Memo:                  input.Memo,
		CaptionY:              input.CaptionY,
		PhotoPath:             input.PhotoPath,
		FriendIDs:             input.FriendIDs,
		ClientRequestedRarity: input.MarkerRarity,
	})
	if err != nil {
		writeDrinkLogError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, row)
}

func (r *router) deleteDrinkLog(w http.ResponseWriter, req *http.Request, authToken string) {
	row, err := r.drinkLogUsecase(req).DeleteDrinkLog(req.Context(), drinklogs.DeleteInput{
		AuthToken:   authToken,
		LogID:       req.PathValue("id"),
		OwnerUserID: req.Header.Get("X-Nomo-User-ID"),
	})
	if err != nil {
		writeDrinkLogError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, row)
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
