package profiles

import (
	"context"
	"net/url"

	"github.com/yota/ohey/backend/internal/supabase"
)

const profileSelectColumns = "id,user_id,display_name,character_key,avatar_url,is_plus"

type SupabaseRepository struct {
	client *supabase.Client
}

func NewSupabaseRepository(client *supabase.Client) *SupabaseRepository {
	return &SupabaseRepository{client: client}
}

func (r *SupabaseRepository) GetByID(ctx context.Context, authToken, authUserID string) (*Profile, error) {
	q := url.Values{}
	q.Set("select", profileSelectColumns)
	q.Set("id", "eq."+authUserID)
	q.Set("limit", "1")
	var rows []Profile
	if err := r.client.Get(ctx, authToken, "profiles", q, &rows); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return &rows[0], nil
}

func (r *SupabaseRepository) GetByUserID(ctx context.Context, authToken, userID string) (*Profile, error) {
	q := url.Values{}
	q.Set("select", profileSelectColumns)
	q.Set("user_id", "eq."+userID)
	q.Set("limit", "1")
	var rows []Profile
	if err := r.client.Get(ctx, authToken, "profiles", q, &rows); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return &rows[0], nil
}

func (r *SupabaseRepository) UpsertBootstrap(ctx context.Context, authToken string, payload map[string]any) (map[string]any, error) {
	q := url.Values{}
	q.Set("on_conflict", "id")
	var rows []map[string]any
	if err := r.client.Upsert(ctx, authToken, "profiles", q, payload, &rows); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return payload, nil
	}
	return rows[0], nil
}

func (r *SupabaseRepository) PatchByID(ctx context.Context, authToken, authUserID string, payload map[string]any) ([]Profile, error) {
	q := url.Values{}
	q.Set("id", "eq."+authUserID)
	var rows []Profile
	if err := r.client.Patch(ctx, authToken, "profiles", q, payload, &rows); err != nil {
		return nil, err
	}
	return rows, nil
}
