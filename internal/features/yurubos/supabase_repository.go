package yurubos

import (
	"context"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/yota/ohey/backend/internal/contracts"
	"github.com/yota/ohey/backend/internal/supabase"
)

const yuruboSelectColumns = "id,wish_item_id,owner_user_id,title,body,category,place_text,place_lat,place_lng,time_label,starts_at,ends_at,status,visibility,expires_at,created_at,updated_at,owner:profiles!yurubos_owner_user_id_fkey(id,user_id,display_name,character_key,avatar_url,is_plus)"

type SupabaseRepository struct {
	client         *supabase.Client
	adminClient    *supabase.Client
	serviceRoleKey string
}

func NewSupabaseRepository(client, adminClient *supabase.Client, serviceRoleKey string) *SupabaseRepository {
	return &SupabaseRepository{client: client, adminClient: adminClient, serviceRoleKey: serviceRoleKey}
}

func (r *SupabaseRepository) WishItemExists(ctx context.Context, authToken, ownerUserID, wishItemID string) (bool, error) {
	q := url.Values{}
	q.Set("select", "id")
	q.Set("id", "eq."+wishItemID)
	q.Set("owner_user_id", "eq."+ownerUserID)
	q.Set("limit", "1")
	var rows []map[string]any
	if err := r.client.Get(ctx, authToken, "wish_items", q, &rows); err != nil {
		return false, err
	}
	return len(rows) > 0, nil
}

func (r *SupabaseRepository) CreateYurubo(ctx context.Context, authToken string, item Yurubo) (map[string]any, error) {
	payload := map[string]any{
		"owner_user_id": item.OwnerUserID,
		"title":         item.Title,
		"body":          item.Body,
		"category":      item.Category,
		"place_text":    item.PlaceText,
		"time_label":    item.TimeLabel,
		"visibility":    item.Visibility,
		"expires_at":    time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339),
	}
	if item.StartsAt != nil {
		payload["starts_at"] = *item.StartsAt
	}
	if item.WishItemID != nil {
		payload["wish_item_id"] = *item.WishItemID
	}
	var rows []map[string]any
	if err := r.client.Post(ctx, authToken, "yurubos", nil, payload, &rows); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, UserError{Kind: ErrorKindUpstream, Message: "yurubo insert returned no rows"}
	}
	return rows[0], nil
}

func (r *SupabaseRepository) LinkVisibilityGroup(ctx context.Context, authToken, yuruboID, groupID string) error {
	var ignored []map[string]any
	return r.client.Post(ctx, authToken, "yurubo_visibility_groups", nil, map[string]any{"yurubo_id": yuruboID, "group_id": groupID}, &ignored)
}

func (r *SupabaseRepository) UpdateYurubo(ctx context.Context, authToken string, update YuruboUpdate) (map[string]any, error) {
	q := url.Values{}
	q.Set("id", "eq."+update.YuruboID)
	q.Set("owner_user_id", "eq."+update.OwnerUserID)
	payload := map[string]any{
		"title":      update.Title,
		"body":       update.Body,
		"place_text": update.PlaceText,
		"time_label": update.TimeLabel,
	}
	if update.StartsAtSet {
		if update.StartsAt != nil {
			payload["starts_at"] = *update.StartsAt
		} else {
			payload["starts_at"] = nil
		}
	}
	var rows []map[string]any
	if err := r.client.Patch(ctx, authToken, "yurubos", q, payload, &rows); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return rows[0], nil
}

func (r *SupabaseRepository) DeleteYurubo(ctx context.Context, authToken, yuruboID, ownerUserID string) (map[string]any, error) {
	q := url.Values{}
	q.Set("id", "eq."+yuruboID)
	q.Set("owner_user_id", "eq."+ownerUserID)
	var rows []map[string]any
	if err := r.client.Delete(ctx, authToken, "yurubos", q, &rows); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return rows[0], nil
}

func (r *SupabaseRepository) HiddenYuruboIDs(ctx context.Context, authToken, userID string) (map[string]bool, error) {
	q := url.Values{}
	q.Set("select", "yurubo_id")
	q.Set("user_id", "eq."+userID)
	var rows []map[string]any
	if err := r.client.Get(ctx, authToken, "hidden_yurubos", q, &rows); err != nil {
		return nil, err
	}
	hidden := map[string]bool{}
	for _, row := range rows {
		if id, _ := row["yurubo_id"].(string); id != "" {
			hidden[id] = true
		}
	}
	return hidden, nil
}

