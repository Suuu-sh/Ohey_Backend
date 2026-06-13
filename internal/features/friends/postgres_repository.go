package friends

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

func (r *PostgresRepository) ListFriendships(ctx context.Context, _ string, userID string) ([]map[string]any, error) {
	if r.pool == nil {
		return nil, errors.New("postgres pool is not configured")
	}
	rows, err := r.pool.Query(ctx, `
		select f.user_a_id::text, f.user_b_id::text, f.is_favorite,
		       a.id::text, a.user_id, a.display_name, a.character_key, a.avatar_url, a.is_plus,
		       b.id::text, b.user_id, b.display_name, b.character_key, b.avatar_url, b.is_plus
		from friendships f
		join profiles a on a.id = f.user_a_id
		join profiles b on b.id = f.user_b_id
		where f.user_a_id = $1 or f.user_b_id = $1
		order by f.created_at desc`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		row, err := scanFriendshipRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) AttachTodayStatuses(ctx context.Context, _ string, rows []map[string]any, date string) error {
	if r.pool == nil {
		return errors.New("postgres pool is not configured")
	}
	ids := []string{}
	profiles := map[string]map[string]any{}
	for _, row := range rows {
		for _, key := range []string{"user_a", "user_b"} {
			profile, ok := row[key].(map[string]any)
			if !ok {
				continue
			}
			id, _ := profile["id"].(string)
			if id == "" {
				continue
			}
			if profiles[id] == nil {
				ids = append(ids, id)
			}
			profiles[id] = profile
		}
	}
	if len(ids) == 0 {
		return nil
	}
	statusRows, err := r.pool.Query(ctx, `select user_id::text, status from daily_statuses where user_id = any($1::uuid[]) and status_date = $2`, ids, date)
	if err != nil {
		return err
	}
	defer statusRows.Close()
	for statusRows.Next() {
		var id, status string
		if err := statusRows.Scan(&id, &status); err != nil {
			return err
		}
		if p := profiles[id]; p != nil && status != "" {
			p["status_key"] = status
		}
	}
	return statusRows.Err()
}

func (r *PostgresRepository) UpdateFriendFavorite(ctx context.Context, _ string, userID, friendID string, isFavorite bool) (map[string]any, error) {
	return r.friendshipMutation(ctx, `update friendships set is_favorite = $3 where (user_a_id = $1 and user_b_id = $2) or (user_a_id = $2 and user_b_id = $1) returning user_a_id::text,user_b_id::text,is_favorite`, userID, friendID, isFavorite)
}

func (r *PostgresRepository) UpsertFriendshipPair(ctx context.Context, _ string, userA, userB string) (map[string]any, error) {
	if r.pool == nil {
		return nil, errors.New("postgres pool is not configured")
	}
	first, second := OrderedPair(userA, userB)
	row := r.pool.QueryRow(ctx, `insert into friendships (user_a_id,user_b_id) values ($1,$2) on conflict (user_a_id,user_b_id) do update set user_a_id=excluded.user_a_id returning user_a_id::text,user_b_id::text,is_favorite`, first, second)
	out, err := scanFriendshipSimple(row)
	if err != nil {
		return nil, mapPostgresFriendError(err)
	}
	return out, nil
}

func (r *PostgresRepository) DeleteFriendship(ctx context.Context, _ string, userID, friendID string) (map[string]any, error) {
	return r.friendshipMutation(ctx, `delete from friendships where (user_a_id = $1 and user_b_id = $2) or (user_a_id = $2 and user_b_id = $1) returning user_a_id::text,user_b_id::text,is_favorite`, userID, friendID)
}

func (r *PostgresRepository) friendshipMutation(ctx context.Context, sql, userID, friendID string, args ...any) (map[string]any, error) {
	if r.pool == nil {
		return nil, errors.New("postgres pool is not configured")
	}
	params := append([]any{userID, friendID}, args...)
	out, err := scanFriendshipSimple(r.pool.QueryRow(ctx, sql, params...))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, mapPostgresFriendError(err)
	}
	return out, nil
}

