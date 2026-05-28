package usersafety

import (
	"context"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/yota/nomo/backend/internal/supabase"
)

type SupabaseRepository struct {
	client         *supabase.Client
	adminClient    *supabase.Client
	serviceRoleKey string
}

func NewSupabaseRepository(client, adminClient *supabase.Client, serviceRoleKey string) *SupabaseRepository {
	return &SupabaseRepository{client: client, adminClient: adminClient, serviceRoleKey: serviceRoleKey}
}

func (r *SupabaseRepository) ListBlockedUsers(ctx context.Context, authToken, userID string) ([]map[string]any, error) {
	q := url.Values{}
	q.Set("select", "blocked_user_id,created_at")
	q.Set("blocker_user_id", "eq."+userID)
	q.Set("order", "created_at.desc")
	var rows []map[string]any
	if err := r.client.Get(ctx, authToken, "user_blocks", q, &rows); err != nil {
		return nil, err
	}
	return r.attachTargetProfiles(ctx, authToken, rows, "blocked_user_id")
}

func (r *SupabaseRepository) BlockUser(ctx context.Context, authToken string, relation UserRelation) (map[string]any, error) {
	payload := map[string]any{"blocker_user_id": relation.ActorUserID, "blocked_user_id": relation.TargetUserID}
	q := url.Values{}
	q.Set("on_conflict", "blocker_user_id,blocked_user_id")
	var rows []map[string]any
	if err := r.client.Upsert(ctx, authToken, "user_blocks", q, payload, &rows); err != nil {
		return nil, err
	}
	return firstMap(rows, payload), nil
}

func (r *SupabaseRepository) UnblockUser(ctx context.Context, authToken string, relation UserRelation) error {
	q := url.Values{}
	q.Set("blocker_user_id", "eq."+relation.ActorUserID)
	q.Set("blocked_user_id", "eq."+relation.TargetUserID)
	var ignored []map[string]any
	return r.client.Delete(ctx, authToken, "user_blocks", q, &ignored)
}

func (r *SupabaseRepository) ListMutedUsers(ctx context.Context, authToken, userID string) ([]map[string]any, error) {
	q := url.Values{}
	q.Set("select", "muted_user_id,created_at")
	q.Set("muter_user_id", "eq."+userID)
	q.Set("order", "created_at.desc")
	var rows []map[string]any
	if err := r.client.Get(ctx, authToken, "user_mutes", q, &rows); err != nil {
		return nil, err
	}
	return r.attachTargetProfiles(ctx, authToken, rows, "muted_user_id")
}

func (r *SupabaseRepository) MuteUser(ctx context.Context, authToken string, relation UserRelation) (map[string]any, error) {
	payload := map[string]any{"muter_user_id": relation.ActorUserID, "muted_user_id": relation.TargetUserID}
	q := url.Values{}
	q.Set("on_conflict", "muter_user_id,muted_user_id")
	var rows []map[string]any
	if err := r.client.Upsert(ctx, authToken, "user_mutes", q, payload, &rows); err != nil {
		return nil, err
	}
	return firstMap(rows, payload), nil
}

func (r *SupabaseRepository) UnmuteUser(ctx context.Context, authToken string, relation UserRelation) error {
	q := url.Values{}
	q.Set("muter_user_id", "eq."+relation.ActorUserID)
	q.Set("muted_user_id", "eq."+relation.TargetUserID)
	var ignored []map[string]any
	return r.client.Delete(ctx, authToken, "user_mutes", q, &ignored)
}

func (r *SupabaseRepository) HideDrinkLog(ctx context.Context, authToken string, hidden HiddenDrinkLog) (map[string]any, error) {
	payload := map[string]any{"user_id": hidden.UserID, "drink_log_id": hidden.DrinkLogID}
	q := url.Values{}
	q.Set("on_conflict", "user_id,drink_log_id")
	var rows []map[string]any
	if err := r.client.Upsert(ctx, authToken, "feed_hidden_drink_logs", q, payload, &rows); err != nil {
		return nil, err
	}
	return firstMap(rows, payload), nil
}

func (r *SupabaseRepository) UnhideDrinkLog(ctx context.Context, authToken string, hidden HiddenDrinkLog) error {
	q := url.Values{}
	q.Set("user_id", "eq."+hidden.UserID)
	q.Set("drink_log_id", "eq."+hidden.DrinkLogID)
	var ignored []map[string]any
	return r.client.Delete(ctx, authToken, "feed_hidden_drink_logs", q, &ignored)
}

