package usersafety

import (
	"errors"
	"regexp"
	"strings"

	"github.com/yota/ohey/backend/internal/contracts"
)

type ErrorKind string

const (
	ErrorKindInvalidInput ErrorKind = "invalid_input"
	ErrorKindForbidden    ErrorKind = "forbidden"
)

type UserError struct {
	Kind    ErrorKind
	Message string
}

func (e UserError) Error() string { return e.Message }

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

func ValidateDifferentUsers(actorUserID, targetUserID string) error {
	if actorUserID == targetUserID {
		return UserError{Kind: ErrorKindForbidden, Message: "target user must be different from yourself"}
	}
	return nil
}

type UserRelation struct {
	ActorUserID  string
	TargetUserID string
}

type UserReport struct {
	ReporterUserID string
	ReportedUserID string
	Reason         string
}

type HiddenMemory struct {
	UserID   string
	MemoryID string
}

func CleanReportReason(value string) (string, error) {
	reason := strings.ToLower(strings.TrimSpace(value))
	if reason == "" {
		return contracts.ReportReasonOther, nil
	}
	switch reason {
	case contracts.ReportReasonSpam,
		contracts.ReportReasonHarassment,
		contracts.ReportReasonInappropriate,
		contracts.ReportReasonViolence,
		contracts.ReportReasonMinorSafety,
		contracts.ReportReasonOther:
		return reason, nil
	default:
		return "", UserError{Kind: ErrorKindInvalidInput, Message: "report reason is invalid"}
	}
}
