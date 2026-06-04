package wishitems

import (
	"context"
	"net/url"
	"strconv"

	"github.com/yota/ohey/backend/internal/contracts"
	"github.com/yota/ohey/backend/internal/supabase"
)

const wishItemSelectColumns = "id,owner_user_id,title,note,category,place_text,place_url,visibility,status,created_at,updated_at"

type SupabaseRepository struct {
	client *supabase.Client
}

func NewSupabaseRepository(client *supabase.Client) *SupabaseRepository {
	return &SupabaseRepository{client: client}
}

func (r *SupabaseRepository) ListWishItems(ctx context.Context, authToken, ownerUserID string, limit int) ([]map[string]any, error) {
	q := url.Values{}
	q.Set("select", wishItemSelectColumns)
	q.Set("owner_user_id", "eq."+ownerUserID)
	q.Set("status", supabase.PostgRESTEq(contracts.StatusActive))
	q.Set("order", "created_at.desc")
	q.Set("limit", strconv.Itoa(limit))
	var rows []map[string]any
	if err := r.client.Get(ctx, authToken, "wish_items", q, &rows); err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *SupabaseRepository) ListProfileWishItems(ctx context.Context, authToken, profileID string, limit int) ([]map[string]any, error) {
	q := url.Values{}
	q.Set("select", wishItemSelectColumns)
	q.Set("owner_user_id", "eq."+profileID)
	q.Set("visibility", supabase.PostgRESTEq(contracts.VisibilityFriends))
	q.Set("status", supabase.PostgRESTEq(contracts.StatusActive))
	q.Set("order", "created_at.desc")
	q.Set("limit", strconv.Itoa(limit))
	var rows []map[string]any
	if err := r.client.Get(ctx, authToken, "wish_items", q, &rows); err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *SupabaseRepository) CreateWishItem(ctx context.Context, authToken string, item WishItem) (map[string]any, error) {
	payload := map[string]any{
		"owner_user_id": item.OwnerUserID,
		"title":         item.Title,
		"note":          item.Note,
		"category":      item.Category,
		"place_text":    item.PlaceText,
		"place_url":     item.PlaceURL,
		"visibility":    item.Visibility,
	}
	var rows []map[string]any
	if err := r.client.Post(ctx, authToken, "wish_items", nil, payload, &rows); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, UserError{Kind: ErrorKindUpstream, Message: "wish item insert returned no rows"}
	}
	return rows[0], nil
}

func (r *SupabaseRepository) UpdateWishItem(ctx context.Context, authToken string, update WishItemUpdate) (map[string]any, error) {
	q := url.Values{}
	q.Set("id", "eq."+update.WishItemID)
	q.Set("owner_user_id", "eq."+update.OwnerUserID)
	payload := map[string]any{
		"title":      update.Title,
		"note":       update.Note,
		"category":   update.Category,
		"place_text": update.PlaceText,
		"place_url":  update.PlaceURL,
		"visibility": update.Visibility,
	}
	var rows []map[string]any
	if err := r.client.Patch(ctx, authToken, "wish_items", q, payload, &rows); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return rows[0], nil
}

func (r *SupabaseRepository) DeleteWishItem(ctx context.Context, authToken, wishItemID, ownerUserID string) (map[string]any, error) {
	q := url.Values{}
	q.Set("id", "eq."+wishItemID)
	q.Set("owner_user_id", "eq."+ownerUserID)
	var rows []map[string]any
	if err := r.client.Delete(ctx, authToken, "wish_items", q, &rows); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return rows[0], nil
}
