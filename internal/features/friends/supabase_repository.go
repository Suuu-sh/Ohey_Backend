package friends

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/yota/ohey/backend/internal/contracts"
	"github.com/yota/ohey/backend/internal/supabase"
)

const friendshipSelectColumns = "user_a_id,user_b_id,is_favorite,user_a:profiles!friendships_user_a_id_fkey(id,user_id,display_name,character_key,avatar_url,is_plus),user_b:profiles!friendships_user_b_id_fkey(id,user_id,display_name,character_key,avatar_url,is_plus)"
const friendRequestSelectColumns = "id,from_user_id,to_user_id,status,created_at,responded_at,from_user:profiles!friend_requests_from_user_id_fkey(id,user_id,display_name,character_key,avatar_url,is_plus),to_user:profiles!friend_requests_to_user_id_fkey(id,user_id,display_name,character_key,avatar_url,is_plus)"

type SupabaseRepository struct {
	client         *supabase.Client
	adminClient    *supabase.Client
	serviceRoleKey string
}

func NewSupabaseRepository(client *supabase.Client, adminClient *supabase.Client, serviceRoleKey string) *SupabaseRepository {
	return &SupabaseRepository{client: client, adminClient: adminClient, serviceRoleKey: serviceRoleKey}
}

func (r *SupabaseRepository) ListFriendships(ctx context.Context, authToken, userID string) ([]map[string]any, error) {
	q := url.Values{}
	q.Set("select", friendshipSelectColumns)
	q.Set("or", "(user_a_id.eq."+userID+",user_b_id.eq."+userID+")")
	q.Set("order", "created_at.desc")
	var rows []map[string]any
	if err := r.client.Get(ctx, authToken, "friendships", q, &rows); err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *SupabaseRepository) AttachTodayStatuses(ctx context.Context, authToken string, rows []map[string]any, date string) error {
	profiles := map[string]map[string]any{}
	for _, row := range rows {
		for _, key := range []string{"user_a", "user_b"} {
			profile, ok := row[key].(map[string]any)
			if !ok {
				continue
			}
			id, _ := profile["id"].(string)
			if id != "" {
				profiles[id] = profile
			}
		}
	}
	if len(profiles) == 0 {
		return nil
	}
	profileIDs := make([]string, 0, len(profiles))
	for id := range profiles {
		profileIDs = append(profileIDs, id)
	}
	sort.Strings(profileIDs)
	q := url.Values{}
	q.Set("select", "user_id,status")
	q.Set("user_id", "in.("+strings.Join(profileIDs, ",")+")")
	q.Set("status_date", "eq."+date)
	var statuses []map[string]any
	if err := r.client.Get(ctx, authToken, "daily_statuses", q, &statuses); err != nil {
		return err
	}
	for _, status := range statuses {
		userID, _ := status["user_id"].(string)
		statusKey, _ := status["status"].(string)
		if profile := profiles[userID]; profile != nil {
			if strings.TrimSpace(statusKey) != "" {
				profile["status_key"] = statusKey
			}
		}
	}
	for _, profile := range profiles {
		if _, hasStatus := profile["status_key"]; hasStatus {
			continue
		}
		if status, ok := profile["status"].(string); ok && strings.TrimSpace(status) != "" {
			profile["status_key"] = status
		}
	}
	return nil
}

func (r *SupabaseRepository) UpdateFriendFavorite(ctx context.Context, authToken, userID, friendID string, isFavorite bool) (map[string]any, error) {
	q := url.Values{}
	q.Set("or", "(and(user_a_id.eq."+userID+",user_b_id.eq."+friendID+"),and(user_a_id.eq."+friendID+",user_b_id.eq."+userID+"))")
	var rows []map[string]any
	if err := r.client.Patch(ctx, authToken, "friendships", q, map[string]any{"is_favorite": isFavorite}, &rows); err != nil {
		return nil, err
	}
	return firstMap(rows), nil
}

func (r *SupabaseRepository) UpsertFriendshipPair(ctx context.Context, authToken, userA, userB string) (map[string]any, error) {
	first, second := OrderedPair(userA, userB)
	payload := map[string]any{"user_a_id": first, "user_b_id": second}
	q := url.Values{}
	q.Set("on_conflict", "user_a_id,user_b_id")
	var rows []map[string]any
	if err := r.client.Upsert(ctx, authToken, "friendships", q, payload, &rows); err != nil {
		return nil, err
	}
	if row := firstMap(rows); row != nil {
		return row, nil
	}
	return payload, nil
}

