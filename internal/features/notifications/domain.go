package notifications

import (
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/yota/ohey/backend/internal/contracts"
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
	KindFriendRequestReceived    Kind = contracts.NotificationKindFriendRequestReceived
	KindFriendRequestAccepted    Kind = contracts.NotificationKindFriendRequestAccepted
	KindInviteReceived           Kind = contracts.NotificationKindInviteReceived
	KindInviteAccepted           Kind = contracts.NotificationKindInviteAccepted
	KindTodayReservationReminder Kind = contracts.NotificationKindTodayReservationReminder
	KindMemoryTagged             Kind = contracts.NotificationKindMemoryTagged
	KindYuruboCreated            Kind = contracts.NotificationKindYuruboCreated
	KindSystem                   Kind = contracts.NotificationKindSystem
)

type Notification struct {
	RecipientUserID  string
	ActorUserID      string
	MemoryID         string
	FriendRequestID  string
	InviteID         string
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
	if n.MemoryID != "" {
		payload["memory_id"] = n.MemoryID
	}
	if n.FriendRequestID != "" {
		payload["friend_request_id"] = n.FriendRequestID
	}
	if n.InviteID != "" {
		payload["invite_id"] = n.InviteID
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
	if n.MemoryID != "" {
		data["memory_id"] = n.MemoryID
	}
	if n.FriendRequestID != "" {
		data["friend_request_id"] = n.FriendRequestID
	}
	if n.InviteID != "" {
		data["invite_id"] = n.InviteID
	}
	return data
}

type Invite struct {
	ID            string
	InviterUserID string
	InviteeUserID string
	ScheduledDate string
	ActivityLabel string
	Status        string
}

func InviteFromRow(row map[string]any) Invite {
	id, _ := row["id"].(string)
	inviterUserID, _ := row["inviter_user_id"].(string)
	inviteeUserID, _ := row["invitee_user_id"].(string)
	scheduledDate, _ := row["scheduled_date"].(string)
	activityLabel, _ := row["activity_label"].(string)
	status, _ := row["status"].(string)
	return Invite{ID: id, InviterUserID: inviterUserID, InviteeUserID: inviteeUserID, ScheduledDate: scheduledDate, ActivityLabel: activityLabel, Status: status}
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

func ScheduledDatePhrase(value string, now time.Time) string {
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

func InvitePlanPhrase(invite Invite, now time.Time) string {
	date := ScheduledDatePhrase(invite.ScheduledDate, now)
	activity := ShortText(invite.ActivityLabel, 40)
	if activity == "" {
		return date + "のお誘い"
	}
	return date + "に「" + activity + "」"
}

func ShortText(value string, limit int) string {
	trimmed := strings.TrimSpace(value)
	if limit <= 0 || utf8.RuneCountInString(trimmed) <= limit {
		return trimmed
	}
	runes := []rune(trimmed)
	return string(runes[:limit])
}
