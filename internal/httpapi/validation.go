package httpapi

import (
	"regexp"
	"strings"
	"time"
)

const (
	maxSearchRunes = 64
)

var uuidPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func cleanUUID(value, field string) (string, string) {
	trimmed := strings.ToLower(strings.TrimSpace(value))
	if trimmed == "" {
		return "", field + " is required"
	}
	if !uuidPattern.MatchString(trimmed) {
		return "", field + " must be a valid UUID"
	}
	return trimmed, ""
}

func cleanUUIDs(values []string, field string) ([]string, string) {
	seen := map[string]bool{}
	ids := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		id, errMessage := cleanUUID(trimmed, field)
		if errMessage != "" {
			return nil, errMessage
		}
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	return ids, ""
}

func cleanDateOnlyOrToday(value, field string) (string, string) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return time.Now().Format(time.DateOnly), ""
	}
	parsed, err := time.Parse(time.DateOnly, trimmed)
	if err != nil {
		return "", field + " must be YYYY-MM-DD"
	}
	return parsed.Format(time.DateOnly), ""
}

func shortText(value string, limit int) string {
	trimmed := strings.TrimSpace(value)
	runes := []rune(trimmed)
	if len(runes) <= limit {
		return trimmed
	}
	return string(runes[:limit])
}
