package homefeed

import (
	"context"
	"net/url"
	"strings"

	"github.com/yota/nomo/backend/internal/supabase"
)

const homeFeedDrinkLogSelectColumns = "id,owner_user_id,drank_at,place_name,place_lat,place_lng,memo,caption_y,photo_path,link_url,marker_rarity,is_official,owner:profiles!drink_logs_owner_user_id_fkey(id,user_id,display_name,gender,character_key,avatar_url,is_plus),drink_log_likes(user_id),drink_log_friends(profiles(id,user_id,display_name,gender,character_key,avatar_url,is_plus))"

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

func (r *SupabaseRepository) ListDrinkLogs(ctx context.Context, authToken string, ownerUserIDs []string) ([]map[string]any, error) {
	if len(ownerUserIDs) == 0 {
		return []map[string]any{}, nil
	}
	q := url.Values{}
	q.Set("select", homeFeedDrinkLogSelectColumns)
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
	q.Set("select", homeFeedDrinkLogSelectColumns)
	q.Set("is_official", "eq.true")
	q.Set("order", "drank_at.desc")
	var rows []map[string]any
	if err := r.client.Get(ctx, authToken, "drink_logs", q, &rows); err != nil {
		return nil, err
	}
	return rows, nil
}
