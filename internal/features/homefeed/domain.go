package homefeed

import (
	"errors"
	"hash/fnv"
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

type FeedItem struct {
	ID          string         `json:"id"`
	Type        string         `json:"type"`
	PostKind    string         `json:"post_kind"`
	Displayable bool           `json:"displayable"`
	AuthorName  string         `json:"author_name"`
	OwnerUserID string         `json:"owner_user_id"`
	OwnedByMe   bool           `json:"owned_by_me"`
	IsOfficial  bool           `json:"is_official"`
	Body        string         `json:"body"`
	Place       string         `json:"place"`
	PhotoPath   string         `json:"photo_path"`
	CaptionY    float64        `json:"caption_y"`
	LinkURL     string         `json:"link_url"`
	LikeCount   int            `json:"like_count"`
	LikedByMe   bool           `json:"liked_by_me"`
	CanLike     bool           `json:"can_like"`
	CanReport   bool           `json:"can_report"`
	CanDelete   bool           `json:"can_delete"`
	SortAt      string         `json:"sort_at"`
	AccentSeed  int            `json:"accent_seed"`
	Tilt        float64        `json:"tilt"`
	Prop        string         `json:"prop"`
	Sparkles    []FeedOffset   `json:"sparkles"`
	Author      map[string]any `json:"author,omitempty"`
}

type FeedOffset struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

func BuildFeedItem(row map[string]any, currentUserID string) (FeedItem, bool) {
	id := stringValue(row, "id")
	ownerUserID := stringValue(row, "owner_user_id")
	isOfficial := boolValue(row, "is_official")
	ownedByMe := ownerUserID != "" && ownerUserID == currentUserID
	photoPath := strings.TrimSpace(stringValue(row, "photo_path"))
	displayable := isOfficial || (photoPath != "" && ownerUserID != "")
	if !displayable || id == "" {
		return FeedItem{}, false
	}
	postKind := "friend"
	switch {
	case isOfficial:
		postKind = "official"
	case ownedByMe:
		postKind = "mine"
	}
	owner := mapValue(row, "owner")
	authorName := feedAuthorName(owner, isOfficial)
	sortAt := stringValue(row, "drank_at")
	item := FeedItem{
		ID:          id,
		Type:        "drink_log",
		PostKind:    postKind,
		Displayable: true,
		AuthorName:  authorName,
		OwnerUserID: ownerUserID,
		OwnedByMe:   ownedByMe,
		IsOfficial:  isOfficial,
		Body:        strings.TrimSpace(stringValue(row, "memo")),
		Place:       strings.TrimSpace(stringValue(row, "place_name")),
		PhotoPath:   photoPath,
		CaptionY:    captionYValue(row),
		LinkURL:     strings.TrimSpace(stringValue(row, "link_url")),
		LikeCount:   intValue(row, "like_count"),
		LikedByMe:   boolValue(row, "liked_by_me"),
		CanLike:     id != "",
		CanReport:   id != "" && !ownedByMe && !isOfficial,
		CanDelete:   id != "" && ownedByMe && !isOfficial,
		SortAt:      sortAt,
		AccentSeed:  accentSeed(id),
		Tilt:        tiltForID(id),
		Prop:        "beer",
		Sparkles: []FeedOffset{
			{X: 12, Y: 18},
			{X: 54, Y: 2},
			{X: 118, Y: 26},
			{X: 28, Y: 66},
		},
		Author: owner,
	}
	return item, true
}

func AttachFeedItem(row map[string]any, item FeedItem) map[string]any {
	row["feed_item"] = item
	row["feed_post_kind"] = item.PostKind
	row["feed_displayable"] = item.Displayable
	row["feed_author_name"] = item.AuthorName
	row["feed_owned_by_me"] = item.OwnedByMe
	row["feed_can_report"] = item.CanReport
	row["feed_can_delete"] = item.CanDelete
	row["feed_accent_seed"] = item.AccentSeed
	row["feed_tilt"] = item.Tilt
	row["feed_prop"] = item.Prop
	return row
}

func HideReportedRows(rows []map[string]any, hiddenDrinkLogIDs map[string]bool) []map[string]any {
	if len(hiddenDrinkLogIDs) == 0 {
		return rows
	}
	filtered := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		if id := stringValue(row, "id"); id != "" && hiddenDrinkLogIDs[id] {
			continue
		}
		filtered = append(filtered, row)
	}
	return filtered
}

func feedAuthorName(owner map[string]any, isOfficial bool) string {
	if isOfficial {
		return "Nomo"
	}
	for _, key := range []string{"display_name", "user_id"} {
		if value := strings.TrimSpace(stringValue(owner, key)); value != "" {
			return value
		}
	}
	return "nomo_user"
}

func captionYValue(row map[string]any) float64 {
	value, ok := numericValue(row["caption_y"])
	if !ok {
		return 0.5
	}
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func accentSeed(value string) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(value))
	return int(h.Sum32() % 100000)
}

func tiltForID(value string) float64 {
	if accentSeed(value)%2 == 0 {
		return -0.08
	}
	return 0.08
}

func stringValue(row map[string]any, key string) string {
	value, _ := row[key].(string)
	return value
}

func boolValue(row map[string]any, key string) bool {
	value, _ := row[key].(bool)
	return value
}

func intValue(row map[string]any, key string) int {
	value, ok := numericValue(row[key])
	if !ok {
		return 0
	}
	return int(value)
}

func numericValue(value any) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case int32:
		return float64(v), true
	case jsonNumber:
		parsed, err := v.Float64()
		return parsed, err == nil
	default:
		return 0, false
	}
}

type jsonNumber interface {
	Float64() (float64, error)
}

func mapValue(row map[string]any, key string) map[string]any {
	if value, ok := row[key].(map[string]any); ok {
		return value
	}
	if value, ok := row[key].(map[string]interface{}); ok {
		return value
	}
	return map[string]any{}
}

func RowTime(row map[string]any) time.Time {
	parsed, err := time.Parse(time.RFC3339, stringValue(row, "drank_at"))
	if err == nil {
		return parsed
	}
	return time.Time{}
}
