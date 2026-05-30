package memories

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/yota/ohey/backend/internal/supabase"
)

const memorySelectColumns = "id,owner_user_id,happened_at,place_name,place_lat,place_lng,memo,caption_y,photo_path,link_url,marker_rarity,is_official,owner:profiles!memories_owner_user_id_fkey(id,user_id,display_name,gender,character_key,avatar_url,is_plus),memory_likes(user_id),memory_tagged_users(profiles(id,user_id,display_name,gender,character_key,avatar_url,is_plus))"

type SupabaseRepository struct {
	client *supabase.Client
}

func NewSupabaseRepository(client *supabase.Client) *SupabaseRepository {
	return &SupabaseRepository{client: client}
}

func (r *SupabaseRepository) VisibleFeedUserIDs(ctx context.Context, authToken, userID string) ([]string, error) {
	q := url.Values{}
	q.Set("select", "user_a_id,user_b_id")
	q.Set("or", "(user_a_id.eq."+userID+",user_b_id.eq."+userID+")")
	var friendships []map[string]any
	if err := r.client.Get(ctx, authToken, "friendships", q, &friendships); err != nil {
		return nil, err
	}
	seen := map[string]bool{userID: true}
	ids := []string{userID}
	for _, friendship := range friendships {
		for _, key := range []string{"user_a_id", "user_b_id"} {
			id, ok := friendship[key].(string)
			if ok && id != "" && !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
		}
	}
	return ids, nil
}

func (r *SupabaseRepository) ListMemories(ctx context.Context, authToken string, ownerUserIDs []string) ([]map[string]any, error) {
	if len(ownerUserIDs) == 0 {
		return []map[string]any{}, nil
	}
	q := url.Values{}
	q.Set("select", memorySelectColumns)
	q.Set("owner_user_id", "in.("+strings.Join(ownerUserIDs, ",")+")")
	q.Set("order", "happened_at.desc")
	var rows []map[string]any
	if err := r.client.Get(ctx, authToken, "memories", q, &rows); err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *SupabaseRepository) ListOfficialMemories(ctx context.Context, authToken string) ([]map[string]any, error) {
	q := url.Values{}
	q.Set("select", memorySelectColumns)
	q.Set("is_official", "eq.true")
	q.Set("order", "happened_at.desc")
	var rows []map[string]any
	if err := r.client.Get(ctx, authToken, "memories", q, &rows); err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *SupabaseRepository) HasMemoryInWindow(ctx context.Context, authToken, ownerUserID string, start, end time.Time) (bool, error) {
	q := url.Values{}
	q.Set("select", "id")
	q.Set("owner_user_id", "eq."+ownerUserID)
	q.Set("is_official", "eq.false")
	q.Add("happened_at", "gte."+start.Format(time.RFC3339))
	q.Add("happened_at", "lt."+end.Format(time.RFC3339))
	q.Set("limit", "1")
	var rows []map[string]any
	if err := r.client.Get(ctx, authToken, "memories", q, &rows); err != nil {
		return false, err
	}
	return len(rows) > 0, nil
}

func (r *SupabaseRepository) FriendshipExists(ctx context.Context, authToken, userID, friendID string) (bool, error) {
	q := url.Values{}
	q.Set("select", "id")
	q.Set("or", "(and(user_a_id.eq."+userID+",user_b_id.eq."+friendID+"),and(user_a_id.eq."+friendID+",user_b_id.eq."+userID+"))")
	q.Set("limit", "1")
	var rows []map[string]any
	if err := r.client.Get(ctx, authToken, "friendships", q, &rows); err != nil {
		return false, err
	}
	return len(rows) > 0, nil
}

func (r *SupabaseRepository) CreateMemory(ctx context.Context, authToken string, memory NewMemory) (map[string]any, error) {
	payload := map[string]any{
		"owner_user_id": memory.OwnerUserID,
		"happened_at":   memory.HappenedAt.Format(time.RFC3339),
		"place_name":    strings.TrimSpace(memory.PlaceName),
		"place_lat":     memory.PlaceLat,
		"place_lng":     memory.PlaceLng,
		"memo":          strings.TrimSpace(memory.Memo),
		"caption_y":     memory.CaptionY,
		"photo_path":    memory.PhotoPath,
		"marker_rarity": string(memory.MarkerRarity),
		"is_official":   memory.IsOfficial,
	}
	var rows []map[string]any
	if err := r.client.Post(ctx, authToken, "memories", nil, payload, &rows); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, UserError{Kind: ErrorKindUpstream, Message: "memory insert returned no rows"}
	}
	return rows[0], nil
}

