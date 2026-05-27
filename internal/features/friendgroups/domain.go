package friendgroups

import (
	"errors"
	"regexp"
	"strings"
	"unicode/utf8"
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
var clientIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

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

func CleanClientID(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if !clientIDPattern.MatchString(trimmed) {
		return "", UserError{Kind: ErrorKindInvalidInput, Message: "group id is invalid"}
	}
	return trimmed, nil
}

func CleanName(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	length := utf8.RuneCountInString(trimmed)
	if length < 1 || length > 24 {
		return "", UserError{Kind: ErrorKindInvalidInput, Message: "group name must be 1-24 characters"}
	}
	return trimmed, nil
}

func CleanFriendIDs(values []string) ([]string, error) {
	seen := map[string]bool{}
	ids := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		id, err := CleanUUID(trimmed, "friend id")
		if err != nil {
			return nil, err
		}
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil, UserError{Kind: ErrorKindInvalidInput, Message: "friend_ids must contain at least one friend"}
	}
	if len(ids) > 100 {
		return nil, UserError{Kind: ErrorKindInvalidInput, Message: "friend_ids is too large"}
	}
	return ids, nil
}

type GroupInput struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	FriendIDs []string `json:"friend_ids"`
	FriendIds []string `json:"friendIds"`
}

type SaveInputBody struct {
	Groups []GroupInput `json:"groups"`
}

type FriendGroup struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	FriendIDs []string `json:"friend_ids"`
	FriendIds []string `json:"friendIds"`
	SortOrder int      `json:"sort_order"`
}

func NormalizeGroups(inputs []GroupInput) ([]FriendGroup, error) {
	if len(inputs) > 20 {
		return nil, UserError{Kind: ErrorKindInvalidInput, Message: "groups is too large"}
	}
	seenIDs := map[string]bool{}
	groups := make([]FriendGroup, 0, len(inputs))
	for index, input := range inputs {
		id, err := CleanClientID(input.ID)
		if err != nil {
			return nil, err
		}
		if seenIDs[id] {
			return nil, UserError{Kind: ErrorKindInvalidInput, Message: "group id is duplicated"}
		}
		seenIDs[id] = true
		name, err := CleanName(input.Name)
		if err != nil {
			return nil, err
		}
		friendIDs := input.FriendIDs
		if len(friendIDs) == 0 {
			friendIDs = input.FriendIds
		}
		cleanFriendIDs, err := CleanFriendIDs(friendIDs)
		if err != nil {
			return nil, err
		}
		groups = append(groups, FriendGroup{ID: id, Name: name, FriendIDs: cleanFriendIDs, FriendIds: cleanFriendIDs, SortOrder: index})
	}
	return groups, nil
}
