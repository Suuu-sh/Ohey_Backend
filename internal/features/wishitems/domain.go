package wishitems

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/Suuu-sh/Ohey_Backend/internal/contracts"
)

type ErrorKind string

const (
	ErrorKindInvalidInput ErrorKind = "invalid_input"
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

func CleanVisibility(value string) (string, error) {
	visibility := strings.TrimSpace(value)
	if visibility == "" {
		return contracts.VisibilityPrivate, nil
	}
	switch visibility {
	case contracts.VisibilityPrivate, contracts.VisibilityFriends:
		return visibility, nil
	default:
		return "", UserError{Kind: ErrorKindInvalidInput, Message: "invalid visibility"}
	}
}

type CreateBody struct {
	Title      string `json:"title"`
	Note       string `json:"note"`
	Category   string `json:"category"`
	PlaceText  string `json:"place_text"`
	PlaceURL   string `json:"place_url"`
	Visibility string `json:"visibility"`
}

type UpdateBody = CreateBody

type WishItem struct {
	OwnerUserID string
	Title       string
	Note        string
	Category    string
	PlaceText   string
	PlaceURL    string
	Visibility  string
}

type WishItemUpdate struct {
	WishItemID  string
	OwnerUserID string
	Title       string
	Note        string
	Category    string
	PlaceText   string
	PlaceURL    string
	Visibility  string
}

func NewWishItem(ownerUserID string, body CreateBody) (WishItem, error) {
	ownerUserID, err := CleanUUID(ownerUserID, "owner user id")
	if err != nil {
		return WishItem{}, err
	}
	title, err := CleanTitle(body.Title)
	if err != nil {
		return WishItem{}, err
	}
	visibility, err := CleanVisibility(body.Visibility)
	if err != nil {
		return WishItem{}, err
	}
	return WishItem{
		OwnerUserID: ownerUserID,
		Title:       title,
		Note:        strings.TrimSpace(body.Note),
		Category:    CleanCategory(body.Category),
		PlaceText:   strings.TrimSpace(body.PlaceText),
		PlaceURL:    strings.TrimSpace(body.PlaceURL),
		Visibility:  visibility,
	}, nil
}

func NewWishItemUpdate(wishItemID, ownerUserID string, body UpdateBody) (WishItemUpdate, error) {
	wishItemID, err := CleanUUID(wishItemID, "wish item id")
	if err != nil {
		return WishItemUpdate{}, err
	}
	ownerUserID, err = CleanUUID(ownerUserID, "owner user id")
	if err != nil {
		return WishItemUpdate{}, err
	}
	title, err := CleanTitle(body.Title)
	if err != nil {
		return WishItemUpdate{}, err
	}
	visibility, err := CleanVisibility(body.Visibility)
	if err != nil {
		return WishItemUpdate{}, err
	}
	return WishItemUpdate{
		WishItemID:  wishItemID,
		OwnerUserID: ownerUserID,
		Title:       title,
		Note:        strings.TrimSpace(body.Note),
		Category:    CleanCategory(body.Category),
		PlaceText:   strings.TrimSpace(body.PlaceText),
		PlaceURL:    strings.TrimSpace(body.PlaceURL),
		Visibility:  visibility,
	}, nil
}
