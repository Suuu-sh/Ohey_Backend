package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/yota/nomo/backend/internal/supabase"
)

const (
	notificationKindDrinkLogLike             = "drink_log_like"
	notificationKindFriendRequestReceived    = "friend_request_received"
	notificationKindFriendRequestAccepted    = "friend_request_accepted"
	notificationKindDrinkInviteReceived      = "drink_invite_received"
	notificationKindDrinkInviteAccepted      = "drink_invite_accepted"
	notificationKindTodayReservationReminder = "today_reservation_reminder"
	notificationKindDrinkLogTagged           = "drink_log_tagged"
	notificationKindSystem                   = "system"
)

func (r *router) insertNotification(req *http.Request, payload map[string]any) (bool, error) {
	if r.deps.AdminSupabase == nil || r.deps.Config.SupabaseServiceRoleKey == "" {
		return false, errors.New("admin supabase client is not configured")
	}
	var rows []map[string]any
	if err := r.deps.AdminSupabase.Post(req.Context(), r.deps.Config.SupabaseServiceRoleKey, "notifications", nil, payload, &rows); err != nil {
		var apiErr supabase.APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusConflict {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (r *router) tryInsertNotification(req *http.Request, payload map[string]any, event string) {
	if _, err := r.insertNotification(req, payload); err != nil {
		r.logNotificationWarning("failed to create notification", event, err)
	}
}

func (r *router) logNotificationWarning(message, event string, err error) {
	if r.deps.Logger == nil {
		return
	}
	r.deps.Logger.Warn(message, "event", event, "error", err)
}

func (r *router) createFriendRequestReceivedNotification(req *http.Request, authToken string, requestRow map[string]any) {
	requestID, _ := requestRow["id"].(string)
	fromUserID, _ := requestRow["from_user_id"].(string)
	toUserID, _ := requestRow["to_user_id"].(string)
	if requestID == "" || fromUserID == "" || toUserID == "" || fromUserID == toUserID {
		return
	}
	actorName := r.displayNameForNotification(req, authToken, fromUserID)
	r.tryInsertNotification(req, map[string]any{
		"recipient_user_id": toUserID,
		"actor_user_id":     fromUserID,
		"friend_request_id": requestID,
		"kind":              notificationKindFriendRequestReceived,
		"title":             "フレンド申請が届きました",
		"message":           actorName + "さんからフレンド申請が届きました。",
	}, notificationKindFriendRequestReceived)
}

func (r *router) createFriendRequestAcceptedNotification(req *http.Request, authToken string, requestRow map[string]any) {
	requestID, _ := requestRow["id"].(string)
	fromUserID, _ := requestRow["from_user_id"].(string)
	toUserID, _ := requestRow["to_user_id"].(string)
	if requestID == "" || fromUserID == "" || toUserID == "" || fromUserID == toUserID {
		return
	}
	actorName := r.displayNameForNotification(req, authToken, toUserID)
	r.tryInsertNotification(req, map[string]any{
		"recipient_user_id": fromUserID,
		"actor_user_id":     toUserID,
		"friend_request_id": requestID,
		"kind":              notificationKindFriendRequestAccepted,
		"title":             "フレンド申請が承認されました",
		"message":           actorName + "さんとフレンドになりました。",
	}, notificationKindFriendRequestAccepted)
}

func (r *router) createDrinkInviteReceivedNotification(req *http.Request, authToken string, inviteRow map[string]any) {
	inviteID, _ := inviteRow["id"].(string)
	fromUserID, _ := inviteRow["from_user_id"].(string)
	toUserID, _ := inviteRow["to_user_id"].(string)
	inviteDate, _ := inviteRow["invite_date"].(string)
	if inviteID == "" || fromUserID == "" || toUserID == "" || fromUserID == toUserID {
		return
	}
	actorName := r.displayNameForNotification(req, authToken, fromUserID)
	r.tryInsertNotification(req, map[string]any{
		"recipient_user_id": toUserID,
		"actor_user_id":     fromUserID,
		"drink_invite_id":   inviteID,
		"notification_date": dateOrNil(inviteDate),
		"kind":              notificationKindDrinkInviteReceived,
		"title":             "飲み誘いが届きました",
		"message":           actorName + "さんから" + inviteDatePhrase(inviteDate) + "の飲み誘いが届きました。",
	}, notificationKindDrinkInviteReceived)
}

func (r *router) createDrinkInviteAcceptedNotification(req *http.Request, authToken string, inviteRow map[string]any) {
	inviteID, _ := inviteRow["id"].(string)
	fromUserID, _ := inviteRow["from_user_id"].(string)
	toUserID, _ := inviteRow["to_user_id"].(string)
	inviteDate, _ := inviteRow["invite_date"].(string)
	if inviteID == "" || fromUserID == "" || toUserID == "" || fromUserID == toUserID {
		return
	}
	actorName := r.displayNameForNotification(req, authToken, toUserID)
	r.tryInsertNotification(req, map[string]any{
		"recipient_user_id": fromUserID,
		"actor_user_id":     toUserID,
		"drink_invite_id":   inviteID,
		"notification_date": dateOrNil(inviteDate),
		"kind":              notificationKindDrinkInviteAccepted,
		"title":             "飲み誘いが承認されました",
		"message":           actorName + "さんが" + inviteDatePhrase(inviteDate) + "の飲み誘いを承認しました。",
	}, notificationKindDrinkInviteAccepted)
}

func (r *router) createDrinkLogTaggedNotifications(req *http.Request, authToken, logID, ownerUserID string, friendIDs []string) {
	if logID == "" || ownerUserID == "" || len(friendIDs) == 0 {
		return
	}
	actorName := r.displayNameForNotification(req, authToken, ownerUserID)
	seen := map[string]bool{}
	for _, rawID := range friendIDs {
		friendID := strings.TrimSpace(rawID)
		if friendID == "" || friendID == ownerUserID || seen[friendID] {
			continue
		}
		seen[friendID] = true
		r.tryInsertNotification(req, map[string]any{
			"recipient_user_id": friendID,
			"actor_user_id":     ownerUserID,
			"drink_log_id":      logID,
			"kind":              notificationKindDrinkLogTagged,
			"title":             "飲みログに追加されました",
			"message":           actorName + "さんがあなたを一緒に飲んだ人に追加しました。",
		}, notificationKindDrinkLogTagged)
	}
}

func (r *router) createTodayReservationReminderNotifications(req *http.Request, authToken, userID string) {
	if userID == "" {
		return
	}
	date := dateOnlyParam(req, "date")
	q := url.Values{}
	q.Set("select", "id,from_user_id,to_user_id,invite_date,status")
	q.Set("invite_date", "eq."+date)
	q.Set("status", "eq.accepted")
	q.Set("or", "(from_user_id.eq."+userID+",to_user_id.eq."+userID+")")
	var invites []map[string]any
	if err := r.deps.Supabase.Get(req.Context(), authToken, "drink_invites", q, &invites); err != nil {
		r.logNotificationWarning("failed to fetch today reservations for notification", notificationKindTodayReservationReminder, err)
		return
	}
	for _, invite := range invites {
		inviteID, _ := invite["id"].(string)
		fromUserID, _ := invite["from_user_id"].(string)
		toUserID, _ := invite["to_user_id"].(string)
		actorUserID := fromUserID
		if actorUserID == userID {
			actorUserID = toUserID
		}
		if inviteID == "" || actorUserID == "" || actorUserID == userID {
			continue
		}
		actorName := r.displayNameForNotification(req, authToken, actorUserID)
		r.tryInsertNotification(req, map[string]any{
			"recipient_user_id": userID,
			"actor_user_id":     actorUserID,
			"drink_invite_id":   inviteID,
			"notification_date": date,
			"kind":              notificationKindTodayReservationReminder,
			"title":             "今日の飲み予定があります",
			"message":           actorName + "さんとの飲み予定が今日あります。",
		}, notificationKindTodayReservationReminder)
	}
}

func (r *router) adminCreateNotification(w http.ResponseWriter, req *http.Request, _ AuthUser) {
	var input AdminCreateSystemNotificationRequest
	if err := json.NewDecoder(req.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	title := strings.TrimSpace(input.Title)
	message := strings.TrimSpace(input.Message)
	if title == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}
	if message == "" {
		writeError(w, http.StatusBadRequest, "message is required")
		return
	}
	recipientIDs, err := r.notificationRecipientIDs(req, input)
	if err != nil {
		writeSupabaseError(w, err)
		return
	}
	if len(recipientIDs) == 0 {
		writeError(w, http.StatusBadRequest, "recipient_user_ids or send_to_all is required")
		return
	}

	createdCount := 0
	for _, recipientID := range recipientIDs {
		payload := map[string]any{
			"recipient_user_id": recipientID,
			"kind":              notificationKindSystem,
			"title":             title,
			"message":           message,
		}
		if systemKey := strings.TrimSpace(input.SystemKey); systemKey != "" {
			payload["system_key"] = systemKey
		}
		created, err := r.insertNotification(req, payload)
		if err != nil {
			writeSupabaseError(w, err)
			return
		}
		if created {
			createdCount++
		}
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"recipient_count": len(recipientIDs),
		"created_count":   createdCount,
	})
}

func (r *router) notificationRecipientIDs(req *http.Request, input AdminCreateSystemNotificationRequest) ([]string, error) {
	seen := map[string]bool{}
	ids := make([]string, 0, len(input.RecipientUserIDs))
	add := func(id string) {
		trimmed := strings.TrimSpace(id)
		if trimmed != "" && !seen[trimmed] {
			seen[trimmed] = true
			ids = append(ids, trimmed)
		}
	}
	for _, id := range input.RecipientUserIDs {
		add(id)
	}
	if !input.SendToAll {
		return ids, nil
	}
	q := url.Values{}
	q.Set("select", "id")
	q.Set("order", "created_at.desc")
	q.Set("limit", "10000")
	var profiles []map[string]any
	if err := r.deps.AdminSupabase.Get(req.Context(), r.deps.Config.SupabaseServiceRoleKey, "profiles", q, &profiles); err != nil {
		return nil, err
	}
	for _, profile := range profiles {
		id, _ := profile["id"].(string)
		add(id)
	}
	return ids, nil
}

func dateOrNil(value string) any {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return trimmed
}

func inviteDatePhrase(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "今日"
	}
	if trimmed == time.Now().Format(time.DateOnly) {
		return "今日"
	}
	parsed, err := time.Parse(time.DateOnly, trimmed)
	if err != nil {
		return trimmed
	}
	return parsed.Format("1/2")
}
