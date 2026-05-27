package httpapi

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/yota/nomo/backend/internal/features/drinkinvites"
	"github.com/yota/nomo/backend/internal/features/drinklogs"
	"github.com/yota/nomo/backend/internal/features/friends"
	"github.com/yota/nomo/backend/internal/features/notifications"
)

type ProfileSaveRequest struct {
	UserID       string `json:"user_id"`
	DisplayName  string `json:"display_name"`
	Gender       string `json:"gender"`
	CharacterKey string `json:"character_key"`
	AvatarURL    string `json:"avatar_url"`
}

type FriendIDRequest struct {
	FriendID string `json:"friend_id"`
	ToUserID string `json:"to_user_id"`
}

type FriendRequestUpdateRequest struct {
	Status string `json:"status"`
}

type DrinkInviteRequest struct {
	ToUserID   string `json:"to_user_id"`
	InviteDate string `json:"invite_date"`
}

type DrinkInviteUpdateRequest struct {
	Status string `json:"status"`
}

type DrinkLogReportRequest struct {
	Reason string `json:"reason"`
}

func (r *router) upsertProfile(w http.ResponseWriter, req *http.Request, authToken string) {
	var input ProfileSaveRequest
	if !decodeJSONBody(w, req, &input) {
		return
	}
	input.normalize()
	if errMessage := input.validate(); errMessage != "" {
		writeError(w, http.StatusBadRequest, errMessage)
		return
	}

	payload := input.profilePayload(req.Header.Get("X-Nomo-User-ID"))
	q := url.Values{}
	q.Set("on_conflict", "id")
	var rows []Profile
	if err := r.deps.Supabase.Upsert(req.Context(), authToken, "profiles", q, payload, &rows); err != nil {
		writeSupabaseError(w, err)
		return
	}
	if len(rows) == 0 {
		writeJSON(w, http.StatusOK, payload)
		return
	}
	writeJSON(w, http.StatusOK, rows[0])
}

