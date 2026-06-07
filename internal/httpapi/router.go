package httpapi

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/yota/ohey/backend/internal/config"
	"github.com/yota/ohey/backend/internal/contracts"
	"github.com/yota/ohey/backend/internal/features/dailystatuses"
	"github.com/yota/ohey/backend/internal/features/friends"
	"github.com/yota/ohey/backend/internal/features/profiles"
	"github.com/yota/ohey/backend/internal/supabase"
)

type Dependencies struct {
	Config        config.Config
	Logger        *slog.Logger
	Supabase      *supabase.Client
	AdminSupabase *supabase.Client
	FCM           *fcmSender
}

type router struct {
	deps         Dependencies
	mux          *http.ServeMux
	rateLimiter  *actionRateLimiter
	authVerifier *authVerifier
}

func NewRouter(deps Dependencies) http.Handler {
	r := &router{
		deps:         deps,
		mux:          http.NewServeMux(),
		rateLimiter:  newActionRateLimiter(timeNow),
		authVerifier: newAuthVerifier(deps.Supabase, deps.Config.SupabaseURL, timeNow),
	}
	r.routes()
	return r.withCORS(r.mux)
}

func route(method, path string) string {
	return method + " " + path
}

func (r *router) routes() {
	r.mux.HandleFunc(route(http.MethodGet, contracts.APIPathHealth), r.health)
	r.mux.HandleFunc(route(http.MethodGet, contracts.APIPathShareYurubo), r.shareYurubo)
	r.mux.HandleFunc(route(http.MethodPost, contracts.APIPathAuthSignup), r.signupWithPassword)
	r.mux.HandleFunc(route(http.MethodGet, contracts.APIPathMeProfile), r.auth(r.getProfile))
	r.mux.HandleFunc(route(http.MethodPut, contracts.APIPathMeProfile), r.auth(r.upsertProfile))
	r.mux.HandleFunc(route(http.MethodPatch, contracts.APIPathMeProfile), r.auth(r.updateProfile))
	r.mux.HandleFunc(route(http.MethodGet, contracts.APIPathProfileByUserID), r.auth(r.getProfileByUserID))
	r.mux.HandleFunc(route(http.MethodGet, contracts.APIPathFriends), r.auth(r.listFriends))
	r.mux.HandleFunc(route(http.MethodGet, contracts.APIPathFriendMonthStats), r.auth(r.listFriendMonthlyDailyStatuses))
	r.mux.HandleFunc(route(http.MethodPost, contracts.APIPathFriends), r.auth(r.createFriendship))
	r.mux.HandleFunc(route(http.MethodDelete, contracts.APIPathFriend), r.auth(r.deleteFriendship))
	r.mux.HandleFunc(route(http.MethodPut, contracts.APIPathFriendFavorite), r.auth(r.updateFriendFavorite))
	r.mux.HandleFunc(route(http.MethodGet, contracts.APIPathFriendGroups), r.auth(r.listFriendGroups))
	r.mux.HandleFunc(route(http.MethodPut, contracts.APIPathFriendGroups), r.auth(r.saveFriendGroups))
	r.mux.HandleFunc(route(http.MethodGet, contracts.APIPathFriendRequests), r.auth(r.listFriendRequests))
	r.mux.HandleFunc(route(http.MethodGet, contracts.APIPathFriendReqStatus), r.auth(r.getFriendRequestStatus))
	r.mux.HandleFunc(route(http.MethodPost, contracts.APIPathFriendRequests), r.auth(r.createFriendRequest))
	r.mux.HandleFunc(route(http.MethodPatch, contracts.APIPathFriendRequest), r.auth(r.updateFriendRequest))
	r.mux.HandleFunc(route(http.MethodGet, contracts.APIPathWishItems), r.auth(r.listWishItems))
	r.mux.HandleFunc(route(http.MethodGet, contracts.APIPathProfileWishItems), r.auth(r.listProfileWishItems))
	r.mux.HandleFunc(route(http.MethodPost, contracts.APIPathWishItems), r.auth(r.createWishItem))
	r.mux.HandleFunc(route(http.MethodPatch, contracts.APIPathWishItem), r.auth(r.updateWishItem))
	r.mux.HandleFunc(route(http.MethodDelete, contracts.APIPathWishItem), r.auth(r.deleteWishItem))
	r.mux.HandleFunc(route(http.MethodGet, contracts.APIPathYurubos), r.auth(r.listYurubos))
	r.mux.HandleFunc(route(http.MethodPost, contracts.APIPathYurubos), r.auth(r.createYurubo))
	r.mux.HandleFunc(route(http.MethodPatch, contracts.APIPathYurubo), r.auth(r.updateYurubo))
	r.mux.HandleFunc(route(http.MethodDelete, contracts.APIPathYurubo), r.auth(r.deleteYurubo))
	r.mux.HandleFunc(route(http.MethodPut, contracts.APIPathYuruboReaction), r.auth(r.reactYurubo))
	r.mux.HandleFunc(route(http.MethodDelete, contracts.APIPathYuruboReaction), r.auth(r.unreactYurubo))
	r.mux.HandleFunc(route(http.MethodPatch, contracts.APIPathYuruboReactionApproval), r.auth(r.updateYuruboReaction))
	r.mux.HandleFunc(route(http.MethodPost, contracts.APIPathUserBlocks), r.auth(r.blockUser))
	r.mux.HandleFunc(route(http.MethodGet, contracts.APIPathUserBlocks), r.auth(r.listBlockedUsers))
	r.mux.HandleFunc(route(http.MethodDelete, contracts.APIPathUserBlock), r.auth(r.unblockUser))
	r.mux.HandleFunc(route(http.MethodPost, contracts.APIPathUserMutes), r.auth(r.muteUser))
	r.mux.HandleFunc(route(http.MethodGet, contracts.APIPathUserMutes), r.auth(r.listMutedUsers))
	r.mux.HandleFunc(route(http.MethodDelete, contracts.APIPathUserMute), r.auth(r.unmuteUser))
	r.mux.HandleFunc(route(http.MethodPost, contracts.APIPathUserReports), r.auth(r.reportUser))
	r.mux.HandleFunc(route(http.MethodGet, contracts.APIPathNotifications), r.auth(r.listNotifications))
	r.mux.HandleFunc(route(http.MethodPatch, contracts.APIPathNotificationsReadAll), r.auth(r.markNotificationsRead))
	r.mux.HandleFunc(route(http.MethodPut, contracts.APIPathMePushToken), r.auth(r.registerPushToken))
	r.mux.HandleFunc(route(http.MethodDelete, contracts.APIPathMePushToken), r.auth(r.unregisterPushToken))
	r.mux.HandleFunc(route(http.MethodDelete, contracts.APIPathMeAccount), r.auth(r.deleteOwnAccount))
	r.mux.HandleFunc(route(http.MethodGet, contracts.APIPathDailyStatus), r.auth(r.getDailyStatus))
	r.mux.HandleFunc(route(http.MethodPut, contracts.APIPathDailyStatus), r.auth(r.upsertDailyStatus))
	r.mux.HandleFunc(route(http.MethodGet, contracts.APIPathMonthlyDailyStatuses), r.auth(r.listMonthlyDailyStatuses))
	r.mux.HandleFunc(route(http.MethodGet, contracts.APIPathTodayReservations), r.auth(r.listTodayReservations))
	r.mux.HandleFunc(route(http.MethodGet, contracts.APIPathIncomingPendingInvites), r.auth(r.listIncomingPendingInvites))
	r.mux.HandleFunc(route(http.MethodGet, contracts.APIPathOutgoingActiveInvites), r.auth(r.listOutgoingActiveInvites))
	r.mux.HandleFunc(route(http.MethodPost, contracts.APIPathInvites), r.auth(r.createInvite))
	r.mux.HandleFunc(route(http.MethodPatch, contracts.APIPathInvite), r.auth(r.updateInvite))
	r.mux.HandleFunc(route(http.MethodGet, contracts.APIPathAdminMe), r.admin(r.adminMe))
	r.mux.HandleFunc(route(http.MethodGet, contracts.APIPathAdminUsers), r.admin(r.adminListUsers))
	r.mux.HandleFunc(route(http.MethodPost, contracts.APIPathAdminUsers), r.admin(r.adminCreateUser))
	r.mux.HandleFunc(route(http.MethodPatch, contracts.APIPathAdminUser), r.admin(r.adminUpdateUser))
	r.mux.HandleFunc(route(http.MethodDelete, contracts.APIPathAdminUser), r.admin(r.adminDeleteUser))
	r.mux.HandleFunc(route(http.MethodGet, contracts.APIPathAdminYurubos), r.admin(r.adminListYurubos))
	r.mux.HandleFunc(route(http.MethodPost, contracts.APIPathAdminYurubos), r.admin(r.adminCreateYurubo))
	r.mux.HandleFunc(route(http.MethodPatch, contracts.APIPathAdminYurubo), r.admin(r.adminUpdateYurubo))
	r.mux.HandleFunc(route(http.MethodDelete, contracts.APIPathAdminYurubo), r.admin(r.adminDeleteYurubo))
	r.mux.HandleFunc(route(http.MethodGet, contracts.APIPathAdminNotificationOutbox), r.admin(r.adminListNotificationOutbox))
	r.mux.HandleFunc(route(http.MethodPost, contracts.APIPathAdminNotificationOutboxProcess), r.admin(r.adminProcessNotificationOutbox))
	r.mux.HandleFunc(route(http.MethodPost, contracts.APIPathAdminNotifications), r.admin(r.adminCreateNotification))
}