func (r *PostgresRepository) FriendshipExists(ctx context.Context, _ string, userID, friendID string) (bool, error) {
	return r.exists(ctx, `select exists(select 1 from friendships where (user_a_id=$1 and user_b_id=$2) or (user_a_id=$2 and user_b_id=$1))`, userID, friendID)
}
func (r *PostgresRepository) BlockExistsBetweenUsers(ctx context.Context, _ string, userID, friendID string) (bool, error) {
	return r.exists(ctx, `select exists(select 1 from user_blocks where (blocker_user_id=$1 and blocked_user_id=$2) or (blocker_user_id=$2 and blocked_user_id=$1))`, userID, friendID)
}
func (r *PostgresRepository) exists(ctx context.Context, sql, a, b string) (bool, error) {
	if r.pool == nil {
		return false, errors.New("postgres pool is not configured")
	}
	var ok bool
	err := r.pool.QueryRow(ctx, sql, a, b).Scan(&ok)
	return ok, err
}

func (r *PostgresRepository) ListPendingFriendRequests(ctx context.Context, _ string, userID string, direction RequestDirection) ([]map[string]any, error) {
	if r.pool == nil {
		return nil, errors.New("postgres pool is not configured")
	}
	where := `(fr.from_user_id = $1 or fr.to_user_id = $1)`
	if direction == RequestDirectionIncoming {
		where = `fr.to_user_id = $1`
	} else if direction == RequestDirectionOutgoing {
		where = `fr.from_user_id = $1`
	}
	rows, err := r.pool.Query(ctx, friendRequestSelectSQL+` where fr.status = $2 and `+where+` order by fr.created_at desc`, userID, contracts.StatusPending)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanFriendRequestRows(rows)
}

func (r *PostgresRepository) PendingFriendRequestBetween(ctx context.Context, _ string, userID, friendID string) (map[string]any, error) {
	if r.pool == nil {
		return nil, errors.New("postgres pool is not configured")
	}
	return r.pendingFriendRequestBetween(ctx, userID, friendID)
}

