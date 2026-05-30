package homefeed

import (
	"errors"
	"fmt"
	"hash/fnv"
	"regexp"
	"strconv"
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
	RankScore   int64          `json:"rank_score"`
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
	sortAt := stringValue(row, "happened_at")
	item := FeedItem{
		ID:          id,
		Type:        "memory",
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
		RankScore:   RankScore(row, currentUserID),
		AccentSeed:  accentSeed(id),
		Tilt:        tiltForID(id),
		Prop:        "memory",
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
	row["rank_score"] = item.RankScore
	row["feed_rank_score"] = item.RankScore
	row["feed_cursor"] = EncodeCursor(item)
	row["feed_accent_seed"] = item.AccentSeed
	row["feed_tilt"] = item.Tilt
	row["feed_prop"] = item.Prop
	return row
}

func HideReportedRows(rows []map[string]any, hiddenMemoryIDs map[string]bool) []map[string]any {
	if len(hiddenMemoryIDs) == 0 {
		return rows
	}
	filtered := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		if id := stringValue(row, "id"); id != "" && hiddenMemoryIDs[id] {
			continue
		}
		filtered = append(filtered, row)
	}
	return filtered
}

func HideRowsByOwner(rows []map[string]any, hiddenUserIDs map[string]bool) []map[string]any {
	if len(hiddenUserIDs) == 0 {
		return rows
	}
	filtered := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		ownerUserID := stringValue(row, "owner_user_id")
		if ownerUserID != "" && hiddenUserIDs[ownerUserID] && !boolValue(row, "is_official") {
			continue
		}
		filtered = append(filtered, row)
	}
	return filtered
}

func ExcludeHiddenUserIDs(userIDs []string, hiddenUserIDs map[string]bool) []string {
	if len(hiddenUserIDs) == 0 {
		return userIDs
	}
	filtered := make([]string, 0, len(userIDs))
	for _, id := range userIDs {
		if id != "" && hiddenUserIDs[id] {
			continue
		}
		filtered = append(filtered, id)
	}
	return filtered
}

func CleanLimit(value string, defaultLimit, maxLimit int) (int, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return defaultLimit, nil
	}
	limit, err := strconv.Atoi(trimmed)
	if err != nil || limit <= 0 {
		return 0, UserError{Kind: ErrorKindInvalidInput, Message: "limit must be a positive integer"}
	}
	if maxLimit > 0 && limit > maxLimit {
		return maxLimit, nil
	}
	return limit, nil
}

func CleanCursor(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", nil
	}
	if len(trimmed) > 160 {
		return "", UserError{Kind: ErrorKindInvalidInput, Message: "cursor is invalid"}
	}
	return trimmed, nil
}

func RankScore(row map[string]any, currentUserID string) int64 {
	score := RowTime(row).Unix()
	if RowTime(row).IsZero() {
		score = 0
	}
	ownerUserID := stringValue(row, "owner_user_id")
	switch {
	case boolValue(row, "is_official"):
		score += 300
	case ownerUserID != "" && ownerUserID == currentUserID:
		score += 200
	default:
		score += 100
	}
	return score
}

func EncodeCursor(item FeedItem) string {
	if item.ID == "" {
		return ""
	}
	return fmt.Sprintf("%d:%s", item.RankScore, item.ID)
}

type FeedCursor struct {
	RankScore int64
	ID        string
	SortAt    time.Time
}

func ParseCursor(value string) (FeedCursor, bool) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return FeedCursor{}, false
	}
	if parsed, err := time.Parse(time.RFC3339, trimmed); err == nil {
		return FeedCursor{SortAt: parsed}, true
	}
	parts := strings.SplitN(trimmed, ":", 2)
	if len(parts) != 2 {
		return FeedCursor{}, false
	}
	rankScore, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || strings.TrimSpace(parts[1]) == "" {
		return FeedCursor{}, false
	}
	return FeedCursor{RankScore: rankScore, ID: strings.TrimSpace(parts[1])}, true
}

func feedAuthorName(owner map[string]any, isOfficial bool) string {
	if isOfficial {
		return "Ohey"
	}
	for _, key := range []string{"display_name", "user_id"} {
		if value := strings.TrimSpace(stringValue(owner, key)); value != "" {
			return value
		}
	}
	return "ohey_user"
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
	parsed, err := time.Parse(time.RFC3339, stringValue(row, "happened_at"))
	if err == nil {
		return parsed
	}
	return time.Time{}
}
