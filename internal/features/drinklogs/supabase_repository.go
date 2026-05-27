package drinklogs

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/yota/nomo/backend/internal/supabase"
)

const drinkLogSelectColumns = "id,owner_user_id,drank_at,place_name,place_lat,place_lng,memo,caption_y,photo_path,link_url,marker_rarity,is_official,owner:profiles!drink_logs_owner_user_id_fkey(id,user_id,display_name,gender,character_key,avatar_url,is_plus),drink_log_likes(user_id),drink_log_friends(profiles(id,user_id,display_name,gender,character_key,avatar_url,is_plus))"

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

func (r *SupabaseRepository) ListDrinkLogs(ctx context.Context, authToken string, ownerUserIDs []string) ([]map[string]any, error) {
	q := url.Values{}
	q.Set("select", drinkLogSelectColumns)
	q.Set("owner_user_id", "in.("+strings.Join(ownerUserIDs, ",")+")")
	q.Set("order", "drank_at.desc")
	var rows []map[string]any
	if err := r.client.Get(ctx, authToken, "drink_logs", q, &rows); err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *SupabaseRepository) ListOfficialDrinkLogs(ctx context.Context, authToken string) ([]map[string]any, error) {
	q := url.Values{}
	q.Set("select", drinkLogSelectColumns)
	q.Set("is_official", "eq.true")
	q.Set("order", "drank_at.desc")
	var rows []map[string]any
	if err := r.client.Get(ctx, authToken, "drink_logs", q, &rows); err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *SupabaseRepository) HasDrinkLogInWindow(ctx context.Context, authToken, ownerUserID string, start, end time.Time) (bool, error) {
	q := url.Values{}
	q.Set("select", "id")
	q.Set("owner_user_id", "eq."+ownerUserID)
	q.Set("is_official", "eq.false")
	q.Add("drank_at", "gte."+start.Format(time.RFC3339))
	q.Add("drank_at", "lt."+end.Format(time.RFC3339))
	q.Set("limit", "1")
	var rows []map[string]any
	if err := r.client.Get(ctx, authToken, "drink_logs", q, &rows); err != nil {
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

func (r *SupabaseRepository) CreateDrinkLog(ctx context.Context, authToken string, log NewDrinkLog) (map[string]any, error) {
	payload := map[string]any{
		"owner_user_id": log.OwnerUserID,
		"drank_at":      log.DrankAt.Format(time.RFC3339),
		"place_name":    strings.TrimSpace(log.PlaceName),
		"place_lat":     log.PlaceLat,
		"place_lng":     log.PlaceLng,
		"memo":          strings.TrimSpace(log.Memo),
		"caption_y":     log.CaptionY,
		"photo_path":    log.PhotoPath,
		"marker_rarity": string(log.MarkerRarity),
		"is_official":   log.IsOfficial,
	}
	var rows []map[string]any
	if err := r.client.Post(ctx, authToken, "drink_logs", nil, payload, &rows); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, UserError{Kind: ErrorKindUpstream, Message: "drink log insert returned no rows"}
	}
	return rows[0], nil
}

func (r *SupabaseRepository) CreateDrinkLogFriendLinks(ctx context.Context, authToken, drinkLogID string, friendIDs []string) error {
	if len(friendIDs) == 0 {
		return nil
	}
	links := make([]map[string]string, 0, len(friendIDs))
	for _, id := range friendIDs {
		if id != "" {
			links = append(links, map[string]string{"drink_log_id": drinkLogID, "friend_user_id": id})
		}
	}
	var ignored []map[string]any
	return r.client.Post(ctx, authToken, "drink_log_friends", nil, links, &ignored)
}

func (r *SupabaseRepository) DeleteOwnedDrinkLog(ctx context.Context, authToken, logID, ownerUserID string) (map[string]any, error) {
	q := url.Values{}
	q.Set("id", "eq."+logID)
	q.Set("owner_user_id", "eq."+ownerUserID)
	var rows []map[string]any
	if err := r.client.Delete(ctx, authToken, "drink_logs", q, &rows); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return rows[0], nil
}

func (r *SupabaseRepository) CreateLike(ctx context.Context, authToken, logID, userID string) (bool, error) {
	payload := map[string]any{"drink_log_id": logID, "user_id": userID}
	var ignored []map[string]any
	if err := r.client.Post(ctx, authToken, "drink_log_likes", nil, payload, &ignored); err != nil {
		var apiErr supabase.APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusConflict {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (r *SupabaseRepository) DeleteLike(ctx context.Context, authToken, logID, userID string) error {
	q := url.Values{}
	q.Set("drink_log_id", "eq."+logID)
	q.Set("user_id", "eq."+userID)
	var ignored []map[string]any
	return r.client.Delete(ctx, authToken, "drink_log_likes", q, &ignored)
}

func (r *SupabaseRepository) LikeState(ctx context.Context, authToken, logID, userID string) (LikeState, error) {
	q := url.Values{}
	q.Set("select", "user_id")
	q.Set("drink_log_id", "eq."+logID)
	var likes []map[string]any
	if err := r.client.Get(ctx, authToken, "drink_log_likes", q, &likes); err != nil {
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

func (r *SupabaseRepository) HiddenDrinkLogIDs(ctx context.Context, authToken, userID string) (map[string]bool, error) {
	q := url.Values{}
	q.Set("select", "drink_log_id")
	q.Set("reporter_user_id", "eq."+userID)
	var rows []map[string]any
	if err := r.client.Get(ctx, authToken, "drink_log_reports", q, &rows); err != nil {
		return nil, err
	}
	hidden := make(map[string]bool, len(rows))
	for _, row := range rows {
		id, _ := row["drink_log_id"].(string)
		if id != "" {
			hidden[id] = true
		}
	}
	return hidden, nil
}

func (r *SupabaseRepository) DrinkLogOwnerUserID(ctx context.Context, authToken, logID string) (string, error) {
	q := url.Values{}
	q.Set("select", "owner_user_id")
	q.Set("id", "eq."+logID)
	q.Set("limit", "1")
	var rows []map[string]any
	if err := r.client.Get(ctx, authToken, "drink_logs", q, &rows); err != nil {
		return "", err
	}
	if len(rows) == 0 {
		return "", nil
	}
	ownerUserID, _ := rows[0]["owner_user_id"].(string)
	return ownerUserID, nil
}

func (r *SupabaseRepository) FindReport(ctx context.Context, authToken, logID, reporterUserID string) (*Report, error) {
	q := url.Values{}
	q.Set("select", "id,drink_log_id,reporter_user_id,reason")
	q.Set("drink_log_id", "eq."+logID)
	q.Set("reporter_user_id", "eq."+reporterUserID)
	q.Set("limit", "1")
	var rows []map[string]any
	if err := r.client.Get(ctx, authToken, "drink_log_reports", q, &rows); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	row := rows[0]
	reason, _ := CleanReportReason(stringValue(row, "reason"))
	return &Report{
		ID:             stringValue(row, "id"),
		DrinkLogID:     stringValue(row, "drink_log_id"),
		ReporterUserID: stringValue(row, "reporter_user_id"),
		Reason:         reason,
		Status:         ModerationStatusPending,
	}, nil
}

func (r *SupabaseRepository) CreateReport(ctx context.Context, authToken string, report Report) error {
	payload := map[string]any{"drink_log_id": report.DrinkLogID, "reporter_user_id": report.ReporterUserID, "reason": string(report.Reason)}
	var rows []map[string]any
	if err := r.client.Post(ctx, authToken, "drink_log_reports", nil, payload, &rows); err != nil {
		var apiErr supabase.APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusConflict {
			return nil
		}
		return err
	}
	return nil
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
		rawLikes, _ := row["drink_log_likes"].([]any)
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

func SortRowsByDrankAtDesc(rows []map[string]any) {
	sort.SliceStable(rows, func(i, j int) bool {
		return rowTime(rows[i]).After(rowTime(rows[j]))
	})
}

func rowTime(row map[string]any) time.Time {
	value, _ := row["drank_at"].(string)
	parsed, err := time.Parse(time.RFC3339, value)
	if err == nil {
		return parsed
	}
	return time.Time{}
}
