package drinkinvites

import (
	"errors"
	"regexp"
	"strings"
	"time"
)

type InviteStatus string

const (
	InviteStatusPending  InviteStatus = "pending"
	InviteStatusAccepted InviteStatus = "accepted"
	InviteStatusRejected InviteStatus = "rejected"
)

type ErrorKind string

const (
	ErrorKindInvalidInput ErrorKind = "invalid_input"
	ErrorKindConflict     ErrorKind = "conflict"
	ErrorKindNotFound     ErrorKind = "not_found"
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

func CleanDateOnlyOrToday(value, field string, now time.Time) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		if now.IsZero() {
			now = time.Now()
		}
		return now.Format(time.DateOnly), nil
	}
	parsed, err := time.Parse(time.DateOnly, trimmed)
	if err != nil {
		return "", UserError{Kind: ErrorKindInvalidInput, Message: field + " must be YYYY-MM-DD"}
	}
	return parsed.Format(time.DateOnly), nil
}

func ValidateNewInvite(fromUserID, toUserID string) error {
	if fromUserID == toUserID {
		return UserError{Kind: ErrorKindInvalidInput, Message: "cannot invite yourself"}
	}
	return nil
}

func NormalizeResponseStatus(value string) (InviteStatus, error) {
	status := InviteStatus(strings.TrimSpace(value))
	switch status {
	case InviteStatusAccepted, InviteStatusRejected:
		return status, nil
	default:
		return "", UserError{Kind: ErrorKindInvalidInput, Message: "status must be accepted or rejected"}
	}
}

func BlockedDailyStatusMessage(status string) string {
	switch strings.TrimSpace(status) {
	case "liver_rest":
		return "相手が休肝日のため今日は誘えません。"
	case "has_plans":
		return "相手に予定があるため今日は誘えません。"
	default:
		return ""
	}
}

func ExistingInviteConflictMessage(status InviteStatus) string {
	if status == InviteStatusAccepted {
		return "今日はもう予約済みです。"
	}
	return "すでに招待中です。"
}

type DomainEventKind string

const (
	EventDrinkInviteCreated  DomainEventKind = "drink_invite.created"
	EventDrinkInviteAccepted DomainEventKind = "drink_invite.accepted"
)

type DrinkInvite struct {
	ID         string
	FromUserID string
	ToUserID   string
	InviteDate string
	Status     InviteStatus
}

type DomainEvent struct {
	Kind   DomainEventKind
	Invite DrinkInvite
}

func DrinkInviteFromRow(row map[string]any) DrinkInvite {
	id, _ := row["id"].(string)
	fromUserID, _ := row["from_user_id"].(string)
	toUserID, _ := row["to_user_id"].(string)
	inviteDate, _ := row["invite_date"].(string)
	status, _ := row["status"].(string)
	return DrinkInvite{ID: id, FromUserID: fromUserID, ToUserID: toUserID, InviteDate: inviteDate, Status: InviteStatus(status)}
}

func NewDrinkInviteCreatedEvent(row map[string]any) (DomainEvent, bool) {
	invite := DrinkInviteFromRow(row)
	if invite.ID == "" || invite.FromUserID == "" || invite.ToUserID == "" || invite.FromUserID == invite.ToUserID {
		return DomainEvent{}, false
	}
	return DomainEvent{Kind: EventDrinkInviteCreated, Invite: invite}, true
}

func NewDrinkInviteAcceptedEvent(row map[string]any) (DomainEvent, bool) {
	invite := DrinkInviteFromRow(row)
	if invite.ID == "" || invite.FromUserID == "" || invite.ToUserID == "" || invite.FromUserID == invite.ToUserID {
		return DomainEvent{}, false
	}
	invite.Status = InviteStatusAccepted
	return DomainEvent{Kind: EventDrinkInviteAccepted, Invite: invite}, true
}

func (e DomainEvent) InviteRow() map[string]any {
	return map[string]any{
		"id":           e.Invite.ID,
		"from_user_id": e.Invite.FromUserID,
		"to_user_id":   e.Invite.ToUserID,
		"invite_date":  e.Invite.InviteDate,
		"status":       string(e.Invite.Status),
	}
}

type NewInvite struct {
	FromUserID string
	ToUserID   string
	InviteDate string
}

type ExistingInvite struct {
	ID     string
	Status InviteStatus
}
