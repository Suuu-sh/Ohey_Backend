package memories

import (
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/yota/ohey/backend/internal/contracts"
)

type ErrorKind string

const (
	ErrorKindInvalidInput ErrorKind = "invalid_input"
	ErrorKindForbidden    ErrorKind = "forbidden"
	ErrorKindConflict     ErrorKind = "conflict"
	ErrorKindNotFound     ErrorKind = "not_found"
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

type DayWindowInput struct {
	HappenedOn            string
	TimezoneOffsetMinutes *int
}

func MemoryDayWindow(input DayWindowInput, happenedAt time.Time) (time.Time, time.Time, error) {
	happenedOn := strings.TrimSpace(input.HappenedOn)
	if happenedOn == "" {
		utc := happenedAt.UTC()
		start := time.Date(utc.Year(), utc.Month(), utc.Day(), 0, 0, 0, 0, time.UTC)
		return start, start.AddDate(0, 0, 1), nil
	}

	day, err := time.Parse(time.DateOnly, happenedOn)
	if err != nil {
		return time.Time{}, time.Time{}, UserError{Kind: ErrorKindInvalidInput, Message: "happened_on must be YYYY-MM-DD"}
	}
	offsetMinutes := 0
	if input.TimezoneOffsetMinutes != nil {
		offsetMinutes = *input.TimezoneOffsetMinutes
	}
	if offsetMinutes < -14*60 || offsetMinutes > 14*60 {
		return time.Time{}, time.Time{}, UserError{Kind: ErrorKindInvalidInput, Message: "timezone_offset_minutes is out of range"}
	}
	location := time.FixedZone("client", offsetMinutes*60)
	start := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, location)
	end := start.AddDate(0, 0, 1)
	return start.UTC(), end.UTC(), nil
}

type ReportReason string

const (
	ReportReasonSpam          ReportReason = contracts.ReportReasonSpam
	ReportReasonHarassment    ReportReason = contracts.ReportReasonHarassment
	ReportReasonInappropriate ReportReason = contracts.ReportReasonInappropriate
	ReportReasonViolence      ReportReason = contracts.ReportReasonViolence
	ReportReasonMinorSafety   ReportReason = contracts.ReportReasonMinorSafety
	ReportReasonOther         ReportReason = contracts.ReportReasonOther
)

func CleanReportReason(value string) (ReportReason, error) {
	reason := ReportReason(strings.ToLower(strings.TrimSpace(value)))
	if reason == "" {
		return ReportReasonOther, nil
	}
	switch reason {
	case ReportReasonSpam, ReportReasonHarassment, ReportReasonInappropriate, ReportReasonViolence, ReportReasonMinorSafety, ReportReasonOther:
		return reason, nil
	default:
		return "", UserError{Kind: ErrorKindInvalidInput, Message: "report reason is invalid"}
	}
}

type ModerationStatus string

const (
	ModerationStatusPending   ModerationStatus = contracts.StatusPending
	ModerationStatusReviewing ModerationStatus = contracts.ModerationStatusReviewing
	ModerationStatusResolved  ModerationStatus = contracts.ModerationStatusResolved
	ModerationStatusDismissed ModerationStatus = contracts.ModerationStatusDismissed
)

func CleanModerationStatus(value string) (ModerationStatus, error) {
	status := ModerationStatus(strings.ToLower(strings.TrimSpace(value)))
	switch status {
	case ModerationStatusPending, ModerationStatusReviewing, ModerationStatusResolved, ModerationStatusDismissed:
		return status, nil
	default:
		return "", UserError{Kind: ErrorKindInvalidInput, Message: "moderation status is invalid"}
	}
}

type Report struct {
	ID             string
	MemoryID       string
	ReporterUserID string
	Reason         ReportReason
	Status         ModerationStatus
}

func (r Report) EffectiveStatus() ModerationStatus {
	if r.Status == "" {
		return ModerationStatusPending
	}
	return r.Status
}

func NewReportBody(report Report, duplicate bool) map[string]any {
	return map[string]any{
		"reported":  true,
		"duplicate": duplicate,
		"hidden":    true,
		"reason":    string(report.Reason),
		"status":    string(report.EffectiveStatus()),
	}
}

type NewMemory struct {
	OwnerUserID string
	HappenedAt  time.Time
	PlaceName   string
	PlaceLat    *float64
	PlaceLng    *float64
	Memo        string
	IsOfficial  bool
}

type LikeState struct {
	LikeCount int  `json:"like_count"`
	LikedByMe bool `json:"liked_by_me"`
}

type ReportResult struct {
	Created bool
	Body    map[string]any
}

type DomainEventKind string

const (
	EventMemoryTagged   DomainEventKind = contracts.DomainEventMemoryTagged
	EventMemoryLiked    DomainEventKind = contracts.DomainEventMemoryLiked
	EventMemoryReported DomainEventKind = contracts.DomainEventMemoryReported
)

type DomainEvent struct {
	Kind             DomainEventKind
	MemoryID         string
	OwnerUserID      string
	ActorUserID      string
	FriendIDs        []string
	ReportReason     ReportReason
	ModerationStatus ModerationStatus
}

func NewMemoryTaggedEvent(memoryID, ownerUserID string, friendIDs []string) (DomainEvent, bool) {
	if memoryID == "" || ownerUserID == "" || len(friendIDs) == 0 {
		return DomainEvent{}, false
	}
	return DomainEvent{Kind: EventMemoryTagged, MemoryID: memoryID, OwnerUserID: ownerUserID, ActorUserID: ownerUserID, FriendIDs: append([]string(nil), friendIDs...)}, true
}

func NewMemoryLikedEvent(memoryID, actorUserID string) (DomainEvent, bool) {
	if memoryID == "" || actorUserID == "" {
		return DomainEvent{}, false
	}
	return DomainEvent{Kind: EventMemoryLiked, MemoryID: memoryID, ActorUserID: actorUserID}, true
}

func NewMemoryReportedEvent(memoryID, ownerUserID, reporterUserID string, reason ReportReason) (DomainEvent, bool) {
	if memoryID == "" || ownerUserID == "" || reporterUserID == "" || ownerUserID == reporterUserID {
		return DomainEvent{}, false
	}
	return DomainEvent{
		Kind:             EventMemoryReported,
		MemoryID:         memoryID,
		OwnerUserID:      ownerUserID,
		ActorUserID:      reporterUserID,
		ReportReason:     reason,
		ModerationStatus: ModerationStatusPending,
	}, true
}
