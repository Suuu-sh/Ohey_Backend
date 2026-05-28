package memories

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"regexp"
	"strings"
	"time"
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

type MarkerRarity string

const (
	MarkerRarityNormal    MarkerRarity = "normal"
	MarkerRarityUncommon  MarkerRarity = "uncommon"
	MarkerRarityRare      MarkerRarity = "rare"
	MarkerRaritySuperRare MarkerRarity = "super_rare"
	MarkerRarityUltraRare MarkerRarity = "ultra_rare"
	MarkerRaritySecret    MarkerRarity = "secret"
)

func MarkerRarityForPhotoRoll(roll float64) MarkerRarity {
	switch {
	case roll < 0.001:
		return MarkerRaritySecret
	case roll < 0.010:
		return MarkerRarityUltraRare
	case roll < 0.070:
		return MarkerRaritySuperRare
	case roll < 0.250:
		return MarkerRarityRare
	default:
		return MarkerRarityUncommon
	}
}

func RandomFloat64() float64 {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return float64(time.Now().UnixNano()%1_000_000) / 1_000_000
	}
	value := binary.BigEndian.Uint64(buf[:]) >> 11
	return float64(value) / (1 << 53)
}

func MarkerRarityForPhotoPath(photoPath string, randomFloat func() float64) MarkerRarity {
	if strings.TrimSpace(photoPath) == "" {
		return MarkerRarityNormal
	}
	if randomFloat == nil {
		randomFloat = RandomFloat64
	}
	return MarkerRarityForPhotoRoll(randomFloat())
}

func CleanCaptionY(value *float64) float64 {
	if value == nil {
		return 0.5
	}
	if *value < 0 {
		return 0
	}
	if *value > 1 {
		return 1
	}
	return *value
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

func CleanUserPhotoPath(ownerUserID, value string) (string, error) {
	path := strings.TrimSpace(value)
	if path == "" {
		return "", nil
	}
	if len(path) > 512 {
		return "", UserError{Kind: ErrorKindInvalidInput, Message: "photo_path is too long"}
	}
	if strings.HasPrefix(path, "/") || strings.Contains(path, "..") || strings.Contains(path, "\\") {
		return "", UserError{Kind: ErrorKindInvalidInput, Message: "photo_path is invalid"}
	}
	prefix := "users/" + ownerUserID + "/memories/"
	if !strings.HasPrefix(path, prefix) {
		return "", UserError{Kind: ErrorKindInvalidInput, Message: "photo_path must be an uploaded memory photo"}
	}
	if !hasAllowedPhotoExtension(path) {
		return "", UserError{Kind: ErrorKindInvalidInput, Message: "photo_path file type is unsupported"}
	}
	return path, nil
}

func hasAllowedPhotoExtension(path string) bool {
	lower := strings.ToLower(path)
	for _, suffix := range []string{".jpg", ".jpeg", ".png", ".heic", ".webp"} {
		if strings.HasSuffix(lower, suffix) {
			return true
		}
	}
	return false
}

type ReportReason string

const (
	ReportReasonSpam          ReportReason = "spam"
	ReportReasonHarassment    ReportReason = "harassment"
	ReportReasonInappropriate ReportReason = "inappropriate"
	ReportReasonViolence      ReportReason = "violence"
	ReportReasonMinorSafety   ReportReason = "minor_safety"
	ReportReasonOther         ReportReason = "other"
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
	ModerationStatusPending   ModerationStatus = "pending"
	ModerationStatusReviewing ModerationStatus = "reviewing"
	ModerationStatusResolved  ModerationStatus = "resolved"
	ModerationStatusDismissed ModerationStatus = "dismissed"
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
	OwnerUserID  string
	HappenedAt   time.Time
	PlaceName    string
	PlaceLat     *float64
	PlaceLng     *float64
	Memo         string
	CaptionY     float64
	PhotoPath    string
	MarkerRarity MarkerRarity
	IsOfficial   bool
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
	EventMemoryTagged   DomainEventKind = "memory.tagged"
	EventMemoryLiked    DomainEventKind = "memory.liked"
	EventMemoryReported DomainEventKind = "memory.reported"
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
