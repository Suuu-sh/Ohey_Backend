package notifications

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/yota/ohey/backend/internal/contracts"
)

type PostgresRepository struct{ pool *pgxpool.Pool }

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}
func (r *PostgresRepository) CreateNotification(ctx context.Context, n Notification) (bool, error) {
	if r.pool == nil {
		return false, errors.New("postgres pool is not configured")
	}
	_, err := r.pool.Exec(ctx, `insert into notifications (recipient_user_id,actor_user_id,friend_request_id,invite_id,notification_date,kind,title,message,system_key) values ($1,nullif($2,'')::uuid,nullif($3,'')::uuid,nullif($4,'')::uuid,nullif($5,'')::date,$6,$7,$8,nullif($9,''))`, n.RecipientUserID, n.ActorUserID, n.FriendRequestID, n.InviteID, n.NotificationDate, string(n.Kind), n.Title, n.Message, n.SystemKey)
	if err != nil {
		var pg *pgconn.PgError
		if errors.As(err, &pg) && pg.Code == "23505" {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
func (r *PostgresRepository) ListNotifications(ctx context.Context, _ string, recipientUserID string, limit int) ([]map[string]any, error) {
	if r.pool == nil {
		return nil, errors.New("postgres pool is not configured")
	}
	if limit <= 0 {
		limit = 50
	}
	rows, err := r.pool.Query(ctx, notificationSelectSQL+` where n.recipient_user_id=$1 order by n.created_at desc limit $2`, recipientUserID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		m, err := scanNotification(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
func (r *PostgresRepository) MarkAllRead(ctx context.Context, _ string, recipientUserID string, readAt time.Time) (int, error) {
	if r.pool == nil {
		return 0, errors.New("postgres pool is not configured")
	}
	tag, err := r.pool.Exec(ctx, `update notifications set read_at=$2 where recipient_user_id=$1 and read_at is null`, recipientUserID, readAt)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}
func (r *PostgresRepository) DisplayName(ctx context.Context, _ string, userID string) (string, error) {
	if r.pool == nil {
		return "", errors.New("postgres pool is not configured")
	}
	var name, uid string
	err := r.pool.QueryRow(ctx, `select display_name,user_id from profiles where id=$1`, userID).Scan(&name, &uid)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if name != "" {
		return name, nil
	}
	return uid, nil
}
func (r *PostgresRepository) TodayAcceptedInvites(ctx context.Context, _ string, userID, date string) ([]Invite, error) {
	if r.pool == nil {
		return nil, errors.New("postgres pool is not configured")
	}
	rows, err := r.pool.Query(ctx, `select id::text,inviter_user_id::text,invitee_user_id::text,scheduled_date,coalesce(activity_label,''),status from invites where scheduled_date=$1 and status=$2 and (inviter_user_id=$3 or invitee_user_id=$3)`, date, contracts.StatusAccepted, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Invite{}
	for rows.Next() {
		var i Invite
		var d time.Time
		if err := rows.Scan(&i.ID, &i.InviterUserID, &i.InviteeUserID, &d, &i.ActivityLabel, &i.Status); err != nil {
			return nil, err
		}
		i.ScheduledDate = d.Format(time.DateOnly)
		out = append(out, i)
	}
	return out, rows.Err()
}
func (r *PostgresRepository) AllProfileIDs(ctx context.Context) ([]string, error) {
	if r.pool == nil {
		return nil, errors.New("postgres pool is not configured")
	}
	rows, err := r.pool.Query(ctx, `select id::text from profiles order by created_at desc limit 10000`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}
func (r *PostgresRepository) VisibleYuruboRecipientIDs(ctx context.Context, _ string, ownerUserID, visibility string, groupIDs []string) ([]string, error) {
	if r.pool == nil {
		return nil, errors.New("postgres pool is not configured")
	}
	if visibility == contracts.VisibilityGroup && len(groupIDs) > 0 {
		rows, err := r.pool.Query(ctx, `select distinct friend_user_id::text from friend_group_members where group_id=any($1::uuid[]) and friend_user_id<>$2`, groupIDs, ownerUserID)
		return scanIDRows(rows, err)
	}
	rows, err := r.pool.Query(ctx, `select case when user_a_id=$1 then user_b_id::text else user_a_id::text end from friendships where user_a_id=$1 or user_b_id=$1`, ownerUserID)
	return scanIDRows(rows, err)
}
func (r *PostgresRepository) PushTokens(ctx context.Context, recipientUserID string) ([]string, error) {
	if r.pool == nil {
		return nil, errors.New("postgres pool is not configured")
	}
	rows, err := r.pool.Query(ctx, `select token from push_tokens where user_id=$1`, recipientUserID)
	return scanIDRows(rows, err)
}
func (r *PostgresRepository) DeletePushToken(ctx context.Context, token string) error {
	if r.pool == nil {
		return errors.New("postgres pool is not configured")
	}
	if token == "" {
		return nil
	}
	_, err := r.pool.Exec(ctx, `delete from push_tokens where token=$1`, token)
	return err
}
func scanIDRows(rows pgx.Rows, err error) ([]string, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		if id != "" {
			out = append(out, id)
		}
	}
	return out, rows.Err()
}

const notificationSelectSQL = `select n.id::text,n.kind,n.title,n.message,n.created_at,n.read_at,n.actor_user_id::text,n.friend_request_id::text,n.invite_id::text,n.notification_date,n.system_key,a.id::text,a.user_id,a.display_name,a.avatar_url,fr.id::text,fr.status,i.id::text,i.status,i.activity_label from notifications n left join profiles a on a.id=n.actor_user_id left join friend_requests fr on fr.id=n.friend_request_id left join invites i on i.id=n.invite_id`

func scanNotification(row pgx.Row) (map[string]any, error) {
	var id, kind, title, msg string
	var created time.Time
	var read *time.Time
	var actorID, frID, inviteID, systemKey *string
	var ndate *time.Time
	var aid, auid, aname, aavatar *string
	var fid, fstatus *string
	var iid, istatus, iactivity *string
	if err := row.Scan(&id, &kind, &title, &msg, &created, &read, &actorID, &frID, &inviteID, &ndate, &systemKey, &aid, &auid, &aname, &aavatar, &fid, &fstatus, &iid, &istatus, &iactivity); err != nil {
		return nil, err
	}
	m := map[string]any{"id": id, "kind": kind, "title": title, "message": msg, "created_at": created.UTC().Format(time.RFC3339Nano)}
	if read != nil {
		m["read_at"] = read.UTC().Format(time.RFC3339Nano)
	}
	if actorID != nil {
		m["actor_user_id"] = *actorID
	}
	if frID != nil {
		m["friend_request_id"] = *frID
	}
	if inviteID != nil {
		m["invite_id"] = *inviteID
	}
	if ndate != nil {
		m["notification_date"] = ndate.Format(time.DateOnly)
	}
	if systemKey != nil {
		m["system_key"] = *systemKey
	}
	if aid != nil {
		actor := map[string]any{"id": *aid}
		if auid != nil {
			actor["user_id"] = *auid
		}
		if aname != nil {
			actor["display_name"] = *aname
		}
		if aavatar != nil {
			actor["avatar_url"] = *aavatar
		}
		m["actor"] = actor
	}
	if fid != nil {
		m["friend_request"] = map[string]any{"id": *fid, "status": valueOrEmpty(fstatus)}
	}
	if iid != nil {
		m["invite"] = map[string]any{"id": *iid, "status": valueOrEmpty(istatus), "activity_label": valueOrEmpty(iactivity)}
	}
	return m, nil
}
func valueOrEmpty(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

var _ Repository = (*PostgresRepository)(nil)