func (r *router) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "ohey-backend"})
}

func (r *router) getProfile(w http.ResponseWriter, req *http.Request, authToken string) {
	profile, err := r.profileUsecase().GetProfile(req.Context(), profiles.AuthInput{
		AuthToken:  authToken,
		AuthUserID: req.Header.Get("X-Ohey-User-ID"),
	})
	if err != nil {
		writeProfileError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, profile)
}

func (r *router) updateProfile(w http.ResponseWriter, req *http.Request, authToken string) {
	var body map[string]any
	if !decodeJSONBody(w, req, &body) {
		return
	}
	rows, err := r.profileUsecase().UpdateProfile(req.Context(), profiles.UpdateInput{
		AuthToken:  authToken,
		AuthUserID: req.Header.Get("X-Ohey-User-ID"),
		Body:       body,
	})
	if err != nil {
		writeProfileError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (r *router) listFriends(w http.ResponseWriter, req *http.Request, authToken string) {
	rows, err := r.friendsUsecase(req).ListFriends(req.Context(), friends.ListInput{
		AuthToken: authToken,
		UserID:    req.Header.Get("X-Ohey-User-ID"),
		Date:      dateOnlyParam(req, "date"),
	})
	if err != nil {
		writeFriendsError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (r *router) updateFriendFavorite(w http.ResponseWriter, req *http.Request, authToken string) {
	var input FriendFavoriteRequest
	if !decodeJSONBody(w, req, &input) {
		return
	}
	row, err := r.friendsUsecase(req).UpdateFriendFavorite(req.Context(), friends.FavoriteInput{
		AuthToken:  authToken,
		UserID:     req.Header.Get("X-Ohey-User-ID"),
		FriendID:   req.PathValue("id"),
		IsFavorite: input.IsFavorite,
	})
	if err != nil {
		writeFriendsError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, row)
}

func (r *router) listFriendMonthlyDailyStatuses(w http.ResponseWriter, req *http.Request, authToken string) {
	rows, err := r.dailyStatusUsecase().ListFriendMonthlyStatuses(req.Context(), dailystatuses.FriendMonthInput{
		AuthToken: authToken,
		UserID:    req.Header.Get("X-Ohey-User-ID"),
		FriendID:  req.PathValue("id"),
		Month:     req.URL.Query().Get("month"),
	})
	if err != nil {
		writeDailyStatusError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (r *router) getDailyStatus(w http.ResponseWriter, req *http.Request, authToken string) {
	rows, err := r.dailyStatusUsecase().GetDailyStatus(req.Context(), dailystatuses.GetInput{
		AuthToken: authToken,
		UserID:    req.Header.Get("X-Ohey-User-ID"),
		Date:      req.URL.Query().Get("date"),
	})
	if err != nil {
		writeDailyStatusError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (r *router) upsertDailyStatus(w http.ResponseWriter, req *http.Request, authToken string) {
	var input DailyStatusRequest
	if !decodeJSONBody(w, req, &input) {
		return
	}
	rows, err := r.dailyStatusUsecase().UpsertDailyStatus(req.Context(), dailystatuses.UpsertInput{
		AuthToken:  authToken,
		UserID:     req.Header.Get("X-Ohey-User-ID"),
		StatusDate: input.StatusDate,
		Status:     input.Status,
	})
	if err != nil {
		writeDailyStatusError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (r *router) listMonthlyDailyStatuses(w http.ResponseWriter, req *http.Request, authToken string) {
	rows, err := r.dailyStatusUsecase().ListMonthlyStatuses(req.Context(), dailystatuses.MonthInput{
		AuthToken: authToken,
		UserID:    req.Header.Get("X-Ohey-User-ID"),
		Month:     req.URL.Query().Get("month"),
	})
	if err != nil {
		writeDailyStatusError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (r *router) auth(next func(http.ResponseWriter, *http.Request, string)) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		token, ok := bearerTokenFromRequest(req)
		if !ok {
			writeError(w, http.StatusUnauthorized, "missing Bearer token")
			return
		}
		userID := strings.TrimSpace(req.Header.Get("X-Ohey-User-ID"))
		cleanUserID, errMessage := cleanUUID(userID, "X-Ohey-User-ID")
		if errMessage != "" {
			writeError(w, http.StatusBadRequest, errMessage)
			return
		}
		authUser, err := r.verifyAuthToken(req.Context(), token)
		if err != nil {
			writeAuthVerificationError(w, err)
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
		req.Header.Set("X-Ohey-User-ID", authUserID)
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
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Ohey-User-ID")
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
