package invites

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/yota/nomo/backend/internal/supabase"
)

const inviteSelect = "id,inviter_user_id,invitee_user_id,scheduled_date,status,inviter:profiles!invites_inviter_user_id_fkey(id,display_name,user_id,gender,avatar_url),invitee:profiles!invites_invitee_user_id_fkey(id,display_name,user_id,gender,avatar_url)"

type SupabaseRepository struct {
	client *supabase.Client
}

func NewSupabaseRepository(client *supabase.Client) *SupabaseRepository {
	return &SupabaseRepository{client: client}
}

func (r *SupabaseRepository) ListTodayReservations(ctx context.Context, authToken, userID, scheduledDate string) ([]map[string]any, error) {
	q := url.Values{}
	q.Set("select", inviteSelect)
	q.Set("scheduled_date", "eq."+scheduledDate)
	q.Set("status", "eq.accepted")
	q.Set("or", "(inviter_user_id.eq."+userID+",invitee_user_id.eq."+userID+")")
	q.Set("order", "responded_at.desc")
	var rows []map[string]any
	if err := r.client.Get(ctx, authToken, "invites", q, &rows); err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *SupabaseRepository) ListIncomingPending(ctx context.Context, authToken, userID, scheduledDate string) ([]map[string]any, error) {
	q := url.Values{}
	q.Set("select", inviteSelect)
	q.Set("scheduled_date", "eq."+scheduledDate)
	q.Set("invitee_user_id", "eq."+userID)
	q.Set("status", "eq.pending")
	q.Set("order", "created_at.desc")
	var rows []map[string]any
	if err := r.client.Get(ctx, authToken, "invites", q, &rows); err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *SupabaseRepository) ListOutgoingActive(ctx context.Context, authToken, userID, scheduledDate string) ([]map[string]any, error) {
	q := url.Values{}
	q.Set("select", inviteSelect)
	q.Set("scheduled_date", "eq."+scheduledDate)
	q.Set("inviter_user_id", "eq."+userID)
	q.Set("status", "in.(pending,accepted)")
	q.Set("order", "created_at.desc")
	var rows []map[string]any
	if err := r.client.Get(ctx, authToken, "invites", q, &rows); err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *SupabaseRepository) DailyStatus(ctx context.Context, authToken, userID, statusDate string) (string, error) {
	q := url.Values{}
	q.Set("select", "status")
	q.Set("user_id", "eq."+userID)
	q.Set("status_date", "eq."+statusDate)
	q.Set("limit", "1")
	var rows []map[string]any
	if err := r.client.Get(ctx, authToken, "daily_statuses", q, &rows); err != nil {
		return "", err
	}
	if len(rows) == 0 {
		return "", nil
	}
	status, _ := rows[0]["status"].(string)
	return status, nil
}

func (r *SupabaseRepository) BlockExistsBetweenUsers(ctx context.Context, authToken, inviterUserID, inviteeUserID string) (bool, error) {
	q := url.Values{}
	q.Set("select", "blocker_user_id")
	q.Set("or", "(and(blocker_user_id.eq."+inviterUserID+",blocked_user_id.eq."+inviteeUserID+"),and(blocker_user_id.eq."+inviteeUserID+",blocked_user_id.eq."+inviterUserID+"))")
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

func (r *SupabaseRepository) FindActiveInviteBetweenUsersForDate(ctx context.Context, authToken, inviterUserID, inviteeUserID, scheduledDate string) (*ExistingInvite, error) {
	q := url.Values{}
	q.Set("select", "id,status")
	q.Set("scheduled_date", "eq."+scheduledDate)
	q.Set("or", "(and(inviter_user_id.eq."+inviterUserID+",invitee_user_id.eq."+inviteeUserID+"),and(inviter_user_id.eq."+inviteeUserID+",invitee_user_id.eq."+inviterUserID+"))")
	q.Set("status", "in.(pending,accepted)")
	q.Set("limit", "1")
	var rows []map[string]any
	if err := r.client.Get(ctx, authToken, "invites", q, &rows); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	id, _ := rows[0]["id"].(string)
	status, _ := rows[0]["status"].(string)
	return &ExistingInvite{ID: id, Status: InviteStatus(status)}, nil
}

func (r *SupabaseRepository) CreateInvite(ctx context.Context, authToken string, invite NewInvite) (map[string]any, error) {
	payload := map[string]any{
		"inviter_user_id": invite.InviterUserID,
		"invitee_user_id": invite.InviteeUserID,
		"scheduled_date":  invite.ScheduledDate,
		"status":          string(InviteStatusPending),
	}
	var rows []map[string]any
	if err := r.client.Post(ctx, authToken, "invites", nil, payload, &rows); err != nil {
		return nil, err
	}
	return firstMap(rows, payload), nil
}

func (r *SupabaseRepository) UpdatePendingInviteStatus(ctx context.Context, authToken, inviteID, recipientUserID string, status InviteStatus, respondedAt time.Time) (map[string]any, error) {
	q := url.Values{}
	q.Set("id", "eq."+inviteID)
	q.Set("invitee_user_id", "eq."+recipientUserID)
	q.Set("status", "eq.pending")
	payload := map[string]any{
		"status":       string(status),
		"responded_at": respondedAt.Format(time.RFC3339),
	}
	var rows []map[string]any
	if err := r.client.Patch(ctx, authToken, "invites", q, payload, &rows); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return rows[0], nil
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

func firstMap(rows []map[string]any, fallback map[string]any) map[string]any {
	if len(rows) == 0 {
		return fallback
	}
	return rows[0]
}
