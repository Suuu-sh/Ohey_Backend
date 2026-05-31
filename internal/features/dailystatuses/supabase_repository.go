package dailystatuses

import (
	"context"
	"net/url"

	"github.com/yota/ohey/backend/internal/supabase"
)

type SupabaseRepository struct {
	client *supabase.Client
}

func NewSupabaseRepository(client *supabase.Client) *SupabaseRepository {
	return &SupabaseRepository{client: client}
}

func (r *SupabaseRepository) GetDailyStatus(ctx context.Context, authToken, userID, statusDate string) ([]map[string]any, error) {
	q := url.Values{}
	q.Set("select", "user_id,status_date,status,updated_at")
	q.Set("user_id", "eq."+userID)
	q.Set("status_date", "eq."+statusDate)
	var rows []map[string]any
	if err := r.client.Get(ctx, authToken, "daily_statuses", q, &rows); err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *SupabaseRepository) ListMonthlyStatuses(ctx context.Context, authToken, userID, startDate, endDate string) ([]map[string]any, error) {
	q := url.Values{}
	q.Set("select", "user_id,status_date,status,updated_at")
	q.Set("user_id", "eq."+userID)
	q.Add("status_date", "gte."+startDate)
	q.Add("status_date", "lt."+endDate)
	q.Set("order", "status_date.asc")
	var rows []map[string]any
	if err := r.client.Get(ctx, authToken, "daily_statuses", q, &rows); err != nil {
		return nil, err
	}
	return rows, nil
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

func (r *SupabaseRepository) UpsertDailyStatus(ctx context.Context, authToken string, status DailyStatus) ([]map[string]any, error) {
	q := url.Values{}
	q.Set("on_conflict", "user_id,status_date")
	payload := map[string]any{"user_id": status.UserID, "status_date": status.StatusDate, "status": string(status.Status)}
	var rows []map[string]any
	if err := r.client.Upsert(ctx, authToken, "daily_statuses", q, payload, &rows); err != nil {
		return nil, err
	}
	return rows, nil
}
