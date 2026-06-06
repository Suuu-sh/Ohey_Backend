package yurubos

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/yota/ohey/backend/internal/contracts"
)

type ErrorKind string

const (
	ErrorKindInvalidInput ErrorKind = "invalid_input"
	ErrorKindForbidden    ErrorKind = "forbidden"
	ErrorKindNotFound     ErrorKind = "not_found"
	ErrorKindUpstream     ErrorKind = "upstream"
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

func CleanOptionalUUID(value, field string) (string, bool, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", false, nil
	}
	id, err := CleanUUID(trimmed, field)
	if err != nil {
		return "", false, err
	}
	return id, true, nil
}

func CleanLimit(raw string, defaultLimit int) int {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return defaultLimit
	}
	parsed, err := strconv.Atoi(trimmed)
	if err != nil || parsed <= 0 || parsed > 100 {
		return defaultLimit
	}
	return parsed
}

func CleanTitle(value string) (string, error) {
	title := strings.TrimSpace(value)
	if title == "" {
		return "", UserError{Kind: ErrorKindInvalidInput, Message: "title is required"}
	}
	if utf8.RuneCountInString(title) > 80 {
		return "", UserError{Kind: ErrorKindInvalidInput, Message: "title is too long"}
	}
	return title, nil
}

func CleanCategory(value string) string {
	category := strings.TrimSpace(value)
	if category == "" {
		return contracts.CategoryOther
	}
	return category
}

func CleanCreateVisibility(value, groupIDValue string) (visibility string, groupID string, err error) {
	visibility = strings.TrimSpace(value)
	if visibility == "" {
		visibility = contracts.VisibilityFriends
	}
	if visibility != contracts.VisibilityFriends && visibility != contracts.VisibilityGroup {
		return "", "", UserError{Kind: ErrorKindInvalidInput, Message: "invalid visibility"}
	}
	if visibility == contracts.VisibilityGroup {
		groupID, err = CleanUUID(groupIDValue, "group id")
		if err != nil {
			return "", "", err
		}
	}
	return visibility, groupID, nil
}

func CleanReactionType(value string) (string, error) {
	reaction := strings.TrimSpace(value)
	if reaction == "" {
		reaction = contracts.ReactionTypeInterested
	}
	switch reaction {
	case contracts.ReactionTypeInterested, contracts.ReactionTypeAvailable, contracts.ReactionTypeAnotherDay:
		return reaction, nil
	default:
		return "", UserError{Kind: ErrorKindInvalidInput, Message: "invalid reaction_type"}
	}
}

func NormalizeStartsAt(raw string) (string, bool, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", false, nil
	}
	if parsed, err := time.Parse(time.RFC3339, trimmed); err == nil {
		return parsed.UTC().Format(time.RFC3339), true, nil
	}
	if parsed, err := time.Parse("2006-01-02", trimmed); err == nil {
		return parsed.UTC().Format(time.RFC3339), true, nil
	}
	return "", false, UserError{Kind: ErrorKindInvalidInput, Message: fmt.Sprintf("invalid starts_at")}
}

type ReactionBody struct {
	ReactionType string `json:"reaction_type"`
}

type CreateBody struct {
	Title      string `json:"title"`
	Body       string `json:"body"`
	Category   string `json:"category"`
	PlaceText  string `json:"place_text"`
	TimeLabel  string `json:"time_label"`
	StartsAt   string `json:"starts_at"`
	Visibility string `json:"visibility"`
	GroupID    string `json:"group_id"`
	WishItemID string `json:"wish_item_id"`
}

type UpdateBody struct {
	Title     string  `json:"title"`
	Body      string  `json:"body"`
	PlaceText string  `json:"place_text"`
	TimeLabel string  `json:"time_label"`
	StartsAt  *string `json:"starts_at"`
}

type Yurubo struct {
	OwnerUserID string
	Title       string
	Body        string
	Category    string
	PlaceText   string
	TimeLabel   string
	StartsAt    *string
	Visibility  string
	WishItemID  *string
}

type YuruboUpdate struct {
	YuruboID    string
	OwnerUserID string
	Title       string
	Body        string
	PlaceText   string
	TimeLabel   string
	StartsAtSet bool
	StartsAt    *string
}

type Reaction struct {
	YuruboID     string
	UserID       string
	ReactionType string
}

type ReactionState struct {
	ReactedByMe bool `json:"reacted_by_me"`
}

type ApprovalState struct {
	Approved bool `json:"approved"`
}

type DomainEventKind string

const EventYuruboCreated DomainEventKind = "yurubo.created"

type DomainEvent struct {
	Kind     DomainEventKind
	Yurubo   Yurubo
	Row      map[string]any
	GroupIDs []string
}

type EventPublisher interface {
	Publish(ctx context.Context, authToken string, event DomainEvent)
}

func NewYurubo(ownerUserID string, body CreateBody) (Yurubo, string, error) {
	ownerUserID, err := CleanUUID(ownerUserID, "owner user id")
	if err != nil {
		return Yurubo{}, "", err
	}
	title, err := CleanTitle(body.Title)
	if err != nil {
		return Yurubo{}, "", err
	}
	visibility, groupID, err := CleanCreateVisibility(body.Visibility, body.GroupID)
	if err != nil {
		return Yurubo{}, "", err
	}
	var startsAt *string
	if normalized, ok, err := NormalizeStartsAt(body.StartsAt); err != nil {
		return Yurubo{}, "", err
	} else if ok {
		startsAt = &normalized
	}
	var wishItemID *string
	if id, ok, err := CleanOptionalUUID(body.WishItemID, "wish item id"); err != nil {
		return Yurubo{}, "", err
	} else if ok {
		wishItemID = &id
	}
	return Yurubo{
		OwnerUserID: ownerUserID,
		Title:       title,
		Body:        strings.TrimSpace(body.Body),
		Category:    CleanCategory(body.Category),
		PlaceText:   strings.TrimSpace(body.PlaceText),
		TimeLabel:   strings.TrimSpace(body.TimeLabel),
		StartsAt:    startsAt,
		Visibility:  visibility,
		WishItemID:  wishItemID,
	}, groupID, nil
}

func NewYuruboUpdate(yuruboID, ownerUserID string, body UpdateBody) (YuruboUpdate, error) {
	yuruboID, err := CleanUUID(yuruboID, "yurubo id")
	if err != nil {
		return YuruboUpdate{}, err
	}
	ownerUserID, err = CleanUUID(ownerUserID, "owner user id")
	if err != nil {
		return YuruboUpdate{}, err
	}
	title, err := CleanTitle(body.Title)
	if err != nil {
		return YuruboUpdate{}, err
	}
	update := YuruboUpdate{
		YuruboID:    yuruboID,
		OwnerUserID: ownerUserID,
		Title:       title,
		Body:        strings.TrimSpace(body.Body),
		PlaceText:   strings.TrimSpace(body.PlaceText),
		TimeLabel:   strings.TrimSpace(body.TimeLabel),
	}
	if body.StartsAt != nil {
		update.StartsAtSet = true
		if normalized, ok, err := NormalizeStartsAt(*body.StartsAt); err != nil {
			return YuruboUpdate{}, err
		} else if ok {
			update.StartsAt = &normalized
		}
	}
	return update, nil
}
