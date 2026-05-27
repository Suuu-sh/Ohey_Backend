package notifications

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/yota/nomo/backend/internal/supabase"
)

const notificationSelectColumns = "id,kind,title,message,created_at,read_at,actor_user_id,drink_log_id,friend_request_id,drink_invite_id,notification_date,system_key,actor:profiles!notifications_actor_user_id_fkey(id,user_id,display_name,avatar_url),friend_request:friend_requests!notifications_friend_request_id_fkey(id,status),drink_invite:drink_invites!notifications_drink_invite_id_fkey(id,status)"

type SupabaseRepository struct {
	client         *supabase.Client
	adminClient    *supabase.Client
	serviceRoleKey string
}

func NewSupabaseRepository(client, adminClient *supabase.Client, serviceRoleKey string) *SupabaseRepository {
	return &SupabaseRepository{client: client, adminClient: adminClient, serviceRoleKey: strings.TrimSpace(serviceRoleKey)}
}

func (r *SupabaseRepository) CreateNotification(ctx context.Context, notification Notification) (bool, error) {
	if r.adminClient == nil || r.serviceRoleKey == "" {
		return false, errors.New("admin supabase client is not configured")
	}
	var rows []map[string]any
	if err := r.adminClient.Post(ctx, r.serviceRoleKey, "notifications", nil, notification.Payload(), &rows); err != nil {
		var apiErr supabase.APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusConflict {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (r *SupabaseRepository) ListNotifications(ctx context.Context, authToken, recipientUserID string, limit int) ([]map[string]any, error) {
	q := url.Values{}
	q.Set("select", notificationSelectColumns)
	q.Set("recipient_user_id", "eq."+recipientUserID)
	q.Set("order", "created_at.desc")
	q.Set("limit", "50")
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	var rows []map[string]any
	if err := r.client.Get(ctx, authToken, "notifications", q, &rows); err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *SupabaseRepository) MarkAllRead(ctx context.Context, authToken, recipientUserID string, readAt time.Time) (int, error) {
	q := url.Values{}
	q.Set("recipient_user_id", "eq."+recipientUserID)
	q.Set("read_at", "is.null")
	var rows []map[string]any
	if err := r.client.Patch(ctx, authToken, "notifications", q, map[string]any{"read_at": readAt.UTC().Format(time.RFC3339)}, &rows); err != nil {
		return 0, err
	}
	return len(rows), nil
}

func (r *SupabaseRepository) DisplayName(ctx context.Context, authToken, userID string) (string, error) {
	q := url.Values{}
	q.Set("select", "display_name,user_id")
	q.Set("id", "eq."+userID)
	q.Set("limit", "1")
	if r.adminClient != nil && r.serviceRoleKey != "" {
		var adminRows []map[string]any
		if err := r.adminClient.Get(ctx, r.serviceRoleKey, "profiles", q, &adminRows); err == nil && len(adminRows) > 0 {
			if name := displayNameFromProfile(adminRows[0]); name != "" {
				return name, nil
			}
		}
	}
	var rows []map[string]any
	if err := r.client.Get(ctx, authToken, "profiles", q, &rows); err != nil {
		return "", err
	}
	if len(rows) == 0 {
		return "", nil
	}
	return displayNameFromProfile(rows[0]), nil
}

func (r *SupabaseRepository) DrinkLogOwnerUserID(ctx context.Context, authToken, logID string) (string, error) {
	q := url.Values{}
	q.Set("select", "id,owner_user_id")
	q.Set("id", "eq."+logID)
	q.Set("limit", "1")
	var logs []map[string]any
	if err := r.client.Get(ctx, authToken, "drink_logs", q, &logs); err != nil {
		return "", err
	}
	if len(logs) == 0 {
		return "", nil
	}
	ownerUserID, _ := logs[0]["owner_user_id"].(string)
	return ownerUserID, nil
}

func (r *SupabaseRepository) TodayAcceptedInvites(ctx context.Context, authToken, userID, date string) ([]DrinkInvite, error) {
	q := url.Values{}
	q.Set("select", "id,from_user_id,to_user_id,invite_date,status")
	q.Set("invite_date", "eq."+date)
	q.Set("status", "eq.accepted")
	q.Set("or", "(from_user_id.eq."+userID+",to_user_id.eq."+userID+")")
	var rows []map[string]any
	if err := r.client.Get(ctx, authToken, "drink_invites", q, &rows); err != nil {
		return nil, err
	}
	invites := make([]DrinkInvite, 0, len(rows))
	for _, row := range rows {
		invites = append(invites, DrinkInviteFromRow(row))
	}
	return invites, nil
}

func (r *SupabaseRepository) AllProfileIDs(ctx context.Context) ([]string, error) {
	if r.adminClient == nil || r.serviceRoleKey == "" {
		return nil, errors.New("admin supabase client is not configured")
	}
	q := url.Values{}
	q.Set("select", "id")
	q.Set("order", "created_at.desc")
	q.Set("limit", "10000")
	var profiles []map[string]any
	if err := r.adminClient.Get(ctx, r.serviceRoleKey, "profiles", q, &profiles); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(profiles))
	for _, profile := range profiles {
		id, _ := profile["id"].(string)
		if cleanID, err := CleanUUID(id, "recipient user id"); err == nil {
			ids = append(ids, cleanID)
		}
	}
	return ids, nil
}

func (r *SupabaseRepository) PushTokens(ctx context.Context, recipientUserID string) ([]string, error) {
	if r.adminClient == nil || r.serviceRoleKey == "" {
		return nil, nil
	}
	q := url.Values{}
	q.Set("select", "token")
	q.Set("user_id", "eq."+recipientUserID)
	var rows []map[string]any
	if err := r.adminClient.Get(ctx, r.serviceRoleKey, "push_tokens", q, &rows); err != nil {
		return nil, err
	}
	tokens := make([]string, 0, len(rows))
	for _, row := range rows {
		token, _ := row["token"].(string)
		if token != "" {
			tokens = append(tokens, token)
		}
	}
	return tokens, nil
}

func displayNameFromProfile(profile map[string]any) string {
	if name, ok := profile["display_name"].(string); ok && strings.TrimSpace(name) != "" {
		return strings.TrimSpace(name)
	}
	if userName, ok := profile["user_id"].(string); ok && strings.TrimSpace(userName) != "" {
		return strings.TrimSpace(userName)
	}
	return ""
}
