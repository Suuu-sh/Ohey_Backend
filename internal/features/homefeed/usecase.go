package homefeed

import (
	"context"
	"sort"
	"time"
)

type Dependencies struct {
	Repository Repository
	Now        func() time.Time
}

type Usecase struct {
	repository Repository
	now        func() time.Time
}

func NewUsecase(deps Dependencies) *Usecase {
	now := deps.Now
	if now == nil {
		now = time.Now
	}
	return &Usecase{repository: deps.Repository, now: now}
}

type ListInput struct {
	AuthToken string
	UserID    string
	Limit     string
	Cursor    string
}

func (u *Usecase) ListHomeFeed(ctx context.Context, input ListInput) ([]map[string]any, error) {
	userID, err := CleanUUID(input.UserID, "user id")
	if err != nil {
		return nil, err
	}
	limit, err := CleanLimit(input.Limit, 50, 100)
	if err != nil {
		return nil, err
	}
	cursor, err := CleanCursor(input.Cursor)
	if err != nil {
		return nil, err
	}
	// The client asks for a small page (usually 20 items), but filtering below can
	// remove reported/hidden/muted rows. Fetch a bounded over-read from Supabase so
	// the DB never returns every memory for large friend graphs.
	fetchLimit := feedFetchLimit(limit)
	// Convert the feed cursor back into a timestamp and push it down to PostgREST.
	// This keeps later pages from re-scanning newer memories that were already seen.
	before := cursorBefore(cursor)
	visibleUserIDs, err := u.repository.VisibleFeedUserIDs(ctx, input.AuthToken, userID)
	if err != nil {
		return nil, err
	}
	hiddenIDs, err := u.repository.HiddenMemoryIDs(ctx, input.AuthToken, userID)
	if err != nil {
		return nil, err
	}
	hiddenUserIDs, err := u.repository.HiddenUserIDs(ctx, input.AuthToken, userID)
	if err != nil {
		return nil, err
	}
	visibleUserIDs = ExcludeHiddenUserIDs(visibleUserIDs, hiddenUserIDs)
	rows, err := u.repository.ListMemories(ctx, input.AuthToken, visibleUserIDs, fetchLimit, before)
	if err != nil {
		return nil, err
	}
	officialRows, err := u.repository.ListOfficialMemories(ctx, input.AuthToken, fetchLimit, before)
	if err != nil {
		return nil, err
	}
	rows = appendUniqueRows(rows, officialRows...)
	attachLikeState(rows, userID)
	rows = HideReportedRows(rows, hiddenIDs)
	rows = HideRowsByOwner(rows, hiddenUserIDs)
	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		item, ok := BuildFeedItem(row, userID)
		if !ok {
			continue
		}
		items = append(items, AttachFeedItem(row, item))
	}
	sort.SliceStable(items, func(i, j int) bool {
		iScore := int64Value(items[i], "rank_score")
		jScore := int64Value(items[j], "rank_score")
		if iScore != jScore {
			return iScore > jScore
		}
		if !RowTime(items[i]).Equal(RowTime(items[j])) {
			return RowTime(items[i]).After(RowTime(items[j]))
		}
		return stringValue(items[i], "id") < stringValue(items[j], "id")
	})
	items = applyCursor(items, cursor)
	if len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func feedFetchLimit(limit int) int {
	// Over-read by 5x to leave room for safety filters, but cap the DB result set
	// to protect backend JSON decoding and Supabase transfer as data grows.
	fetchLimit := limit * 5
	if fetchLimit < 100 {
		fetchLimit = 100
	}
	if fetchLimit > 300 {
		fetchLimit = 300
	}
	return fetchLimit
}

func cursorBefore(cursor string) time.Time {
	// rank_score is now just happened_at.Unix(), so legacy rank_score cursors can
	// be safely converted to a time boundary for DB-side pagination.
	parsed, ok := ParseCursor(cursor)
	if !ok {
		return time.Time{}
	}
	if !parsed.SortAt.IsZero() {
		return parsed.SortAt
	}
	if parsed.RankScore > 0 {
		return time.Unix(parsed.RankScore, 0).UTC()
	}
	return time.Time{}
}

func applyCursor(items []map[string]any, cursor string) []map[string]any {
	if cursor == "" || len(items) == 0 {
		return items
	}
	parsed, ok := ParseCursor(cursor)
	if !ok {
		return items
	}
	if !parsed.SortAt.IsZero() {
		filtered := make([]map[string]any, 0, len(items))
		for _, item := range items {
			if RowTime(item).Before(parsed.SortAt) {
				filtered = append(filtered, item)
			}
		}
		return filtered
	}
	filtered := make([]map[string]any, 0, len(items))
	for _, item := range items {
		score := int64Value(item, "rank_score")
		id := stringValue(item, "id")
		if score < parsed.RankScore || (score == parsed.RankScore && id > parsed.ID) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func appendUniqueRows(rows []map[string]any, extraRows ...map[string]any) []map[string]any {
	seen := make(map[string]bool, len(rows)+len(extraRows))
	for _, row := range rows {
		if id, _ := row["id"].(string); id != "" {
			seen[id] = true
		}
	}
	for _, row := range extraRows {
		id, _ := row["id"].(string)
		if id != "" && seen[id] {
			continue
		}
		if id != "" {
			seen[id] = true
		}
		rows = append(rows, row)
	}
	return rows
}

func attachLikeState(rows []map[string]any, userID string) {
	for _, row := range rows {
		rawLikes, _ := row["memory_likes"].([]any)
		row["like_count"] = len(rawLikes)
		likedByMe := false
		for _, rawLike := range rawLikes {
			like, ok := rawLike.(map[string]any)
			if ok && like["user_id"] == userID {
				likedByMe = true
				break
			}
		}
		row["liked_by_me"] = likedByMe
	}
}

func int64Value(row map[string]any, key string) int64 {
	switch v := row[key].(type) {
	case int64:
		return v
	case int:
		return int64(v)
	case int32:
		return int64(v)
	case float64:
		return int64(v)
	case float32:
		return int64(v)
	default:
		return 0
	}
}
