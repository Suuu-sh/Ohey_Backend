package friendgroups

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/yota/nomo/backend/internal/supabase"
)

type SupabaseRepository struct {
	client *supabase.Client
}

func NewSupabaseRepository(client *supabase.Client) *SupabaseRepository {
	return &SupabaseRepository{client: client}
}

func (r *SupabaseRepository) ListGroups(ctx context.Context, authToken, ownerUserID string) ([]FriendGroup, error) {
	q := url.Values{}
	q.Set("select", "id,client_id,name,sort_order,friend_group_members(friend_user_id,sort_order)")
	q.Set("owner_user_id", "eq."+ownerUserID)
	q.Set("order", "sort_order.asc")
	var rows []map[string]any
	if err := r.client.Get(ctx, authToken, "friend_groups", q, &rows); err != nil {
		if isMissingTable(err) {
			return []FriendGroup{}, nil
		}
		return nil, err
	}
	groups := make([]FriendGroup, 0, len(rows))
	for _, row := range rows {
		groups = append(groups, groupFromRow(row))
	}
	return groups, nil
}

func (r *SupabaseRepository) FriendshipExists(ctx context.Context, authToken, ownerUserID, friendUserID string) (bool, error) {
	q := url.Values{}
	q.Set("select", "id")
	q.Set("or", "(and(user_a_id.eq."+ownerUserID+",user_b_id.eq."+friendUserID+"),and(user_a_id.eq."+friendUserID+",user_b_id.eq."+ownerUserID+"))")
	q.Set("limit", "1")
	var rows []map[string]any
	if err := r.client.Get(ctx, authToken, "friendships", q, &rows); err != nil {
		return false, err
	}
	return len(rows) > 0, nil
}

func (r *SupabaseRepository) SaveGroups(ctx context.Context, authToken, ownerUserID string, groups []FriendGroup) ([]FriendGroup, error) {
	existing, err := r.listGroupIDs(ctx, authToken, ownerUserID)
	if err != nil {
		if isMissingTable(err) {
			return groups, nil
		}
		return nil, err
	}
	keep := map[string]bool{}
	for _, group := range groups {
		keep[group.ID] = true
	}
	for clientID, rowID := range existing {
		if keep[clientID] {
			continue
		}
		q := url.Values{}
		q.Set("id", "eq."+rowID)
		var ignored []map[string]any
		if err := r.client.Delete(ctx, authToken, "friend_groups", q, &ignored); err != nil {
			return nil, err
		}
	}
	for _, group := range groups {
		rowID, err := r.upsertGroup(ctx, authToken, ownerUserID, group)
		if err != nil {
			return nil, err
		}
		if err := r.replaceMembers(ctx, authToken, rowID, group.FriendIDs); err != nil {
			return nil, err
		}
	}
	return r.ListGroups(ctx, authToken, ownerUserID)
}

func (r *SupabaseRepository) listGroupIDs(ctx context.Context, authToken, ownerUserID string) (map[string]string, error) {
	q := url.Values{}
	q.Set("select", "id,client_id")
	q.Set("owner_user_id", "eq."+ownerUserID)
	var rows []map[string]any
	if err := r.client.Get(ctx, authToken, "friend_groups", q, &rows); err != nil {
		return nil, err
	}
	ids := map[string]string{}
	for _, row := range rows {
		clientID, _ := row["client_id"].(string)
		id, _ := row["id"].(string)
		if clientID != "" && id != "" {
			ids[clientID] = id
		}
	}
	return ids, nil
}

func (r *SupabaseRepository) upsertGroup(ctx context.Context, authToken, ownerUserID string, group FriendGroup) (string, error) {
	q := url.Values{}
	q.Set("on_conflict", "owner_user_id,client_id")
	payload := map[string]any{"owner_user_id": ownerUserID, "client_id": group.ID, "name": group.Name, "sort_order": group.SortOrder}
	var rows []map[string]any
	if err := r.client.Upsert(ctx, authToken, "friend_groups", q, payload, &rows); err != nil {
		return "", err
	}
	if len(rows) == 0 {
		return "", UserError{Kind: ErrorKindInvalidInput, Message: "friend group upsert returned no rows"}
	}
	id, _ := rows[0]["id"].(string)
	if id == "" {
		return "", UserError{Kind: ErrorKindInvalidInput, Message: "friend group upsert returned no id"}
	}
	return id, nil
}

func (r *SupabaseRepository) replaceMembers(ctx context.Context, authToken, groupID string, friendIDs []string) error {
	q := url.Values{}
	q.Set("group_id", "eq."+groupID)
	var deleted []map[string]any
	if err := r.client.Delete(ctx, authToken, "friend_group_members", q, &deleted); err != nil {
		return err
	}
	if len(friendIDs) == 0 {
		return nil
	}
	rows := make([]map[string]any, 0, len(friendIDs))
	for index, friendID := range friendIDs {
		rows = append(rows, map[string]any{"group_id": groupID, "friend_user_id": friendID, "sort_order": index})
	}
	var ignored []map[string]any
	return r.client.Post(ctx, authToken, "friend_group_members", nil, rows, &ignored)
}

func groupFromRow(row map[string]any) FriendGroup {
	clientID, _ := row["client_id"].(string)
	name, _ := row["name"].(string)
	sortOrder := intFromAny(row["sort_order"])
	rawMembers, _ := row["friend_group_members"].([]any)
	type member struct {
		id    string
		order int
	}
	members := make([]member, 0, len(rawMembers))
	for _, raw := range rawMembers {
		m, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		id, _ := m["friend_user_id"].(string)
		if id == "" {
			continue
		}
		members = append(members, member{id: id, order: intFromAny(m["sort_order"])})
	}
	sort.SliceStable(members, func(i, j int) bool { return members[i].order < members[j].order })
	friendIDs := make([]string, 0, len(members))
	for _, member := range members {
		friendIDs = append(friendIDs, member.id)
	}
	return FriendGroup{ID: clientID, Name: strings.TrimSpace(name), FriendIDs: friendIDs, FriendIds: friendIDs, SortOrder: sortOrder}
}

func intFromAny(value any) int {
	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	default:
		return 0
	}
}

func isMissingTable(err error) bool {
	var apiErr supabase.APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	return apiErr.StatusCode == http.StatusNotFound || strings.Contains(apiErr.Body, "PGRST205") || strings.Contains(apiErr.Body, "friend_groups")
}
