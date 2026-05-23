package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/yota/nomo/backend/internal/supabase"
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
	var errMessage string
	friendID, errMessage = cleanUUID(friendID, "friend_id")
	if errMessage != "" {
		writeError(w, http.StatusBadRequest, errMessage)
		return
	}
	userID := req.Header.Get("X-Nomo-User-ID")
	if friendID == userID {
		writeError(w, http.StatusBadRequest, "cannot add yourself as a friend")
		return
	}
	userA, userB := orderedPair(userID, friendID)
	payload := map[string]any{"user_a_id": userA, "user_b_id": userB}
	q := url.Values{}
	q.Set("on_conflict", "user_a_id,user_b_id")
	var rows []map[string]any
	if err := r.deps.Supabase.Upsert(req.Context(), authToken, "friendships", q, payload, &rows); err != nil {
		writeSupabaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, firstMap(rows, payload))
}

func (r *router) getFriendRequestStatus(w http.ResponseWriter, req *http.Request, authToken string) {
	friendID, errMessage := cleanUUID(req.URL.Query().Get("friend_id"), "friend_id")
	if errMessage != "" {
		writeError(w, http.StatusBadRequest, errMessage)
		return
	}
	userID := req.Header.Get("X-Nomo-User-ID")
	if friendID == userID {
		writeJSON(w, http.StatusOK, map[string]any{"already_friend": false, "request_state": "self"})
		return
	}

	alreadyFriend, err := r.friendshipExists(req, authToken, userID, friendID)
	if err != nil {
		writeSupabaseError(w, err)
		return
	}
	requestState := "none"
	if !alreadyFriend {
		q := url.Values{}
		q.Set("select", "id,from_user_id,to_user_id")
		q.Set("status", "eq.pending")
		q.Set("or", "(and(from_user_id.eq."+userID+",to_user_id.eq."+friendID+"),and(from_user_id.eq."+friendID+",to_user_id.eq."+userID+"))")
		q.Set("limit", "1")
		var requests []map[string]any
		if err := r.deps.Supabase.Get(req.Context(), authToken, "friend_requests", q, &requests); err != nil {
			writeSupabaseError(w, err)
			return
		}
		if len(requests) > 0 {
			if requests[0]["from_user_id"] == userID {
				requestState = "outgoing"
			} else {
				requestState = "incoming"
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"already_friend": alreadyFriend,
		"request_state":  requestState,
	})
}

func (r *router) createFriendRequest(w http.ResponseWriter, req *http.Request, authToken string) {
	var input FriendIDRequest
	if !decodeJSONBody(w, req, &input) {
		return
	}
	toUserID := strings.TrimSpace(input.ToUserID)
	if toUserID == "" {
		toUserID = strings.TrimSpace(input.FriendID)
	}
	var errMessage string
	toUserID, errMessage = cleanUUID(toUserID, "to_user_id")
	if errMessage != "" {
		writeError(w, http.StatusBadRequest, errMessage)
		return
	}
	fromUserID := req.Header.Get("X-Nomo-User-ID")
	if toUserID == fromUserID {
		writeError(w, http.StatusBadRequest, "cannot send a friend request to yourself")
		return
	}
	alreadyFriend, err := r.friendshipExists(req, authToken, fromUserID, toUserID)
	if err != nil {
		writeSupabaseError(w, err)
		return
	}
	if alreadyFriend {
		writeError(w, http.StatusConflict, "already friends")
		return
	}
	payload := map[string]any{
		"from_user_id": fromUserID,
		"to_user_id":   toUserID,
		"status":       "pending",
	}
	var rows []map[string]any
	if err := r.deps.Supabase.Post(req.Context(), authToken, "friend_requests", nil, payload, &rows); err != nil {
		writeSupabaseError(w, err)
		return
	}
	row := firstMap(rows, payload)
	r.createFriendRequestReceivedNotification(req, authToken, row)
	writeJSON(w, http.StatusCreated, row)
}

func (r *router) updateFriendRequest(w http.ResponseWriter, req *http.Request, authToken string) {
	requestID, errMessage := cleanUUID(req.PathValue("id"), "friend request id")
	if errMessage != "" {
		writeError(w, http.StatusBadRequest, errMessage)
		return
	}
	var input FriendRequestUpdateRequest
	if !decodeJSONBody(w, req, &input) {
		return
	}
	status := strings.TrimSpace(input.Status)
	if status != "accepted" && status != "rejected" && status != "cancelled" {
		writeError(w, http.StatusBadRequest, "status must be accepted, rejected, or cancelled")
		return
	}

	userID := req.Header.Get("X-Nomo-User-ID")
	q := url.Values{}
	q.Set("id", "eq."+requestID)
	q.Set("status", "eq.pending")
	if status == "cancelled" {
		q.Set("from_user_id", "eq."+userID)
	} else {
		q.Set("to_user_id", "eq."+userID)
	}
	payload := map[string]any{
		"status":       status,
		"responded_at": time.Now().UTC().Format(time.RFC3339),
	}
	var rows []map[string]any
	if err := r.deps.Supabase.Patch(req.Context(), authToken, "friend_requests", q, payload, &rows); err != nil {
		writeSupabaseError(w, err)
		return
	}
	if len(rows) == 0 {
		writeError(w, http.StatusNotFound, "friend request not found")
		return
	}
	if status == "accepted" {
		from, _ := rows[0]["from_user_id"].(string)
		to, _ := rows[0]["to_user_id"].(string)
		if from != "" && to != "" {
			if err := r.upsertFriendshipPair(req, authToken, from, to); err != nil {
				writeSupabaseError(w, err)
				return
			}
			r.createFriendRequestAcceptedNotification(req, authToken, rows[0])
		}
	}
	writeJSON(w, http.StatusOK, rows[0])
}

func (r *router) likeDrinkLog(w http.ResponseWriter, req *http.Request, authToken string) {
	logID, errMessage := cleanUUID(req.PathValue("id"), "drink log id")
	userID := req.Header.Get("X-Nomo-User-ID")
	if errMessage != "" {
		writeError(w, http.StatusBadRequest, errMessage)
		return
	}
	payload := map[string]any{"drink_log_id": logID, "user_id": userID}
	var ignored []map[string]any
	created := true
	if err := r.deps.Supabase.Post(req.Context(), authToken, "drink_log_likes", nil, payload, &ignored); err != nil {
		var apiErr supabase.APIError
		if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusConflict {
			writeSupabaseError(w, err)
			return
		}
		created = false
	}
	if created {
		r.createDrinkLogLikeNotification(req, authToken, logID, userID)
	}
	r.writeDrinkLogLikeState(w, req, authToken, logID, userID)
}

func (r *router) unlikeDrinkLog(w http.ResponseWriter, req *http.Request, authToken string) {
	logID, errMessage := cleanUUID(req.PathValue("id"), "drink log id")
	userID := req.Header.Get("X-Nomo-User-ID")
	if errMessage != "" {
		writeError(w, http.StatusBadRequest, errMessage)
		return
	}
	q := url.Values{}
	q.Set("drink_log_id", "eq."+logID)
	q.Set("user_id", "eq."+userID)
	var ignored []map[string]any
	if err := r.deps.Supabase.Delete(req.Context(), authToken, "drink_log_likes", q, &ignored); err != nil {
		writeSupabaseError(w, err)
		return
	}
	r.writeDrinkLogLikeState(w, req, authToken, logID, userID)
}

func (r *router) reportDrinkLog(w http.ResponseWriter, req *http.Request, authToken string) {
	logID, errMessage := cleanUUID(req.PathValue("id"), "drink log id")
	userID := req.Header.Get("X-Nomo-User-ID")
	if errMessage != "" {
		writeError(w, http.StatusBadRequest, errMessage)
		return
	}
	var input DrinkLogReportRequest
	if !decodeJSONBody(w, req, &input) {
		return
	}
	reason := strings.TrimSpace(input.Reason)
	if reason == "" {
		reason = "other"
	}

	q := url.Values{}
	q.Set("select", "id")
	q.Set("drink_log_id", "eq."+logID)
	q.Set("reporter_user_id", "eq."+userID)
	q.Set("limit", "1")
	var existing []map[string]any
	if err := r.deps.Supabase.Get(req.Context(), authToken, "drink_log_reports", q, &existing); err != nil {
		writeSupabaseError(w, err)
		return
	}
	if len(existing) > 0 {
		writeJSON(w, http.StatusOK, map[string]any{"reported": true})
		return
	}

	payload := map[string]any{"drink_log_id": logID, "reporter_user_id": userID, "reason": reason}
	var rows []map[string]any
	if err := r.deps.Supabase.Post(req.Context(), authToken, "drink_log_reports", nil, payload, &rows); err != nil {
		var apiErr supabase.APIError
		if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusConflict {
			writeSupabaseError(w, err)
			return
		}
	}
	writeJSON(w, http.StatusCreated, map[string]any{"reported": true})
}

func (r *router) listNotifications(w http.ResponseWriter, req *http.Request, authToken string) {
	userID := req.Header.Get("X-Nomo-User-ID")
	r.createTodayReservationReminderNotifications(req, authToken, userID)

	q := url.Values{}
	q.Set("select", "id,kind,title,message,created_at,read_at,actor_user_id,drink_log_id,friend_request_id,drink_invite_id,notification_date,system_key,actor:profiles!notifications_actor_user_id_fkey(id,user_id,display_name,avatar_url),friend_request:friend_requests!notifications_friend_request_id_fkey(id,status),drink_invite:drink_invites!notifications_drink_invite_id_fkey(id,status)")
	q.Set("recipient_user_id", "eq."+userID)
	q.Set("order", "created_at.desc")
	q.Set("limit", "50")
	var rows []map[string]any
	if err := r.deps.Supabase.Get(req.Context(), authToken, "notifications", q, &rows); err != nil {
		writeSupabaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (r *router) markNotificationsRead(w http.ResponseWriter, req *http.Request, authToken string) {
	userID := req.Header.Get("X-Nomo-User-ID")
	now := time.Now().UTC().Format(time.RFC3339)
	q := url.Values{}
	q.Set("recipient_user_id", "eq."+userID)
	q.Set("read_at", "is.null")
	var rows []map[string]any
	if err := r.deps.Supabase.Patch(req.Context(), authToken, "notifications", q, map[string]any{"read_at": now}, &rows); err != nil {
		writeSupabaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"updated_count": len(rows)})
}

func (r *router) createDrinkLogLikeNotification(req *http.Request, authToken, logID, actorUserID string) {
	q := url.Values{}
	q.Set("select", "id,owner_user_id")
	q.Set("id", "eq."+logID)
	q.Set("limit", "1")
	var logs []map[string]any
	if err := r.deps.Supabase.Get(req.Context(), authToken, "drink_logs", q, &logs); err != nil {
		r.deps.Logger.Warn("failed to fetch drink log for like notification", "error", err, "drink_log_id", logID)
		return
	}
	if len(logs) == 0 {
		return
	}
	recipientUserID, _ := logs[0]["owner_user_id"].(string)
	if recipientUserID == "" || recipientUserID == actorUserID {
		return
	}

	actorName := r.displayNameForNotification(req, authToken, actorUserID)
	notification := map[string]any{
		"recipient_user_id": recipientUserID,
		"actor_user_id":     actorUserID,
		"drink_log_id":      logID,
		"kind":              notificationKindDrinkLogLike,
		"title":             "いいねされました",
		"message":           actorName + "さんがあなたの飲みログにいいねしました。",
	}
	r.tryInsertNotification(req, notification, notificationKindDrinkLogLike)
}

func (r *router) displayNameForNotification(req *http.Request, authToken, userID string) string {
	q := url.Values{}
	q.Set("select", "display_name,user_id")
	q.Set("id", "eq."+userID)
	q.Set("limit", "1")
	var profiles []map[string]any
	if r.deps.AdminSupabase != nil && r.deps.Config.SupabaseServiceRoleKey != "" {
		if err := r.deps.AdminSupabase.Get(req.Context(), r.deps.Config.SupabaseServiceRoleKey, "profiles", q, &profiles); err == nil && len(profiles) > 0 {
			if name, ok := profiles[0]["display_name"].(string); ok && strings.TrimSpace(name) != "" {
				return strings.TrimSpace(name)
			}
			if userName, ok := profiles[0]["user_id"].(string); ok && strings.TrimSpace(userName) != "" {
				return strings.TrimSpace(userName)
			}
		}
	}
	if err := r.deps.Supabase.Get(req.Context(), authToken, "profiles", q, &profiles); err != nil || len(profiles) == 0 {
		return "フレンズ"
	}
	if name, ok := profiles[0]["display_name"].(string); ok && strings.TrimSpace(name) != "" {
		return strings.TrimSpace(name)
	}
	if userName, ok := profiles[0]["user_id"].(string); ok && strings.TrimSpace(userName) != "" {
		return strings.TrimSpace(userName)
	}
	return "フレンズ"
}

func (r *router) listTodayReservations(w http.ResponseWriter, req *http.Request, authToken string) {
	userID := req.Header.Get("X-Nomo-User-ID")
	date := dateOnlyParam(req, "date")
	q := url.Values{}
	q.Set("select", drinkInviteSelect)
	q.Set("invite_date", "eq."+date)
	q.Set("status", "eq.accepted")
	q.Set("or", "(from_user_id.eq."+userID+",to_user_id.eq."+userID+")")
	q.Set("order", "responded_at.desc")
	var rows []map[string]any
	if err := r.deps.Supabase.Get(req.Context(), authToken, "drink_invites", q, &rows); err != nil {
		writeSupabaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (r *router) listIncomingPendingInvites(w http.ResponseWriter, req *http.Request, authToken string) {
	userID := req.Header.Get("X-Nomo-User-ID")
	date := dateOnlyParam(req, "date")
	q := url.Values{}
	q.Set("select", drinkInviteSelect)
	q.Set("invite_date", "eq."+date)
	q.Set("to_user_id", "eq."+userID)
	q.Set("status", "eq.pending")
	q.Set("order", "created_at.desc")
	var rows []map[string]any
	if err := r.deps.Supabase.Get(req.Context(), authToken, "drink_invites", q, &rows); err != nil {
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
	fromUserID := req.Header.Get("X-Nomo-User-ID")
	toUserID, errMessage := cleanUUID(input.ToUserID, "to_user_id")
	if errMessage != "" {
		writeError(w, http.StatusBadRequest, errMessage)
		return
	}
	if toUserID == fromUserID {
		writeError(w, http.StatusBadRequest, "cannot invite yourself")
		return
	}
	inviteDate, errMessage := cleanDateOnlyOrToday(input.InviteDate, "invite_date")
	if errMessage != "" {
		writeError(w, http.StatusBadRequest, errMessage)
		return
	}
	if blockedMessage, err := r.blockedInviteStatusMessage(req.Context(), authToken, toUserID, inviteDate); err != nil {
		writeSupabaseError(w, err)
		return
	} else if blockedMessage != "" {
		writeError(w, http.StatusConflict, blockedMessage)
		return
	}

	q := url.Values{}
	q.Set("select", "id,status")
	q.Set("invite_date", "eq."+inviteDate)
	q.Set("or", "(and(from_user_id.eq."+fromUserID+",to_user_id.eq."+toUserID+"),and(from_user_id.eq."+toUserID+",to_user_id.eq."+fromUserID+"))")
	q.Set("status", "in.(pending,accepted)")
	q.Set("limit", "1")
	var existing []map[string]any
	if err := r.deps.Supabase.Get(req.Context(), authToken, "drink_invites", q, &existing); err != nil {
		writeSupabaseError(w, err)
		return
	}
	if len(existing) > 0 {
		if existing[0]["status"] == "accepted" {
			writeError(w, http.StatusConflict, "今日はもう予約済みです。")
		} else {
			writeError(w, http.StatusConflict, "すでに招待中です。")
		}
		return
	}

	payload := map[string]any{
		"from_user_id": fromUserID,
		"to_user_id":   toUserID,
		"invite_date":  inviteDate,
		"status":       "pending",
	}
	var rows []map[string]any
	if err := r.deps.Supabase.Post(req.Context(), authToken, "drink_invites", nil, payload, &rows); err != nil {
		writeSupabaseError(w, err)
		return
	}
	row := firstMap(rows, payload)
	r.createDrinkInviteReceivedNotification(req, authToken, row)
	writeJSON(w, http.StatusCreated, row)
}

func (r *router) blockedInviteStatusMessage(ctx context.Context, authToken, userID, inviteDate string) (string, error) {
	q := url.Values{}
	q.Set("select", "status")
	q.Set("user_id", "eq."+userID)
	q.Set("status_date", "eq."+inviteDate)
	q.Set("limit", "1")
	var rows []map[string]any
	if err := r.deps.Supabase.Get(ctx, authToken, "daily_statuses", q, &rows); err != nil {
		return "", err
	}
	if len(rows) == 0 {
		return "", nil
	}
	status, _ := rows[0]["status"].(string)
	switch strings.TrimSpace(status) {
	case "liver_rest":
		return "相手が休肝日のため今日は誘えません。", nil
	case "has_plans":
		return "相手に予定があるため今日は誘えません。", nil
	default:
		return "", nil
	}
}

func (r *router) updateDrinkInvite(w http.ResponseWriter, req *http.Request, authToken string) {
	inviteID, errMessage := cleanUUID(req.PathValue("id"), "drink invite id")
	if errMessage != "" {
		writeError(w, http.StatusBadRequest, errMessage)
		return
	}
	var input DrinkInviteUpdateRequest
	if !decodeJSONBody(w, req, &input) {
		return
	}
	status := strings.TrimSpace(input.Status)
	if status != "accepted" && status != "rejected" {
		writeError(w, http.StatusBadRequest, "status must be accepted or rejected")
		return
	}

	q := url.Values{}
	q.Set("id", "eq."+inviteID)
	q.Set("to_user_id", "eq."+req.Header.Get("X-Nomo-User-ID"))
	q.Set("status", "eq.pending")
	payload := map[string]any{
		"status":       status,
		"responded_at": time.Now().UTC().Format(time.RFC3339),
	}
	var rows []map[string]any
	if err := r.deps.Supabase.Patch(req.Context(), authToken, "drink_invites", q, payload, &rows); err != nil {
		writeSupabaseError(w, err)
		return
	}
	if len(rows) == 0 {
		writeError(w, http.StatusNotFound, "drink invite not found")
		return
	}
	if status == "accepted" {
		r.createDrinkInviteAcceptedNotification(req, authToken, rows[0])
	}
	writeJSON(w, http.StatusOK, rows[0])
}

const drinkInviteSelect = "id,from_user_id,to_user_id,invite_date,status,from_user:profiles!drink_invites_from_user_id_fkey(id,display_name,user_id,gender,avatar_url),to_user:profiles!drink_invites_to_user_id_fkey(id,display_name,user_id,gender,avatar_url)"

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

func (r *router) attachTodayStatuses(req *http.Request, authToken string, rows []map[string]any) error {
	profiles := map[string]map[string]any{}
	for _, row := range rows {
		for _, key := range []string{"user_a", "user_b"} {
			profile, ok := row[key].(map[string]any)
			if !ok {
				continue
			}
			id, _ := profile["id"].(string)
			if id != "" {
				profiles[id] = profile
			}
		}
	}
	if len(profiles) == 0 {
		return nil
	}
	profileIDs := make([]string, 0, len(profiles))
	for id := range profiles {
		profileIDs = append(profileIDs, id)
	}
	sort.Strings(profileIDs)
	q := url.Values{}
	q.Set("select", "user_id,status")
	q.Set("user_id", "in.("+strings.Join(profileIDs, ",")+")")
	q.Set("status_date", "eq."+dateOnlyParam(req, "date"))
	var statuses []map[string]any
	if err := r.deps.Supabase.Get(req.Context(), authToken, "daily_statuses", q, &statuses); err != nil {
		return err
	}
	for _, status := range statuses {
		userID, _ := status["user_id"].(string)
		statusKey, _ := status["status"].(string)
		if profile := profiles[userID]; profile != nil {
			if strings.TrimSpace(statusKey) != "" {
				profile["status_key"] = statusKey
			}
		}
	}

	for _, profile := range profiles {
		if _, hasStatus := profile["status_key"]; hasStatus {
			continue
		}
		if status, ok := profile["status"].(string); ok && strings.TrimSpace(status) != "" {
			profile["status_key"] = status
		}
	}
	return nil
}

func (r *router) attachFriendDrinkStats(req *http.Request, authToken string, rows []map[string]any) error {
	currentUserID := req.Header.Get("X-Nomo-User-ID")
	profiles := map[string]map[string]any{}
	for _, row := range rows {
		for _, key := range []string{"user_a", "user_b"} {
			profile, ok := row[key].(map[string]any)
			if !ok {
				continue
			}
			id, _ := profile["id"].(string)
			if id != "" && id != currentUserID {
				profiles[id] = profile
			}
		}
	}
	if len(profiles) == 0 {
		return nil
	}

	friendIDs := make([]string, 0, len(profiles))
	for id := range profiles {
		friendIDs = append(friendIDs, id)
	}
	sort.Strings(friendIDs)

	stats := make(map[string]*friendDrinkStats, len(friendIDs))
	for _, id := range friendIDs {
		stats[id] = &friendDrinkStats{}
	}

	if err := r.attachOwnedDrinkStats(req, authToken, currentUserID, friendIDs, stats); err != nil {
		return err
	}
	if err := r.attachTaggedDrinkStats(req, authToken, currentUserID, friendIDs, stats); err != nil {
		return err
	}

	for id, profile := range profiles {
		stat := stats[id]
		if stat == nil {
			profile["total_drink_count"] = 0
			continue
		}
		profile["total_drink_count"] = stat.count
		if !stat.lastDrinkAt.IsZero() {
			profile["last_drink_at"] = stat.lastDrinkAt.Format(time.RFC3339)
		}
	}
	return nil
}

type friendDrinkStats struct {
	count       int
	lastDrinkAt time.Time
}

func (s *friendDrinkStats) add(drankAt time.Time) {
	s.count++
	if drankAt.After(s.lastDrinkAt) {
		s.lastDrinkAt = drankAt
	}
}

func (r *router) attachOwnedDrinkStats(req *http.Request, authToken, currentUserID string, friendIDs []string, stats map[string]*friendDrinkStats) error {
	q := url.Values{}
	q.Set("select", "profile_id,drink_logs!inner(owner_user_id,drank_at)")
	q.Set("profile_id", "in.("+strings.Join(friendIDs, ",")+")")
	q.Set("drink_logs.owner_user_id", "eq."+currentUserID)
	var rows []map[string]any
	if err := r.deps.Supabase.Get(req.Context(), authToken, "drink_log_friends", q, &rows); err != nil {
		return err
	}
	for _, row := range rows {
		friendID, _ := row["profile_id"].(string)
		if stat := stats[friendID]; stat != nil {
			stat.add(embeddedDrinkLogTime(row))
		}
	}
	return nil
}

func (r *router) attachTaggedDrinkStats(req *http.Request, authToken, currentUserID string, friendIDs []string, stats map[string]*friendDrinkStats) error {
	q := url.Values{}
	q.Set("select", "profile_id,drink_logs!inner(owner_user_id,drank_at)")
	q.Set("profile_id", "eq."+currentUserID)
	q.Set("drink_logs.owner_user_id", "in.("+strings.Join(friendIDs, ",")+")")
	var rows []map[string]any
	if err := r.deps.Supabase.Get(req.Context(), authToken, "drink_log_friends", q, &rows); err != nil {
		return err
	}
	for _, row := range rows {
		log, ok := row["drink_logs"].(map[string]any)
		if !ok {
			continue
		}
		friendID, _ := log["owner_user_id"].(string)
		if stat := stats[friendID]; stat != nil {
			stat.add(embeddedDrinkLogTime(row))
		}
	}
	return nil
}

func embeddedDrinkLogTime(row map[string]any) time.Time {
	log, ok := row["drink_logs"].(map[string]any)
	if !ok {
		return time.Time{}
	}
	value, _ := log["drank_at"].(string)
	parsed, err := time.Parse(time.RFC3339, value)
	if err == nil {
		return parsed
	}
	return time.Time{}
}

func (r *router) friendshipExists(req *http.Request, authToken, userID, friendID string) (bool, error) {
	q := url.Values{}
	q.Set("select", "id")
	q.Set("or", "(and(user_a_id.eq."+userID+",user_b_id.eq."+friendID+"),and(user_a_id.eq."+friendID+",user_b_id.eq."+userID+"))")
	q.Set("limit", "1")
	var rows []map[string]any
	if err := r.deps.Supabase.Get(req.Context(), authToken, "friendships", q, &rows); err != nil {
		return false, err
	}
	return len(rows) > 0, nil
}

func (r *router) upsertFriendshipPair(req *http.Request, authToken, userA, userB string) error {
	first, second := orderedPair(userA, userB)
	q := url.Values{}
	q.Set("on_conflict", "user_a_id,user_b_id")
	var rows []map[string]any
	return r.deps.Supabase.Upsert(req.Context(), authToken, "friendships", q, map[string]any{
		"user_a_id": first,
		"user_b_id": second,
	}, &rows)
}

func (r *router) writeDrinkLogLikeState(w http.ResponseWriter, req *http.Request, authToken, logID, userID string) {
	q := url.Values{}
	q.Set("select", "user_id")
	q.Set("drink_log_id", "eq."+logID)
	var likes []map[string]any
	if err := r.deps.Supabase.Get(req.Context(), authToken, "drink_log_likes", q, &likes); err != nil {
		writeSupabaseError(w, err)
		return
	}
	likedByMe := false
	for _, like := range likes {
		if like["user_id"] == userID {
			likedByMe = true
			break
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"like_count":  len(likes),
		"liked_by_me": likedByMe,
	})
}

func orderedPair(a, b string) (string, string) {
	if a < b {
		return a, b
	}
	return b, a
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
