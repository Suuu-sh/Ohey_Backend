package friends

import (
	"errors"
	"regexp"
	"strings"

	"github.com/Suuu-sh/Ohey_Backend/internal/contracts"
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

type RequestStatus string

const (
	RequestStatusAccepted  RequestStatus = contracts.StatusAccepted
	RequestStatusRejected  RequestStatus = contracts.StatusRejected
	RequestStatusCancelled RequestStatus = contracts.StatusCancelled
)

func NormalizeRequestStatus(value string) (RequestStatus, error) {
	status := RequestStatus(strings.TrimSpace(value))
	switch status {
	case RequestStatusAccepted, RequestStatusRejected, RequestStatusCancelled:
		return status, nil
	default:
		return "", UserError{Kind: ErrorKindInvalidInput, Message: "status must be accepted, rejected, or cancelled"}
	}
}

type RequestDirection string

const (
	RequestDirectionAll      RequestDirection = contracts.RequestDirectionAll
	RequestDirectionIncoming RequestDirection = contracts.RequestDirectionIncoming
	RequestDirectionOutgoing RequestDirection = contracts.RequestDirectionOutgoing
)

func NormalizeRequestDirection(value string) (RequestDirection, error) {
	direction := RequestDirection(strings.TrimSpace(value))
	if direction == "" {
		return RequestDirectionAll, nil
	}
	switch direction {
	case RequestDirectionAll, RequestDirectionIncoming, RequestDirectionOutgoing:
		return direction, nil
	default:
		return "", UserError{Kind: ErrorKindInvalidInput, Message: "direction must be all, incoming, or outgoing"}
	}
}

func OrderedPair(a, b string) (string, string) {
	if a < b {
		return a, b
	}
	return b, a
}

type FriendRequestStatus struct {
	AlreadyFriend bool   `json:"already_friend"`
	RequestState  string `json:"request_state"`
	RequestID     string `json:"request_id,omitempty"`
}

type FriendRequest struct {
	ID         string
	FromUserID string
	ToUserID   string
	Status     string
}

func FriendRequestFromRow(row map[string]any) FriendRequest {
	id, _ := row["id"].(string)
	fromUserID, _ := row["from_user_id"].(string)
	toUserID, _ := row["to_user_id"].(string)
	status, _ := row["status"].(string)
	return FriendRequest{ID: id, FromUserID: fromUserID, ToUserID: toUserID, Status: status}
}

type DomainEventKind string

const (
	EventFriendRequestCreated  DomainEventKind = contracts.DomainEventFriendRequestCreated
	EventFriendRequestAccepted DomainEventKind = contracts.DomainEventFriendRequestAccepted
)

type DomainEvent struct {
	Kind    DomainEventKind
	Request FriendRequest
}

func NewFriendRequestCreatedEvent(row map[string]any) (DomainEvent, bool) {
	request := FriendRequestFromRow(row)
	if request.ID == "" || request.FromUserID == "" || request.ToUserID == "" || request.FromUserID == request.ToUserID {
		return DomainEvent{}, false
	}
	return DomainEvent{Kind: EventFriendRequestCreated, Request: request}, true
}

func NewFriendRequestAcceptedEvent(row map[string]any) (DomainEvent, bool) {
	request := FriendRequestFromRow(row)
	if request.ID == "" || request.FromUserID == "" || request.ToUserID == "" || request.FromUserID == request.ToUserID {
		return DomainEvent{}, false
	}
	request.Status = string(RequestStatusAccepted)
	return DomainEvent{Kind: EventFriendRequestAccepted, Request: request}, true
}

func (e DomainEvent) RequestRow() map[string]any {
	return map[string]any{
		"id":           e.Request.ID,
		"from_user_id": e.Request.FromUserID,
		"to_user_id":   e.Request.ToUserID,
		"status":       e.Request.Status,
	}
}