func (r *SupabaseRepository) CreateMemoryFriendLinks(ctx context.Context, authToken, memoryID string, friendIDs []string) error {
	if len(friendIDs) == 0 {
		return nil
	}
	links := make([]map[string]string, 0, len(friendIDs))
	for _, id := range friendIDs {
		if id != "" {
			links = append(links, map[string]string{"memory_id": memoryID, "tagged_user_id": id})
		}
	}
	var ignored []map[string]any
	return r.client.Post(ctx, authToken, "memory_tagged_users", nil, links, &ignored)
}

func (r *SupabaseRepository) DeleteOwnedMemory(ctx context.Context, authToken, memoryID, ownerUserID string) (map[string]any, error) {
	q := url.Values{}
	q.Set("id", "eq."+memoryID)
	q.Set("owner_user_id", "eq."+ownerUserID)
	var rows []map[string]any
	if err := r.client.Delete(ctx, authToken, "memories", q, &rows); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return rows[0], nil
}

func (r *SupabaseRepository) CreateLike(ctx context.Context, authToken, memoryID, userID string) (bool, error) {
	payload := map[string]any{"memory_id": memoryID, "user_id": userID}
	var ignored []map[string]any
	if err := r.client.Post(ctx, authToken, "memory_likes", nil, payload, &ignored); err != nil {
		var apiErr supabase.APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusConflict {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (r *SupabaseRepository) DeleteLike(ctx context.Context, authToken, memoryID, userID string) error {
	q := url.Values{}
	q.Set("memory_id", "eq."+memoryID)
	q.Set("user_id", "eq."+userID)
	var ignored []map[string]any
	return r.client.Delete(ctx, authToken, "memory_likes", q, &ignored)
}

func (r *SupabaseRepository) LikeState(ctx context.Context, authToken, memoryID, userID string) (LikeState, error) {
	q := url.Values{}
	q.Set("select", "user_id")
	q.Set("memory_id", "eq."+memoryID)
	var likes []map[string]any
	if err := r.client.Get(ctx, authToken, "memory_likes", q, &likes); err != nil {
		return LikeState{}, err
	}
	likedByMe := false
	for _, like := range likes {
		if like["user_id"] == userID {
			likedByMe = true
			break
		}
	}
	return LikeState{LikeCount: len(likes), LikedByMe: likedByMe}, nil
}

func (r *SupabaseRepository) HiddenMemoryIDs(ctx context.Context, authToken, userID string) (map[string]bool, error) {
	q := url.Values{}
	q.Set("select", "memory_id")
	q.Set("reporter_user_id", "eq."+userID)
	var rows []map[string]any
	if err := r.client.Get(ctx, authToken, "memory_reports", q, &rows); err != nil {
		return nil, err
	}
	hidden := make(map[string]bool, len(rows))
	for _, row := range rows {
		id, _ := row["memory_id"].(string)
		if id != "" {
			hidden[id] = true
		}
	}
	q = url.Values{}
	q.Set("select", "memory_id")
	q.Set("user_id", "eq."+userID)
	var feedHiddenRows []map[string]any
	if err := r.client.Get(ctx, authToken, "memory_hides", q, &feedHiddenRows); err != nil {
		if isOptionalSafetyTableMissing(err) {
			return hidden, nil
		}
		return nil, err
	}
	for _, row := range feedHiddenRows {
		id, _ := row["memory_id"].(string)
		if id != "" {
			hidden[id] = true
		}
	}
	return hidden, nil
}

func (r *SupabaseRepository) HiddenUserIDs(ctx context.Context, authToken, userID string) (map[string]bool, error) {
	hidden := map[string]bool{}
	q := url.Values{}
	q.Set("select", "blocked_user_id")
	q.Set("blocker_user_id", "eq."+userID)
	var blockRows []map[string]any
	if err := r.client.Get(ctx, authToken, "user_blocks", q, &blockRows); err != nil {
		if isOptionalSafetyTableMissing(err) {
			return hidden, nil
		}
		return nil, err
	}
	for _, row := range blockRows {
		id, _ := row["blocked_user_id"].(string)
		if id != "" {
			hidden[id] = true
		}
	}
	q = url.Values{}
	q.Set("select", "muted_user_id")
	q.Set("muter_user_id", "eq."+userID)
	var muteRows []map[string]any
	if err := r.client.Get(ctx, authToken, "user_mutes", q, &muteRows); err != nil {
		if isOptionalSafetyTableMissing(err) {
			return hidden, nil
		}
		return nil, err
	}
	for _, row := range muteRows {
		id, _ := row["muted_user_id"].(string)
		if id != "" {
			hidden[id] = true
		}
	}
	return hidden, nil
}

func (r *SupabaseRepository) MemoryOwnerUserID(ctx context.Context, authToken, memoryID string) (string, error) {
	q := url.Values{}
	q.Set("select", "owner_user_id")
	q.Set("id", "eq."+memoryID)
	q.Set("limit", "1")
	var rows []map[string]any
	if err := r.client.Get(ctx, authToken, "memories", q, &rows); err != nil {
		return "", err
	}
	if len(rows) == 0 {
		return "", nil
	}
	ownerUserID, _ := rows[0]["owner_user_id"].(string)
	return ownerUserID, nil
}

func (r *SupabaseRepository) FindReport(ctx context.Context, authToken, memoryID, reporterUserID string) (*Report, error) {
	q := url.Values{}
	q.Set("select", "id,memory_id,reporter_user_id,reason")
	q.Set("memory_id", "eq."+memoryID)
	q.Set("reporter_user_id", "eq."+reporterUserID)
	q.Set("limit", "1")
	var rows []map[string]any
	if err := r.client.Get(ctx, authToken, "memory_reports", q, &rows); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	row := rows[0]
	reason, _ := CleanReportReason(stringValue(row, "reason"))
	return &Report{
		ID:             stringValue(row, "id"),
		MemoryID:       stringValue(row, "memory_id"),
		ReporterUserID: stringValue(row, "reporter_user_id"),
		Reason:         reason,
		Status:         ModerationStatusPending,
	}, nil
}

func (r *SupabaseRepository) CreateReport(ctx context.Context, authToken string, report Report) error {
	payload := map[string]any{"memory_id": report.MemoryID, "reporter_user_id": report.ReporterUserID, "reason": string(report.Reason)}
	var rows []map[string]any
	if err := r.client.Post(ctx, authToken, "memory_reports", nil, payload, &rows); err != nil {
		var apiErr supabase.APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusConflict {
			return nil
		}
		return err
	}
	return nil
}

func isOptionalSafetyTableMissing(err error) bool {
	var apiErr supabase.APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	if apiErr.StatusCode == http.StatusNotFound {
		return true
	}
	if apiErr.StatusCode == http.StatusBadRequest && strings.Contains(apiErr.Body, "does not exist") {
		return true
	}
	return false
}

func stringValue(row map[string]any, key string) string {
	value, _ := row[key].(string)
	return value
}

func HideRowsByID(rows []map[string]any, hiddenIDs map[string]bool) []map[string]any {
	if len(hiddenIDs) == 0 {
		return rows
	}
	filtered := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		id, _ := row["id"].(string)
		if id != "" && hiddenIDs[id] {
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
		ownerUserID, _ := row["owner_user_id"].(string)
		isOfficial, _ := row["is_official"].(bool)
		if ownerUserID != "" && hiddenUserIDs[ownerUserID] && !isOfficial {
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

func AppendUniqueRows(rows []map[string]any, extraRows ...map[string]any) []map[string]any {
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

func AttachLikeState(rows []map[string]any, userID string) {
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

func SortRowsByHappenedAtDesc(rows []map[string]any) {
	sort.SliceStable(rows, func(i, j int) bool {
		return rowTime(rows[i]).After(rowTime(rows[j]))
	})
}

func rowTime(row map[string]any) time.Time {
	value, _ := row["happened_at"].(string)
	parsed, err := time.Parse(time.RFC3339, value)
	if err == nil {
		return parsed
	}
	return time.Time{}
}
