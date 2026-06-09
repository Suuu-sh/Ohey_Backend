package profiles

import (
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

type ErrorKind string

const (
	ErrorKindInvalidInput ErrorKind = "invalid_input"
	ErrorKindNotFound     ErrorKind = "not_found"
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
var userIDPattern = regexp.MustCompile(`^[A-Za-z0-9_]{3,24}$`)

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

func IsValidUserID(value string) bool {
	return userIDPattern.MatchString(strings.TrimSpace(value))
}

func CleanUserID(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if !IsValidUserID(trimmed) {
		return "", UserError{Kind: ErrorKindInvalidInput, Message: "user_id must be 3-24 letters, numbers, or underscores"}
	}
	return trimmed, nil
}

func CleanDisplayName(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	nameLength := utf8.RuneCountInString(trimmed)
	if nameLength < 1 || nameLength > 40 {
		return "", UserError{Kind: ErrorKindInvalidInput, Message: "display_name must be 1-40 characters"}
	}
	return trimmed, nil
}

func CleanCharacterKey(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "avatar"
	}
	return trimmed
}

func CleanAvatarURL(value any) (any, error) {
	if value == nil {
		return nil, nil
	}
	text, ok := value.(string)
	if !ok {
		return nil, UserError{Kind: ErrorKindInvalidInput, Message: "avatar_url must be a string"}
	}
	trimmed := strings.TrimSpace(text)
	if len(trimmed) > 4096 {
		return nil, UserError{Kind: ErrorKindInvalidInput, Message: "avatar_url is too long"}
	}
	return trimmed, nil
}

type Profile struct {
	ID           string `json:"id"`
	UserID       string `json:"user_id"`
	DisplayName  string `json:"display_name"`
	CharacterKey string `json:"character_key"`
	AvatarURL    string `json:"avatar_url,omitempty"`
	IsPlus       bool   `json:"is_plus"`
	Status       string `json:"status,omitempty"`
}

type BootstrapInput struct {
	AuthUserID   string
	ClerkUserID  string
	UserID       string
	DisplayName  string
	CharacterKey string
	AvatarURL    string
	UpdatedAt    time.Time
}

func BootstrapPayload(input BootstrapInput) (map[string]any, error) {
	var authUserID string
	if strings.TrimSpace(input.AuthUserID) != "" {
		var err error
		authUserID, err = CleanUUID(input.AuthUserID, "user id")
		if err != nil {
			return nil, err
		}
	}
	clerkUserID := strings.TrimSpace(input.ClerkUserID)
	if authUserID == "" && clerkUserID == "" {
		return nil, UserError{Kind: ErrorKindInvalidInput, Message: "user id is required"}
	}
	userID, err := CleanUserID(input.UserID)
	if err != nil {
		return nil, err
	}
	displayName, err := CleanDisplayName(input.DisplayName)
	if err != nil {
		return nil, err
	}
	avatarURL, err := CleanAvatarURL(input.AvatarURL)
	if err != nil {
		return nil, err
	}
	payload := map[string]any{
		"user_id":       userID,
		"display_name":  displayName,
		"character_key": CleanCharacterKey(input.CharacterKey),
		"avatar_url":    avatarURL,
		"is_plus":       false,
		"updated_at":    input.UpdatedAt.UTC().Format(time.RFC3339),
	}
	if authUserID != "" {
		payload["id"] = authUserID
	}
	if clerkUserID != "" {
		payload["clerk_user_id"] = clerkUserID
	}
	return payload, nil
}

func PatchPayload(body map[string]any, updatedAt time.Time) (map[string]any, error) {
	payload := map[string]any{}
	if raw, ok := body["user_id"]; ok {
		userID, ok := raw.(string)
		if !ok {
			return nil, UserError{Kind: ErrorKindInvalidInput, Message: "user_id must be a string"}
		}
		cleaned, err := CleanUserID(userID)
		if err != nil {
			return nil, err
		}
		payload["user_id"] = cleaned
	}
	if raw, ok := body["display_name"]; ok {
		displayName, ok := raw.(string)
		if !ok {
			return nil, UserError{Kind: ErrorKindInvalidInput, Message: "display_name must be a string"}
		}
		cleaned, err := CleanDisplayName(displayName)
		if err != nil {
			return nil, err
		}
		payload["display_name"] = cleaned
	}
	if raw, ok := body["character_key"]; ok {
		value, ok := raw.(string)
		if !ok {
			return nil, UserError{Kind: ErrorKindInvalidInput, Message: "character_key must be a string"}
		}
		payload["character_key"] = strings.TrimSpace(value)
	}
	if raw, ok := body["avatar_url"]; ok {
		cleaned, err := CleanAvatarURL(raw)
		if err != nil {
			return nil, err
		}
		payload["avatar_url"] = cleaned
	}
	payload["updated_at"] = updatedAt.UTC().Format(time.RFC3339)
	return payload, nil
}
