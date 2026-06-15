package invites

import (
	"context"
	"errors"
	"time"

	"github.com/Suuu-sh/Ohey_Backend/internal/contracts"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepository struct{ pool *pgxpool.Pool }

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) ListTodayReservations(ctx context.Context, _ string, userID, scheduledDate string) ([]map[string]any, error) {
	return r.listInvites(ctx, `i.scheduled_date=$1 and i.status=$2 and (i.inviter_user_id=$3 or i.invitee_user_id=$3) order by i.responded_at desc`, scheduledDate, contracts.StatusAccepted, userID)
}

func (r *PostgresRepository) ListIncomingPending(ctx context.Context, _ string, userID, scheduledDate string) ([]map[string]any, error) {
	return r.listInvites(ctx, `i.scheduled_date=$1 and i.invitee_user_id=$2 and i.status=$3 order by i.created_at desc`, scheduledDate, userID, contracts.StatusPending)
}

func (r *PostgresRepository) ListOutgoingActive(ctx context.Context, _ string, userID, scheduledDate string) ([]map[string]any, error) {
	return r.listInvites(ctx, `i.scheduled_date=$1 and i.inviter_user_id=$2 and i.status in ($3,$4) order by i.created_at desc`, scheduledDate, userID, contracts.StatusPending, contracts.StatusAccepted)
}