func (r *router) getProfileByUserID(w http.ResponseWriter, req *http.Request, authToken string) {
	userID := strings.TrimSpace(req.PathValue("user_id"))
	if !adminUserIDPattern.MatchString(userID) {
		writeError(w, http.StatusBadRequest, "user_id must be 3-24 letters, numbers, or underscores")
		return
	}

	q := url.Values{}
	q.Set("select", "id,user_id,display_name,gender,character_key,avatar_url,is_plus")
	q.Set("user_id", "eq."+userID)
	q.Set("limit", "1")
	var rows []Profile
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

func (r *router) createFriendship(w http.ResponseWriter, req *http.Request, authToken string) {
	var input FriendIDRequest
	if !decodeJSONBody(w, req, &input) {
		return
	}
	friendID := strings.TrimSpace(input.FriendID)
	if friendID == "" {
		friendID = strings.TrimSpace(input.ToUserID)
	}
	row, err := r.friendsUsecase(req).CreateFriendship(req.Context(), friends.FriendInput{
		AuthToken: authToken,
		UserID:    req.Header.Get("X-Nomo-User-ID"),
		FriendID:  friendID,
	})
	if err != nil {
		writeFriendsError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, row)
}

func (r *router) getFriendRequestStatus(w http.ResponseWriter, req *http.Request, authToken string) {
	status, err := r.friendsUsecase(req).GetFriendRequestStatus(req.Context(), friends.FriendInput{
		AuthToken: authToken,
		UserID:    req.Header.Get("X-Nomo-User-ID"),
		FriendID:  req.URL.Query().Get("friend_id"),
	})
	if err != nil {
		writeFriendsError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (r *router) createFriendRequest(w http.ResponseWriter, req *http.Request, authToken string) {
	var input FriendIDRequest
	if !decodeJSONBody(w, req, &input) {
		return
	}
	row, err := r.friendsUsecase(req).CreateFriendRequest(req.Context(), friends.CreateFriendRequestInput{
		AuthToken:  authToken,
		FromUserID: req.Header.Get("X-Nomo-User-ID"),
		ToUserID:   input.ToUserID,
		FriendID:   input.FriendID,
	})
	if err != nil {
		writeFriendsError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, row)
}

func (r *router) updateFriendRequest(w http.ResponseWriter, req *http.Request, authToken string) {
	var input FriendRequestUpdateRequest
	if !decodeJSONBody(w, req, &input) {
		return
	}
	row, err := r.friendsUsecase(req).UpdateFriendRequest(req.Context(), friends.UpdateFriendRequestInput{
		AuthToken: authToken,
		RequestID: req.PathValue("id"),
		UserID:    req.Header.Get("X-Nomo-User-ID"),
		Status:    input.Status,
	})
	if err != nil {
		writeFriendsError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, row)
}

func (r *router) likeDrinkLog(w http.ResponseWriter, req *http.Request, authToken string) {
	state, err := r.drinkLogUsecase(req).LikeDrinkLog(req.Context(), drinklogs.LikeInput{
		AuthToken: authToken,
		LogID:     req.PathValue("id"),
		UserID:    req.Header.Get("X-Nomo-User-ID"),
	})
	if err != nil {
		writeDrinkLogError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, state)
}

func (r *router) unlikeDrinkLog(w http.ResponseWriter, req *http.Request, authToken string) {
	state, err := r.drinkLogUsecase(req).UnlikeDrinkLog(req.Context(), drinklogs.LikeInput{
		AuthToken: authToken,
		LogID:     req.PathValue("id"),
		UserID:    req.Header.Get("X-Nomo-User-ID"),
	})
	if err != nil {
		writeDrinkLogError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, state)
}

func (r *router) reportDrinkLog(w http.ResponseWriter, req *http.Request, authToken string) {
	var input DrinkLogReportRequest
	if !decodeJSONBody(w, req, &input) {
		return
	}
	result, err := r.drinkLogUsecase(req).ReportDrinkLog(req.Context(), drinklogs.ReportInput{
		AuthToken:      authToken,
		LogID:          req.PathValue("id"),
		ReporterUserID: req.Header.Get("X-Nomo-User-ID"),
		Reason:         input.Reason,
	})
	if err != nil {
		writeDrinkLogError(w, err)
		return
	}
	if result.Created {
		writeJSON(w, http.StatusCreated, result.Body)
		return
	}
	writeJSON(w, http.StatusOK, result.Body)
}

func (r *router) listNotifications(w http.ResponseWriter, req *http.Request, authToken string) {
	rows, err := r.notificationUsecase(req).ListNotifications(req.Context(), notifications.ListInput{
		AuthToken: authToken,
		UserID:    req.Header.Get("X-Nomo-User-ID"),
		Date:      dateOnlyParam(req, "date"),
	})
	if err != nil {
		writeNotificationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (r *router) markNotificationsRead(w http.ResponseWriter, req *http.Request, authToken string) {
	updatedCount, err := r.notificationUsecase(req).MarkAllRead(req.Context(), notifications.MarkReadInput{
		AuthToken: authToken,
		UserID:    req.Header.Get("X-Nomo-User-ID"),
	})
	if err != nil {
		writeNotificationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"updated_count": updatedCount})
}

func (r *router) listTodayReservations(w http.ResponseWriter, req *http.Request, authToken string) {
	rows, err := r.drinkInviteUsecase(req).ListTodayReservations(req.Context(), drinkinvites.ListInput{
		AuthToken:  authToken,
		UserID:     req.Header.Get("X-Nomo-User-ID"),
		InviteDate: dateOnlyParam(req, "date"),
	})
	if err != nil {
		writeSupabaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (r *router) listIncomingPendingInvites(w http.ResponseWriter, req *http.Request, authToken string) {
	rows, err := r.drinkInviteUsecase(req).ListIncomingPending(req.Context(), drinkinvites.ListInput{
		AuthToken:  authToken,
		UserID:     req.Header.Get("X-Nomo-User-ID"),
		InviteDate: dateOnlyParam(req, "date"),
	})
	if err != nil {
		writeSupabaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (r *router) listOutgoingActiveInvites(w http.ResponseWriter, req *http.Request, authToken string) {
	rows, err := r.drinkInviteUsecase(req).ListOutgoingActive(req.Context(), drinkinvites.ListInput{
		AuthToken:  authToken,
		UserID:     req.Header.Get("X-Nomo-User-ID"),
		InviteDate: dateOnlyParam(req, "date"),
	})
	if err != nil {
		writeSupabaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (r *router) createDrinkInvite(w http.ResponseWriter, req *http.Request, authToken string) {
	var input DrinkInviteRequest
	if !decodeJSONBody(w, req, &input) {
		return
	}
	row, err := r.drinkInviteUsecase(req).CreateDrinkInvite(req.Context(), drinkinvites.CreateInput{
		AuthToken:  authToken,
		FromUserID: req.Header.Get("X-Nomo-User-ID"),
		ToUserID:   input.ToUserID,
		InviteDate: input.InviteDate,
	})
	if err != nil {
		writeDrinkInviteError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, row)
}

func (r *router) updateDrinkInvite(w http.ResponseWriter, req *http.Request, authToken string) {
	inviteID := req.PathValue("id")
	if _, err := drinkinvites.CleanUUID(inviteID, "drink invite id"); err != nil {
		writeDrinkInviteError(w, err)
		return
	}
	var input DrinkInviteUpdateRequest
	if !decodeJSONBody(w, req, &input) {
		return
	}
	row, err := r.drinkInviteUsecase(req).UpdateDrinkInvite(req.Context(), drinkinvites.UpdateInput{
		AuthToken:       authToken,
		InviteID:        inviteID,
		RecipientUserID: req.Header.Get("X-Nomo-User-ID"),
		Status:          input.Status,
	})
	if err != nil {
		writeDrinkInviteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, row)
}

func (r *router) drinkInviteUsecase(req *http.Request) *drinkinvites.Usecase {
	return drinkinvites.NewUsecase(drinkinvites.Dependencies{
		Repository: drinkinvites.NewSupabaseRepository(r.deps.Supabase),
		Notifier:   drinkInviteNotifier{router: r, req: req},
	})
}

func (r *router) friendsUsecase(req *http.Request) *friends.Usecase {
	return friends.NewUsecase(friends.Dependencies{
		Repository: friends.NewSupabaseRepository(r.deps.Supabase),
		Notifier:   friendRequestNotifier{router: r, req: req},
		Logger:     r.deps.Logger,
	})
}

type friendRequestNotifier struct {
	router *router
	req    *http.Request
}

func (n friendRequestNotifier) FriendRequestReceived(_ context.Context, authToken string, requestRow map[string]any) {
	if n.router == nil || n.req == nil {
		return
	}
	n.router.createFriendRequestReceivedNotification(n.req, authToken, requestRow)
}

func (n friendRequestNotifier) FriendRequestAccepted(_ context.Context, authToken string, requestRow map[string]any) {
	if n.router == nil || n.req == nil {
		return
	}
	n.router.createFriendRequestAcceptedNotification(n.req, authToken, requestRow)
}

type drinkInviteNotifier struct {
	router *router
	req    *http.Request
}

func (n drinkInviteNotifier) DrinkInviteReceived(_ context.Context, authToken string, inviteRow map[string]any) {
	if n.router == nil || n.req == nil {
		return
	}
	n.router.createDrinkInviteReceivedNotification(n.req, authToken, inviteRow)
}

func (n drinkInviteNotifier) DrinkInviteAccepted(_ context.Context, authToken string, inviteRow map[string]any) {
	if n.router == nil || n.req == nil {
		return
	}
	n.router.createDrinkInviteAcceptedNotification(n.req, authToken, inviteRow)
}

func writeDrinkInviteError(w http.ResponseWriter, err error) {
	if kind, ok := drinkinvites.ErrorKindOf(err); ok {
		switch kind {
		case drinkinvites.ErrorKindInvalidInput:
			writeError(w, http.StatusBadRequest, err.Error())
		case drinkinvites.ErrorKindConflict:
			writeError(w, http.StatusConflict, err.Error())
		case drinkinvites.ErrorKindNotFound:
			writeError(w, http.StatusNotFound, err.Error())
		default:
			writeError(w, http.StatusBadRequest, err.Error())
		}
		return
	}
	writeSupabaseError(w, err)
}

func writeFriendsError(w http.ResponseWriter, err error) {
	if kind, ok := friends.ErrorKindOf(err); ok {
		switch kind {
		case friends.ErrorKindInvalidInput:
			writeError(w, http.StatusBadRequest, err.Error())
		case friends.ErrorKindConflict:
			writeError(w, http.StatusConflict, err.Error())
		case friends.ErrorKindNotFound:
			writeError(w, http.StatusNotFound, err.Error())
		default:
			writeError(w, http.StatusBadRequest, err.Error())
		}
		return
	}
	writeSupabaseError(w, err)
}

func (r *router) drinkLogUsecase(req *http.Request) *drinklogs.Usecase {
	return drinklogs.NewUsecase(drinklogs.Dependencies{
		Repository: drinklogs.NewSupabaseRepository(r.deps.Supabase),
		Notifier:   drinkLogNotifier{router: r, req: req},
	})
}

type drinkLogNotifier struct {
	router *router
	req    *http.Request
}

func (n drinkLogNotifier) DrinkLogTagged(_ context.Context, authToken, logID, ownerUserID string, friendIDs []string) {
	if n.router == nil || n.req == nil {
		return
	}
	n.router.createDrinkLogTaggedNotifications(n.req, authToken, logID, ownerUserID, friendIDs)
}

func (n drinkLogNotifier) DrinkLogLiked(_ context.Context, authToken, logID, actorUserID string) {
	if n.router == nil || n.req == nil {
		return
	}
	n.router.createDrinkLogLikeNotification(n.req, authToken, logID, actorUserID)
}

func writeDrinkLogError(w http.ResponseWriter, err error) {
	if kind, ok := drinklogs.ErrorKindOf(err); ok {
		switch kind {
		case drinklogs.ErrorKindInvalidInput:
			writeError(w, http.StatusBadRequest, err.Error())
		case drinklogs.ErrorKindForbidden:
			writeError(w, http.StatusForbidden, err.Error())
		case drinklogs.ErrorKindConflict:
			writeError(w, http.StatusConflict, err.Error())
		case drinklogs.ErrorKindNotFound:
			writeError(w, http.StatusNotFound, err.Error())
		case drinklogs.ErrorKindUpstream:
			writeError(w, http.StatusBadGateway, "upstream service error")
		default:
			writeError(w, http.StatusBadRequest, err.Error())
		}
		return
	}
	writeSupabaseError(w, err)
}

func (input *ProfileSaveRequest) normalize() {
	input.UserID = strings.TrimSpace(input.UserID)
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	input.Gender = normalizeProfileGender(input.Gender)
	input.CharacterKey = strings.TrimSpace(input.CharacterKey)
	input.AvatarURL = strings.TrimSpace(input.AvatarURL)
	if input.CharacterKey == "" {
		input.CharacterKey = "avatar"
	}
}

func (input ProfileSaveRequest) validate() string {
	if !adminUserIDPattern.MatchString(input.UserID) {
		return "user_id must be 3-24 letters, numbers, or underscores"
	}
	nameLength := utf8.RuneCountInString(input.DisplayName)
	if nameLength < 1 || nameLength > 40 {
		return "display_name must be 1-40 characters"
	}
	if !isValidProfileGender(input.Gender) {
		return "gender must be male, female, or unspecified"
	}
	return ""
}

func (input ProfileSaveRequest) profilePayload(authUserID string) map[string]any {
	return map[string]any{
		"id":            authUserID,
		"user_id":       input.UserID,
		"display_name":  input.DisplayName,
		"gender":        input.Gender,
		"character_key": input.CharacterKey,
		"avatar_url":    input.AvatarURL,
		"updated_at":    time.Now().UTC().Format(time.RFC3339),
	}
}

func validateProfilePayload(_ *http.Request, _ string, payload map[string]any) string {
	if raw, ok := payload["user_id"]; ok {
		userID, ok := raw.(string)
		if !ok {
			return "user_id must be a string"
		}
		userID = strings.TrimSpace(userID)
		if !adminUserIDPattern.MatchString(userID) {
			return "user_id must be 3-24 letters, numbers, or underscores"
		}
		payload["user_id"] = userID
	}
	if raw, ok := payload["display_name"]; ok {
		displayName, ok := raw.(string)
		if !ok {
			return "display_name must be a string"
		}
		displayName = strings.TrimSpace(displayName)
		nameLength := utf8.RuneCountInString(displayName)
		if nameLength < 1 || nameLength > 40 {
			return "display_name must be 1-40 characters"
		}
		payload["display_name"] = displayName
	}
	if raw, ok := payload["gender"]; ok {
		gender, ok := raw.(string)
		if !ok {
			return "gender must be a string"
		}
		gender = normalizeProfileGender(gender)
		if !isValidProfileGender(gender) {
			return "gender must be male, female, or unspecified"
		}
		payload["gender"] = gender
	}
	if raw, ok := payload["character_key"]; ok {
		value, ok := raw.(string)
		if !ok {
			return "character_key must be a string"
		}
		payload["character_key"] = strings.TrimSpace(value)
	}
	if raw, ok := payload["avatar_url"]; ok {
		value, ok := raw.(string)
		if !ok && raw != nil {
			return "avatar_url must be a string"
		}
		if ok {
			value = strings.TrimSpace(value)
			if len(value) > 4096 {
				return "avatar_url is too long"
			}
			payload["avatar_url"] = value
		}
	}
	payload["updated_at"] = time.Now().UTC().Format(time.RFC3339)
	return ""
}

func normalizeProfileGender(value string) string {
	gender := strings.ToLower(strings.TrimSpace(value))
	if gender == "" {
		return "unspecified"
	}
	return gender
}

func isValidProfileGender(value string) bool {
	switch normalizeProfileGender(value) {
	case "unspecified", "male", "female":
		return true
	default:
		return false
	}
}

func dateOnlyParam(req *http.Request, name string) string {
	value, errMessage := cleanDateOnlyOrToday(req.URL.Query().Get(name), name)
	if errMessage != "" {
		return time.Now().Format(time.DateOnly)
	}
	return value
}

func isValidDailyStatus(status string) bool {
	switch status {
	case "unselected",
		"can_drink_today",
		"non_alcohol",
		"liver_rest",
		"has_plans":
		return true
	default:
		return false
	}
}