func (r *SupabaseRepository) DeleteFriendship(ctx context.Context, authToken, userID, friendID string) (map[string]any, error) {
	q := url.Values{}
	q.Set("or", "(and(user_a_id.eq."+userID+",user_b_id.eq."+friendID+"),and(user_a_id.eq."+friendID+",user_b_id.eq."+userID+"))")
	client := r.adminClient
	token := r.serviceRoleKey
	if client == nil || strings.TrimSpace(token) == "" {
		client = r.client
		token = authToken
	}
	var rows []map[string]any
	if err := client.Delete(ctx, token, "friendships", q, &rows); err != nil {
		return nil, err
	}
	return firstMap(rows), nil
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

func (r *SupabaseRepository) BlockExistsBetweenUsers(ctx context.Context, authToken, userID, friendID string) (bool, error) {
	q := url.Values{}
	q.Set("select", "blocker_user_id")
	q.Set("or", "(and(blocker_user_id.eq."+userID+",blocked_user_id.eq."+friendID+"),and(blocker_user_id.eq."+friendID+",blocked_user_id.eq."+userID+"))")
	q.Set("limit", "1")
	var rows []map[string]any
	if err := r.client.Get(ctx, authToken, "user_blocks", q, &rows); err != nil {
		if isOptionalSafetyTableMissing(err) {
			return false, nil
		}
		return false, err
	}
	return len(rows) > 0, nil
}

func (r *SupabaseRepository) ListPendingFriendRequests(ctx context.Context, authToken, userID string, direction RequestDirection) ([]map[string]any, error) {
	q := url.Values{}
	q.Set("select", friendRequestSelectColumns)
	q.Set("status", supabase.PostgRESTEq(contracts.StatusPending))
	switch direction {
	case RequestDirectionIncoming:
		q.Set("to_user_id", "eq."+userID)
	case RequestDirectionOutgoing:
		q.Set("from_user_id", "eq."+userID)
	default:
		q.Set("or", "(from_user_id.eq."+userID+",to_user_id.eq."+userID+")")
	}
	q.Set("order", "created_at.desc")
	var rows []map[string]any
	if err := r.client.Get(ctx, authToken, "friend_requests", q, &rows); err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *SupabaseRepository) PendingFriendRequestBetween(ctx context.Context, authToken, userID, friendID string) (map[string]any, error) {
	q := url.Values{}
	q.Set("select", "id,from_user_id,to_user_id")
	q.Set("status", supabase.PostgRESTEq(contracts.StatusPending))
	q.Set("or", "(and(from_user_id.eq."+userID+",to_user_id.eq."+friendID+"),and(from_user_id.eq."+friendID+",to_user_id.eq."+userID+"))")
	q.Set("limit", "1")
	var rows []map[string]any
	if err := r.client.Get(ctx, authToken, "friend_requests", q, &rows); err != nil {
		return nil, err
	}
	return firstMap(rows), nil
}

func (r *SupabaseRepository) CreateFriendRequest(ctx context.Context, authToken, fromUserID, toUserID string) (map[string]any, error) {
	payload := map[string]any{"from_user_id": fromUserID, "to_user_id": toUserID, "status": contracts.StatusPending}
	var rows []map[string]any
	if err := r.client.Post(ctx, authToken, "friend_requests", nil, payload, &rows); err != nil {
		return nil, err
	}
	if row := firstMap(rows); row != nil {
		return row, nil
	}
	return payload, nil
}

func (r *SupabaseRepository) UpdatePendingFriendRequestStatus(ctx context.Context, authToken, requestID, userID string, status RequestStatus, respondedAt time.Time) (map[string]any, error) {
	q := url.Values{}
	q.Set("id", "eq."+requestID)
	q.Set("status", supabase.PostgRESTEq(contracts.StatusPending))
	if status == RequestStatusCancelled {
		q.Set("from_user_id", "eq."+userID)
	} else {
		q.Set("to_user_id", "eq."+userID)
	}
	payload := map[string]any{"status": string(status), "responded_at": respondedAt.UTC().Format(time.RFC3339)}
	var rows []map[string]any
	if err := r.client.Patch(ctx, authToken, "friend_requests", q, payload, &rows); err != nil {
		return nil, err
	}
	return firstMap(rows), nil
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

func firstMap(rows []map[string]any) map[string]any {
	if len(rows) == 0 {
		return nil
	}
	return rows[0]
}