func (r *PostgresRepository) pendingFriendRequestBetween(ctx context.Context, userID, friendID string) (map[string]any, error) {
	row := r.pool.QueryRow(ctx, `select id::text, from_user_id::text, to_user_id::text, status, created_at, responded_at from friend_requests where status=$1 and ((from_user_id=$2 and to_user_id=$3) or (from_user_id=$3 and to_user_id=$2)) limit 1`, contracts.StatusPending, userID, friendID)
	out, err := scanFriendRequestSimple(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return out, err
}

func (r *PostgresRepository) CreateFriendRequest(ctx context.Context, _ string, fromUserID, toUserID string) (map[string]any, error) {
	if r.pool == nil {
		return nil, errors.New("postgres pool is not configured")
	}
	row := r.pool.QueryRow(ctx, `insert into friend_requests (from_user_id,to_user_id,status) values ($1,$2,$3) returning id::text, from_user_id::text, to_user_id::text, status, created_at, responded_at`, fromUserID, toUserID, contracts.StatusPending)
	out, err := scanFriendRequestSimple(row)
	if err != nil {
		return nil, mapPostgresFriendError(err)
	}
	return out, nil
}

func (r *PostgresRepository) UpdatePendingFriendRequestStatus(ctx context.Context, _ string, requestID, userID string, status RequestStatus, respondedAt time.Time) (map[string]any, error) {
	if r.pool == nil {
		return nil, errors.New("postgres pool is not configured")
	}
	userCol := "to_user_id"
	if status == RequestStatusCancelled {
		userCol = "from_user_id"
	}
	row := r.pool.QueryRow(ctx, `update friend_requests set status=$3, responded_at=$4 where id=$1 and `+userCol+`=$2 and status=$5 returning id::text, from_user_id::text, to_user_id::text, status, created_at, responded_at`, requestID, userID, string(status), respondedAt, contracts.StatusPending)
	out, err := scanFriendRequestSimple(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, mapPostgresFriendError(err)
	}
	return out, nil
}

const friendRequestSelectSQL = `select fr.id::text, fr.from_user_id::text, fr.to_user_id::text, fr.status, fr.created_at, fr.responded_at, fp.id::text, fp.user_id, fp.display_name, fp.character_key, fp.avatar_url, fp.is_plus, tp.id::text, tp.user_id, tp.display_name, tp.character_key, tp.avatar_url, tp.is_plus from friend_requests fr join profiles fp on fp.id=fr.from_user_id join profiles tp on tp.id=fr.to_user_id`

func scanFriendshipRow(row pgx.Row) (map[string]any, error) {
	var ua, ub string
	var fav bool
	a := profileScanTarget{}
	b := profileScanTarget{}
	if err := row.Scan(&ua, &ub, &fav, &a.ID, &a.UserID, &a.DisplayName, &a.CharacterKey, &a.AvatarURL, &a.IsPlus, &b.ID, &b.UserID, &b.DisplayName, &b.CharacterKey, &b.AvatarURL, &b.IsPlus); err != nil {
		return nil, err
	}
	return map[string]any{"user_a_id": ua, "user_b_id": ub, "is_favorite": fav, "user_a": a.mapValue(), "user_b": b.mapValue()}, nil
}
func scanFriendshipSimple(row pgx.Row) (map[string]any, error) {
	var ua, ub string
	var fav bool
	if err := row.Scan(&ua, &ub, &fav); err != nil {
		return nil, err
	}
	return map[string]any{"user_a_id": ua, "user_b_id": ub, "is_favorite": fav}, nil
}
func scanFriendRequestRows(rows pgx.Rows) ([]map[string]any, error) {
	out := []map[string]any{}
	for rows.Next() {
		item, err := scanFriendRequestJoined(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}
func scanFriendRequestJoined(row pgx.Row) (map[string]any, error) {
	base, from, to, err := scanFriendRequestBaseProfiles(row)
	if err != nil {
		return nil, err
	}
	base["from_user"] = from.mapValue()
	base["to_user"] = to.mapValue()
	return base, nil
}
func scanFriendRequestBaseProfiles(row pgx.Row) (map[string]any, profileScanTarget, profileScanTarget, error) {
	var id, fromID, toID, status string
	var created time.Time
	var responded *time.Time
	from := profileScanTarget{}
	to := profileScanTarget{}
	err := row.Scan(&id, &fromID, &toID, &status, &created, &responded, &from.ID, &from.UserID, &from.DisplayName, &from.CharacterKey, &from.AvatarURL, &from.IsPlus, &to.ID, &to.UserID, &to.DisplayName, &to.CharacterKey, &to.AvatarURL, &to.IsPlus)
	base := friendRequestMap(id, fromID, toID, status, created, responded)
	return base, from, to, err
}
func scanFriendRequestSimple(row pgx.Row) (map[string]any, error) {
	var id, fromID, toID, status string
	var created time.Time
	var responded *time.Time
	if err := row.Scan(&id, &fromID, &toID, &status, &created, &responded); err != nil {
		return nil, err
	}
	return friendRequestMap(id, fromID, toID, status, created, responded), nil
}
func friendRequestMap(id, fromID, toID, status string, created time.Time, responded *time.Time) map[string]any {
	m := map[string]any{"id": id, "from_user_id": fromID, "to_user_id": toID, "status": status, "created_at": created.UTC().Format(time.RFC3339Nano)}
	if responded != nil {
		m["responded_at"] = responded.UTC().Format(time.RFC3339Nano)
	}
	return m
}

type profileScanTarget struct {
	ID, UserID, DisplayName, CharacterKey string
	AvatarURL                             *string
	IsPlus                                bool
}

func (p profileScanTarget) mapValue() map[string]any {
	m := map[string]any{"id": p.ID, "user_id": p.UserID, "display_name": p.DisplayName, "character_key": p.CharacterKey, "is_plus": p.IsPlus}
	if p.AvatarURL != nil {
		m["avatar_url"] = *p.AvatarURL
	}
	return m
}

func mapPostgresFriendError(err error) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return UserError{Kind: ErrorKindConflict, Message: "friend request already exists"}
		case "23503":
			return UserError{Kind: ErrorKindInvalidInput, Message: "profile not found"}
		case "23514", "22P02":
			return UserError{Kind: ErrorKindInvalidInput, Message: "invalid friend relationship"}
		}
	}
	return err
}