func (r *SupabaseRepository) attachTargetProfiles(ctx context.Context, authToken string, relationRows []map[string]any, targetKey string) ([]map[string]any, error) {
	if len(relationRows) == 0 {
		return []map[string]any{}, nil
	}
	seen := map[string]bool{}
	ids := make([]string, 0, len(relationRows))
	for _, row := range relationRows {
		id, _ := row[targetKey].(string)
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return []map[string]any{}, nil
	}
	sortedIDs := append([]string(nil), ids...)
	sort.Strings(sortedIDs)
	q := url.Values{}
	q.Set("select", "id,user_id,display_name,gender,character_key,avatar_url,is_plus")
	q.Set("id", "in.("+strings.Join(sortedIDs, ",")+")")
	var profiles []map[string]any
	if err := r.client.Get(ctx, authToken, "profiles", q, &profiles); err != nil {
		return nil, err
	}
	profileByID := make(map[string]map[string]any, len(profiles))
	for _, profile := range profiles {
		id, _ := profile["id"].(string)
		if id != "" {
			profileByID[id] = profile
		}
	}
	out := make([]map[string]any, 0, len(relationRows))
	for _, row := range relationRows {
		id, _ := row[targetKey].(string)
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		profile := copyMap(profileByID[id])
		if profile == nil {
			profile = map[string]any{"id": id}
		}
		profile["target_user_id"] = id
		if createdAt, ok := row["created_at"]; ok {
			profile["created_at"] = createdAt
		}
		out = append(out, profile)
	}
	return out, nil
}

func (r *SupabaseRepository) CleanupBlockedRelations(ctx context.Context, relation UserRelation) error {
	if r.adminClient == nil || r.serviceRoleKey == "" {
		return nil
	}
	if err := r.deleteFriendship(ctx, relation); err != nil {
		return err
	}
	if err := r.closeFriendRequests(ctx, relation); err != nil {
		return err
	}
	return r.closeDrinkInvites(ctx, relation)
}

func (r *SupabaseRepository) deleteFriendship(ctx context.Context, relation UserRelation) error {
	q := url.Values{}
	q.Set("or", "(and(user_a_id.eq."+relation.ActorUserID+",user_b_id.eq."+relation.TargetUserID+"),and(user_a_id.eq."+relation.TargetUserID+",user_b_id.eq."+relation.ActorUserID+"))")
	var ignored []map[string]any
	return r.adminClient.Delete(ctx, r.serviceRoleKey, "friendships", q, &ignored)
}

func (r *SupabaseRepository) closeFriendRequests(ctx context.Context, relation UserRelation) error {
	respondedAt := time.Now().UTC().Format(time.RFC3339)
	outgoing := url.Values{}
	outgoing.Set("from_user_id", "eq."+relation.ActorUserID)
	outgoing.Set("to_user_id", "eq."+relation.TargetUserID)
	outgoing.Set("status", "eq.pending")
	var ignored []map[string]any
	if err := r.adminClient.Patch(ctx, r.serviceRoleKey, "friend_requests", outgoing, map[string]any{"status": "cancelled", "responded_at": respondedAt}, &ignored); err != nil {
		return err
	}
	incoming := url.Values{}
	incoming.Set("from_user_id", "eq."+relation.TargetUserID)
	incoming.Set("to_user_id", "eq."+relation.ActorUserID)
	incoming.Set("status", "eq.pending")
	return r.adminClient.Patch(ctx, r.serviceRoleKey, "friend_requests", incoming, map[string]any{"status": "rejected", "responded_at": respondedAt}, &ignored)
}

func (r *SupabaseRepository) closeDrinkInvites(ctx context.Context, relation UserRelation) error {
	respondedAt := time.Now().UTC().Format(time.RFC3339)
	outgoing := url.Values{}
	outgoing.Set("from_user_id", "eq."+relation.ActorUserID)
	outgoing.Set("to_user_id", "eq."+relation.TargetUserID)
	outgoing.Set("status", "eq.pending")
	var ignored []map[string]any
	if err := r.adminClient.Patch(ctx, r.serviceRoleKey, "drink_invites", outgoing, map[string]any{"status": "cancelled", "responded_at": respondedAt}, &ignored); err != nil {
		return err
	}
	incoming := url.Values{}
	incoming.Set("from_user_id", "eq."+relation.TargetUserID)
	incoming.Set("to_user_id", "eq."+relation.ActorUserID)
	incoming.Set("status", "eq.pending")
	return r.adminClient.Patch(ctx, r.serviceRoleKey, "drink_invites", incoming, map[string]any{"status": "rejected", "responded_at": respondedAt}, &ignored)
}

func firstMap(rows []map[string]any, fallback map[string]any) map[string]any {
	if len(rows) == 0 {
		return fallback
	}
	return rows[0]
}

func copyMap(row map[string]any) map[string]any {
	if row == nil {
		return nil
	}
	out := make(map[string]any, len(row)+2)
	for key, value := range row {
		out[key] = value
	}
	return out
}
