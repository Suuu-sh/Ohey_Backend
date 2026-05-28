package dailystatuses

import (
	"errors"
	"regexp"
	"strings"
	"time"
)

type ErrorKind string

const (
	ErrorKindInvalidInput ErrorKind = "invalid_input"
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

type Status string

const (
	StatusUnselected     Status = "unselected"
	StatusAvailable      Status = "available"
	StatusMaybeAvailable Status = "maybe_available"
	StatusDependsOnTime  Status = "depends_on_time"
	StatusHasPlans       Status = "has_plans"
)

func CleanStatus(value string) (Status, error) {
	status := Status(strings.ToLower(strings.TrimSpace(value)))
	switch status {
	case StatusUnselected, StatusAvailable, StatusMaybeAvailable, StatusDependsOnTime, StatusHasPlans:
		return status, nil
	default:
		return "", UserError{Kind: ErrorKindInvalidInput, Message: "status is invalid"}
	}
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

type MonthRange struct {
	Month     string
	StartDate string
	EndDate   string
}

func CleanMonth(value string, now time.Time) (MonthRange, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		if now.IsZero() {
			now = time.Now()
		}
		start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
		end := start.AddDate(0, 1, 0)
		return MonthRange{Month: start.Format("2006-01"), StartDate: start.Format(time.DateOnly), EndDate: end.Format(time.DateOnly)}, nil
	}
	parsed, err := time.Parse("2006-01", trimmed)
	if err != nil {
		return MonthRange{}, UserError{Kind: ErrorKindInvalidInput, Message: "month must be YYYY-MM"}
	}
	start := time.Date(parsed.Year(), parsed.Month(), 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 1, 0)
	return MonthRange{Month: start.Format("2006-01"), StartDate: start.Format(time.DateOnly), EndDate: end.Format(time.DateOnly)}, nil
}

type DailyStatus struct {
	UserID     string
	StatusDate string
	Status     Status
}