func (r *PostgresRepository) listInvites(ctx context.Context, where string, args ...any) ([]map[string]any, error) {
	if r.pool == nil {
		return nil, errors.New("postgres pool is not configured")
	}
	rows, err := r.pool.Query(ctx, inviteSelectSQL+` where `+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		item, err := scanInviteJoined(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) DailyStatus(ctx context.Context, _ string, userID, statusDate string) (string, error) {
	if r.pool == nil {
		return "", errors.New("postgres pool is not configured")
	}
	var status string
	err := r.pool.QueryRow(ctx, `select status from daily_statuses where user_id=$1 and status_date=$2 limit 1`, userID, statusDate).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return status, err
}

func (r *PostgresRepository) BlockExistsBetweenUsers(ctx context.Context, _ string, inviterUserID, inviteeUserID string) (bool, error) {
	if r.pool == nil {
		return false, errors.New("postgres pool is not configured")
	}
	var exists bool
	err := r.pool.QueryRow(ctx, `select exists(select 1 from user_blocks where (blocker_user_id=$1 and blocked_user_id=$2) or (blocker_user_id=$2 and blocked_user_id=$1))`, inviterUserID, inviteeUserID).Scan(&exists)
	return exists, err
}

func (r *PostgresRepository) FriendshipExists(ctx context.Context, _ string, inviterUserID, inviteeUserID string) (bool, error) {
	if r.pool == nil {
		return false, errors.New("postgres pool is not configured")
	}
	var exists bool
	err := r.pool.QueryRow(ctx, `select exists(select 1 from friendships where (user_a_id=$1 and user_b_id=$2) or (user_a_id=$2 and user_b_id=$1))`, inviterUserID, inviteeUserID).Scan(&exists)
	return exists, err
}

func (r *PostgresRepository) FindActiveInviteBetweenUsersForDate(ctx context.Context, _ string, inviterUserID, inviteeUserID, scheduledDate string) (*ExistingInvite, error) {
	if r.pool == nil {
		return nil, errors.New("postgres pool is not configured")
	}
	var item ExistingInvite
	err := r.pool.QueryRow(ctx, `select id::text,status from invites where scheduled_date=$1 and status in ($2,$3) and ((inviter_user_id=$4 and invitee_user_id=$5) or (inviter_user_id=$5 and invitee_user_id=$4)) limit 1`, scheduledDate, contracts.StatusPending, contracts.StatusAccepted, inviterUserID, inviteeUserID).Scan(&item.ID, &item.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *PostgresRepository) CreateInvite(ctx context.Context, _ string, invite NewInvite) (map[string]any, error) {
	if r.pool == nil {
		return nil, errors.New("postgres pool is not configured")
	}
	row := r.pool.QueryRow(ctx, `insert into invites (inviter_user_id,invitee_user_id,scheduled_date,status,activity_label) values ($1,$2,$3,$4,nullif($5,'')) returning id::text, inviter_user_id::text, invitee_user_id::text, scheduled_date, coalesce(activity_label,''), status, created_at, responded_at`, invite.InviterUserID, invite.InviteeUserID, invite.ScheduledDate, contracts.StatusPending, invite.ActivityLabel)
	out, err := scanInviteSimple(row)
	if err != nil {
		return nil, mapPostgresInviteError(err)
	}
	return out, nil
}

func (r *PostgresRepository) UpdatePendingInviteStatus(ctx context.Context, _ string, inviteID, recipientUserID string, status InviteStatus, respondedAt time.Time) (map[string]any, error) {
	if r.pool == nil {
		return nil, errors.New("postgres pool is not configured")
	}
	row := r.pool.QueryRow(ctx, `update invites set status=$3, responded_at=$4 where id=$1 and invitee_user_id=$2 and status=$5 returning id::text, inviter_user_id::text, invitee_user_id::text, scheduled_date, coalesce(activity_label,''), status, created_at, responded_at`, inviteID, recipientUserID, string(status), respondedAt, contracts.StatusPending)
	out, err := scanInviteSimple(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, mapPostgresInviteError(err)
	}
	return out, nil
}

const inviteSelectSQL = `select i.id::text, i.inviter_user_id::text, i.invitee_user_id::text, i.scheduled_date, coalesce(i.activity_label,''), i.status, i.created_at, i.responded_at, inviter.id::text, inviter.display_name, inviter.user_id, inviter.avatar_url, invitee.id::text, invitee.display_name, invitee.user_id, invitee.avatar_url from invites i join profiles inviter on inviter.id=i.inviter_user_id join profiles invitee on invitee.id=i.invitee_user_id`

func scanInviteJoined(row pgx.Row) (map[string]any, error) {
	base, err := scanInviteBase(row, true)
	return base, err
}
func scanInviteSimple(row pgx.Row) (map[string]any, error) { return scanInviteBase(row, false) }

func scanInviteBase(row pgx.Row, withProfiles bool) (map[string]any, error) {
	var id, inviterID, inviteeID, activity, status string
	var scheduled time.Time
	var created time.Time
	var responded *time.Time
	if !withProfiles {
		if err := row.Scan(&id, &inviterID, &inviteeID, &scheduled, &activity, &status, &created, &responded); err != nil {
			return nil, err
		}
		return inviteMap(id, inviterID, inviteeID, scheduled, activity, status, created, responded), nil
	}
	var inviter profileLite
	var invitee profileLite
	if err := row.Scan(&id, &inviterID, &inviteeID, &scheduled, &activity, &status, &created, &responded, &inviter.ID, &inviter.DisplayName, &inviter.UserID, &inviter.AvatarURL, &invitee.ID, &invitee.DisplayName, &invitee.UserID, &invitee.AvatarURL); err != nil {
		return nil, err
	}
	out := inviteMap(id, inviterID, inviteeID, scheduled, activity, status, created, responded)
	out["inviter"] = inviter.mapValue()
	out["invitee"] = invitee.mapValue()
	return out, nil
}

func inviteMap(id, inviterID, inviteeID string, scheduled time.Time, activity, status string, created time.Time, responded *time.Time) map[string]any {
	m := map[string]any{"id": id, "inviter_user_id": inviterID, "invitee_user_id": inviteeID, "scheduled_date": scheduled.Format(time.DateOnly), "activity_label": activity, "status": status, "created_at": created.UTC().Format(time.RFC3339Nano)}
	if responded != nil {
		m["responded_at"] = responded.UTC().Format(time.RFC3339Nano)
	}
	return m
}

type profileLite struct {
	ID, DisplayName, UserID string
	AvatarURL               *string
}

func (p profileLite) mapValue() map[string]any {
	m := map[string]any{"id": p.ID, "display_name": p.DisplayName, "user_id": p.UserID}
	if p.AvatarURL != nil {
		m["avatar_url"] = *p.AvatarURL
	}
	return m
}

func mapPostgresInviteError(err error) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return UserError{Kind: ErrorKindConflict, Message: "すでに招待中です。"}
		case "23503":
			return UserError{Kind: ErrorKindInvalidInput, Message: "profile not found"}
		case "23514", "22P02":
			return UserError{Kind: ErrorKindInvalidInput, Message: "invalid invite"}
		}
	}
	return err
}