func (r *SupabaseRepository) ListOpenYurubos(ctx context.Context, authToken string, limit int) ([]map[string]any, error) {
	q := url.Values{}
	q.Set("select", yuruboSelectColumns)
	q.Set("order", "created_at.desc")
	q.Set("limit", strconv.Itoa(limit))
	q.Set("status", supabase.PostgRESTEq(contracts.StatusOpen))
	q.Set("expires_at", "gt."+time.Now().UTC().Format(time.RFC3339))
	var rows []map[string]any
	if err := r.client.Get(ctx, authToken, "yurubos", q, &rows); err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *SupabaseRepository) ListReactions(ctx context.Context, authToken string, yuruboIDs []string) ([]map[string]any, error) {
	if len(yuruboIDs) == 0 {
		return []map[string]any{}, nil
	}
	q := url.Values{}
	q.Set("select", "yurubo_id,user_id,reaction_type")
	q.Set("yurubo_id", "in.("+strings.Join(yuruboIDs, ",")+")")
	var rows []map[string]any
	if err := r.client.Get(ctx, authToken, "yurubo_reactions", q, &rows); err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *SupabaseRepository) ParticipantProfiles(ctx context.Context, authToken string, userIDs []string) (map[string]map[string]any, error) {
	profilesByID := map[string]map[string]any{}
	if len(userIDs) == 0 {
		return profilesByID, nil
	}
	q := url.Values{}
	q.Set("select", "id,user_id,display_name,avatar_url")
	q.Set("id", "in.("+strings.Join(userIDs, ",")+")")
	var profiles []map[string]any
	if err := r.client.Get(ctx, authToken, "profiles", q, &profiles); err != nil {
		return nil, err
	}
	for _, profile := range profiles {
		id, _ := profile["id"].(string)
		if id != "" {
			profilesByID[id] = profile
		}
	}
	return profilesByID, nil
}

func (r *SupabaseRepository) OwnerID(ctx context.Context, authToken, yuruboID string) (string, error) {
	q := url.Values{}
	q.Set("select", "owner_user_id")
	q.Set("id", "eq."+yuruboID)
	q.Set("limit", "1")
	var rows []map[string]any
	if err := r.client.Get(ctx, authToken, "yurubos", q, &rows); err != nil {
		return "", err
	}
	if len(rows) == 0 {
		return "", nil
	}
	ownerID, _ := rows[0]["owner_user_id"].(string)
	return ownerID, nil
}

func (r *SupabaseRepository) VisibilityLabels(ctx context.Context, authToken string, rows []map[string]any) (map[string]string, error) {
	labels := map[string]string{}
	groupIDs := []string{}
	for _, row := range rows {
		id, _ := row["id"].(string)
		visibility, _ := row["visibility"].(string)
		if visibility != contracts.VisibilityGroup {
			labels[id] = "全フレンズ"
			continue
		}
		groupIDs = append(groupIDs, id)
	}
	if len(groupIDs) == 0 {
		return labels, nil
	}
	q := url.Values{}
	q.Set("select", "yurubo_id,friend_groups(name)")
	q.Set("yurubo_id", "in.("+strings.Join(groupIDs, ",")+")")
	var links []map[string]any
	if err := r.client.Get(ctx, authToken, "yurubo_visibility_groups", q, &links); err != nil {
		for _, id := range groupIDs {
			labels[id] = "グループ"
		}
		return labels, nil
	}
	for _, link := range links {
		id, _ := link["yurubo_id"].(string)
		group, _ := link["friend_groups"].(map[string]any)
		name, _ := group["name"].(string)
		if strings.TrimSpace(name) == "" {
			name = "グループ"
		}
		labels[id] = strings.TrimSpace(name)
	}
	return labels, nil
}

func (r *SupabaseRepository) UpsertReaction(ctx context.Context, authToken string, reaction Reaction) error {
	payload := map[string]any{"yurubo_id": reaction.YuruboID, "user_id": reaction.UserID, "reaction_type": reaction.ReactionType}
	q := url.Values{}
	q.Set("on_conflict", "yurubo_id,user_id")
	var rows []map[string]any
	return r.client.Upsert(ctx, authToken, "yurubo_reactions", q, payload, &rows)
}

func (r *SupabaseRepository) ApproveReaction(ctx context.Context, authToken, ownerUserID, yuruboID, participantID string) (bool, error) {
	q := url.Values{}
	q.Set("yurubo_id", "eq."+yuruboID)
	q.Set("user_id", "eq."+participantID)
	client := r.client
	updateToken := authToken
	if r.adminClient != nil && r.serviceRoleKey != "" {
		client = r.adminClient
		updateToken = r.serviceRoleKey
	}
	var rows []map[string]any
	if err := client.Patch(ctx, updateToken, "yurubo_reactions", q, map[string]any{"reaction_type": contracts.ReactionTypeAvailable}, &rows); err != nil {
		return false, err
	}
	return len(rows) > 0, nil
}

func (r *SupabaseRepository) DeleteReaction(ctx context.Context, authToken, yuruboID, userID string) error {
	q := url.Values{}
	q.Set("yurubo_id", "eq."+yuruboID)
	q.Set("user_id", "eq."+userID)
	var rows []map[string]any
	return r.client.Delete(ctx, authToken, "yurubo_reactions", q, &rows)
}
