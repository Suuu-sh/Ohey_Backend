package invites

import (
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Suuu-sh/Ohey_Backend/internal/contracts"
)

type InviteStatus string

const (
	InviteStatusPending  InviteStatus = contracts.StatusPending
	InviteStatusAccepted InviteStatus = contracts.StatusAccepted
	InviteStatusRejected InviteStatus = contracts.StatusRejected
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

func CleanActivityLabel(value string) (string, error) {
	label := strings.Join(strings.Fields(value), " ")
	if label == "" {
		return "", nil
	}
	if utf8.RuneCountInString(label) > 40 {
		return "", UserError{Kind: ErrorKindInvalidInput, Message: "activity_label must be 40 characters or fewer"}
	}
	return label, nil
}

func ValidateNewInvite(inviterUserID, inviteeUserID string) error {
	if inviterUserID == inviteeUserID {
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
	case contracts.DailyStatusHasPlans:
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
	EventInviteCreated  DomainEventKind = contracts.DomainEventInviteCreated
	EventInviteAccepted DomainEventKind = contracts.DomainEventInviteAccepted
)

type Invite struct {
	ID            string
	InviterUserID string
	InviteeUserID string
	ScheduledDate string
	ActivityLabel string
	Status        InviteStatus
}

type DomainEvent struct {
	Kind   DomainEventKind
	Invite Invite
}

func InviteFromRow(row map[string]any) Invite {
	id, _ := row["id"].(string)
	inviterUserID, _ := row["inviter_user_id"].(string)
	inviteeUserID, _ := row["invitee_user_id"].(string)
	scheduledDate, _ := row["scheduled_date"].(string)
	activityLabel, _ := row["activity_label"].(string)
	status, _ := row["status"].(string)
	return Invite{ID: id, InviterUserID: inviterUserID, InviteeUserID: inviteeUserID, ScheduledDate: scheduledDate, ActivityLabel: activityLabel, Status: InviteStatus(status)}
}

func NewInviteCreatedEvent(row map[string]any) (DomainEvent, bool) {
	invite := InviteFromRow(row)
	if invite.ID == "" || invite.InviterUserID == "" || invite.InviteeUserID == "" || invite.InviterUserID == invite.InviteeUserID {
		return DomainEvent{}, false
	}
	return DomainEvent{Kind: EventInviteCreated, Invite: invite}, true
}

func NewInviteAcceptedEvent(row map[string]any) (DomainEvent, bool) {
	invite := InviteFromRow(row)
	if invite.ID == "" || invite.InviterUserID == "" || invite.InviteeUserID == "" || invite.InviterUserID == invite.InviteeUserID {
		return DomainEvent{}, false
	}
	invite.Status = InviteStatusAccepted
	return DomainEvent{Kind: EventInviteAccepted, Invite: invite}, true
}

func (e DomainEvent) InviteRow() map[string]any {
	return map[string]any{
		"id":              e.Invite.ID,
		"inviter_user_id": e.Invite.InviterUserID,
		"invitee_user_id": e.Invite.InviteeUserID,
		"scheduled_date":  e.Invite.ScheduledDate,
		"activity_label":  e.Invite.ActivityLabel,
		"status":          string(e.Invite.Status),
	}
}

type NewInvite struct {
	InviterUserID string
	InviteeUserID string
	ScheduledDate string
	ActivityLabel string
}

type ExistingInvite struct {
	ID     string
	Status InviteStatus
}
