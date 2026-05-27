package notifications

import (
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

type ErrorKind string

const (
	ErrorKindInvalidInput ErrorKind = "invalid_input"
	ErrorKindUpstream     ErrorKind = "upstream"
)

type UserError struct {
	Kind    ErrorKind
	Message string
}

func (e UserError) Error() string {
	return e.Message
}

func ErrorKindOf(err error) (ErrorKind, bool) {
	var userErr UserError
	if errors.As(err, &userErr) {
		return userErr.Kind, true
	}
	return "", false
}

var uuidPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func CleanUUID(value, field string) (string, error) {
	trimmed := strings.ToLower(strings.TrimSpace(value))
	if trimmed == "" {
		return "", UserError{Kind: ErrorKindInvalidInput, Message: field + " is required"}
	}
	if !uuidPattern.MatchString(trimmed) {
		return "", UserError{Kind: ErrorKindInvalidInput, Message: field + " must be a valid UUID"}
	}
	return trimmed, nil
}

func CleanUUIDs(values []string, field string) ([]string, error) {
	seen := map[string]bool{}
	ids := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		id, err := CleanUUID(trimmed, field)
		if err != nil {
			return nil, err
		}
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	return ids, nil
}

type Kind string

const (
	KindDrinkLogLike             Kind = "drink_log_like"
	KindFriendRequestReceived    Kind = "friend_request_received"
	KindFriendRequestAccepted    Kind = "friend_request_accepted"
	KindDrinkInviteReceived      Kind = "drink_invite_received"
	KindDrinkInviteAccepted      Kind = "drink_invite_accepted"
	KindTodayReservationReminder Kind = "today_reservation_reminder"
	KindDrinkLogTagged           Kind = "drink_log_tagged"
	KindSystem                   Kind = "system"
)

type Notification struct {
	RecipientUserID  string
	ActorUserID      string
	DrinkLogID       string
	FriendRequestID  string
	DrinkInviteID    string
	NotificationDate string
	Kind             Kind
	Title            string
	Message          string
	SystemKey        string
}

func (n Notification) Payload() map[string]any {
	payload := map[string]any{
		"recipient_user_id": n.RecipientUserID,
		"kind":              string(n.Kind),
		"title":             n.Title,
		"message":           n.Message,
	}
	if n.ActorUserID != "" {
		payload["actor_user_id"] = n.ActorUserID
	}
	if n.DrinkLogID != "" {
		payload["drink_log_id"] = n.DrinkLogID
	}
	if n.FriendRequestID != "" {
		payload["friend_request_id"] = n.FriendRequestID
	}
	if n.DrinkInviteID != "" {
		payload["drink_invite_id"] = n.DrinkInviteID
	}
	if n.NotificationDate != "" {
		payload["notification_date"] = n.NotificationDate
	}
	if n.SystemKey != "" {
		payload["system_key"] = n.SystemKey
	}
	return payload
}

func (n Notification) PushData() map[string]string {
	data := map[string]string{"kind": string(n.Kind)}
	if n.DrinkLogID != "" {
		data["drink_log_id"] = n.DrinkLogID
	}
	if n.FriendRequestID != "" {
		data["friend_request_id"] = n.FriendRequestID
	}
	if n.DrinkInviteID != "" {
		data["drink_invite_id"] = n.DrinkInviteID
	}
	return data
}

type DrinkInvite struct {
	ID         string
	FromUserID string
	ToUserID   string
	InviteDate string
	Status     string
}

func DrinkInviteFromRow(row map[string]any) DrinkInvite {
	id, _ := row["id"].(string)
	fromUserID, _ := row["from_user_id"].(string)
	toUserID, _ := row["to_user_id"].(string)
	inviteDate, _ := row["invite_date"].(string)
	status, _ := row["status"].(string)
	return DrinkInvite{ID: id, FromUserID: fromUserID, ToUserID: toUserID, InviteDate: inviteDate, Status: status}
}

func FriendRequestIDs(row map[string]any) (requestID, fromUserID, toUserID string) {
	requestID, _ = row["id"].(string)
	fromUserID, _ = row["from_user_id"].(string)
	toUserID, _ = row["to_user_id"].(string)
	return requestID, fromUserID, toUserID
}

func DateOrEmpty(value string) string {
	return strings.TrimSpace(value)
}

func InviteDatePhrase(value string, now time.Time) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "今日"
	}
	if trimmed == now.Format(time.DateOnly) {
		return "今日"
	}
	parsed, err := time.Parse(time.DateOnly, trimmed)
	if err != nil {
		return trimmed
	}
	return parsed.Format("1/2")
}

func ShortText(value string, limit int) string {
	trimmed := strings.TrimSpace(value)
	if limit <= 0 || utf8.RuneCountInString(trimmed) <= limit {
		return trimmed
	}
	runes := []rune(trimmed)
	return string(runes[:limit])
}
