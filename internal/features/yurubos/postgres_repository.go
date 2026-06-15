package yurubos

import (
	"context"
	"errors"
	"time"

	"github.com/Suuu-sh/Ohey_Backend/internal/contracts"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepository struct{ pool *pgxpool.Pool }

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) WishItemExists(ctx context.Context, _ string, ownerUserID, wishItemID string) (bool, error) {
	return r.exists(ctx, `select exists(select 1 from wish_items where id=$1 and owner_user_id=$2)`, wishItemID, ownerUserID)
}

func (r *PostgresRepository) CreateYurubo(ctx context.Context, _ string, item Yurubo) (map[string]any, error) {
	if r.pool == nil {
		return nil, errors.New("postgres pool is not configured")
	}
	expires := YuruboExpiresAt(item.StartsAt, time.Now().UTC())
	row := r.pool.QueryRow(ctx, yuruboReturningSQL(`insert into yurubos (owner_user_id,title,body,category,place_text,time_label,starts_at,visibility,wish_item_id,expires_at,updated_at) values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,now()) returning`), item.OwnerUserID, item.Title, item.Body, item.Category, item.PlaceText, item.TimeLabel, item.StartsAt, item.Visibility, item.WishItemID, expires)
	out, err := scanYuruboJoined(row)
	if err != nil {
		return nil, mapPostgresYuruboError(err)
	}
	return out, nil
}

func (r *PostgresRepository) LinkVisibilityGroup(ctx context.Context, _ string, ownerUserID, yuruboID, groupID string) (bool, error) {
	if r.pool == nil {
		return false, errors.New("postgres pool is not configured")
	}
	tag, err := r.pool.Exec(ctx, `insert into yurubo_visibility_groups (yurubo_id,group_id)
		select $1, fg.id
		from friend_groups fg
		where fg.id=$2 and fg.owner_user_id=$3
		on conflict do nothing`, yuruboID, groupID, ownerUserID)
	if err != nil {
		return false, mapPostgresYuruboError(err)
	}
	return tag.RowsAffected() > 0, nil
}

func (r *PostgresRepository) UpdateYurubo(ctx context.Context, _ string, update YuruboUpdate) (map[string]any, error) {
	if r.pool == nil {
		return nil, errors.New("postgres pool is not configured")
	}
	if update.StartsAtSet {
		row := r.pool.QueryRow(ctx, yuruboReturningSQL(`update yurubos set title=$3,body=$4,place_text=$5,time_label=$6,starts_at=$7,expires_at=$8,updated_at=now() where id=$1 and owner_user_id=$2 returning`), update.YuruboID, update.OwnerUserID, update.Title, update.Body, update.PlaceText, update.TimeLabel, update.StartsAt, YuruboExpiresAt(update.StartsAt, time.Now().UTC()))
		out, err := scanYuruboJoined(row)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		if err != nil {
			return nil, mapPostgresYuruboError(err)
		}
		return out, nil
	}
	row := r.pool.QueryRow(ctx, yuruboReturningSQL(`update yurubos set title=$3,body=$4,place_text=$5,time_label=$6,updated_at=now() where id=$1 and owner_user_id=$2 returning`), update.YuruboID, update.OwnerUserID, update.Title, update.Body, update.PlaceText, update.TimeLabel)
	out, err := scanYuruboJoined(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, mapPostgresYuruboError(err)
	}
	return out, nil
}

func (r *PostgresRepository) DeleteYurubo(ctx context.Context, _ string, yuruboID, ownerUserID string) (map[string]any, error) {
	if r.pool == nil {
		return nil, errors.New("postgres pool is not configured")
	}
	row := r.pool.QueryRow(ctx, yuruboReturningSQL(`delete from yurubos where id=$1 and owner_user_id=$2 returning`), yuruboID, ownerUserID)
	out, err := scanYuruboJoined(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, mapPostgresYuruboError(err)
	}
	return out, nil
}

func (r *PostgresRepository) HiddenYuruboIDs(ctx context.Context, _ string, userID string) (map[string]bool, error) {
	if r.pool == nil {
		return nil, errors.New("postgres pool is not configured")
	}
	rows, err := r.pool.Query(ctx, `select yurubo_id::text from hidden_yurubos where user_id=$1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}

func (r *PostgresRepository) ListOpenYurubos(ctx context.Context, authToken string, limit int) ([]map[string]any, error) {
	return r.ListOpenYurubosForViewer(ctx, authToken, "00000000-0000-0000-0000-000000000000", limit)
}
func (r *PostgresRepository) ListOpenYurubosForViewer(ctx context.Context, _ string, viewerUserID string, limit int) ([]map[string]any, error) {
	if r.pool == nil {
		return nil, errors.New("postgres pool is not configured")
	}
	rows, err := r.pool.Query(ctx, yuruboSelectSQL+` where y.status=$1 and y.expires_at > now() and (
		y.owner_user_id=$2
		or (
			not exists(
				select 1
				from user_blocks ub
				where (ub.blocker_user_id=$2 and ub.blocked_user_id=y.owner_user_id)
				   or (ub.blocker_user_id=y.owner_user_id and ub.blocked_user_id=$2)
			)
			and (
				(
					y.visibility=$3
					and exists(
						select 1
						from friendships f
						where (f.user_a_id=$2 and f.user_b_id=y.owner_user_id)
						   or (f.user_a_id=y.owner_user_id and f.user_b_id=$2)
					)
				)
				or (
					y.visibility=$4
					and exists(
						select 1
						from yurubo_visibility_groups yvg
						join friend_groups fg on fg.id=yvg.group_id and fg.owner_user_id=y.owner_user_id
						join friend_group_members fgm on fgm.group_id=fg.id
						where yvg.yurubo_id=y.id and fgm.friend_user_id=$2
					)
				)
			)
		)
	) order by y.created_at desc limit $5`, contracts.StatusOpen, viewerUserID, contracts.VisibilityFriends, contracts.VisibilityGroup, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		item, err := scanYuruboJoined(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) ListReactions(ctx context.Context, _ string, yuruboIDs []string) ([]map[string]any, error) {
	if r.pool == nil {
		return nil, errors.New("postgres pool is not configured")
	}
	if len(yuruboIDs) == 0 {
		return []map[string]any{}, nil
	}
	rows, err := r.pool.Query(ctx, `select yurubo_id::text,user_id::text,reaction_type from yurubo_reactions where yurubo_id=any($1::uuid[])`, yuruboIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var y, u, t string
		if err := rows.Scan(&y, &u, &t); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{"yurubo_id": y, "user_id": u, "reaction_type": t})
	}
	return out, rows.Err()
}
func (r *PostgresRepository) ParticipantProfiles(ctx context.Context, _ string, userIDs []string) (map[string]map[string]any, error) {
	out := map[string]map[string]any{}
	if r.pool == nil {
		return nil, errors.New("postgres pool is not configured")
	}
	if len(userIDs) == 0 {
		return out, nil
	}
	rows, err := r.pool.Query(ctx, `select id::text,user_id,display_name,avatar_url from profiles where id=any($1::uuid[])`, userIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, uid, name string
		var avatar *string
		if err := rows.Scan(&id, &uid, &name, &avatar); err != nil {
			return nil, err
		}
		m := map[string]any{"id": id, "user_id": uid, "display_name": name}
		if avatar != nil {
			m["avatar_url"] = *avatar
		}
		out[id] = m
	}
	return out, rows.Err()
}
func (r *PostgresRepository) OwnerID(ctx context.Context, _ string, yuruboID string) (string, error) {
	if r.pool == nil {
		return "", errors.New("postgres pool is not configured")
	}
	var id string
	err := r.pool.QueryRow(ctx, `select owner_user_id::text from yurubos where id=$1`, yuruboID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return id, err
}
func (r *PostgresRepository) VisibilityLabels(ctx context.Context, _ string, rows []map[string]any) (map[string]string, error) {
	labels := map[string]string{}
	if r.pool == nil {
		return labels, errors.New("postgres pool is not configured")
	}
	ids := []string{}
	for _, row := range rows {
		id, _ := row["id"].(string)
		vis, _ := row["visibility"].(string)
		if vis != contracts.VisibilityGroup {
			labels[id] = "全フレンズ"
		} else {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return labels, nil
	}
	q, err := r.pool.Query(ctx, `select yvg.yurubo_id::text, coalesce(fg.name,'グループ') from yurubo_visibility_groups yvg join friend_groups fg on fg.id=yvg.group_id where yvg.yurubo_id=any($1::uuid[])`, ids)
	if err != nil {
		for _, id := range ids {
			labels[id] = "グループ"
		}
		return labels, nil
	}
	defer q.Close()
	for q.Next() {
		var id, name string
		if err := q.Scan(&id, &name); err != nil {
			return nil, err
		}
		labels[id] = name
	}
	return labels, q.Err()
}
func (r *PostgresRepository) UpsertReaction(ctx context.Context, _ string, reaction Reaction) (bool, error) {
	if r.pool == nil {
		return false, errors.New("postgres pool is not configured")
	}
	tag, err := r.pool.Exec(ctx, `insert into yurubo_reactions (yurubo_id,user_id,reaction_type,updated_at)
		select y.id,$2,$3,now()
		from yurubos y
		where y.id=$1 and y.status=$4 and y.expires_at > now() and (
			y.owner_user_id=$2
			or (
				not exists(
					select 1
					from user_blocks ub
					where (ub.blocker_user_id=$2 and ub.blocked_user_id=y.owner_user_id)
					   or (ub.blocker_user_id=y.owner_user_id and ub.blocked_user_id=$2)
				)
				and (
					(
						y.visibility=$5
						and exists(
							select 1
							from friendships f
							where (f.user_a_id=$2 and f.user_b_id=y.owner_user_id)
							   or (f.user_a_id=y.owner_user_id and f.user_b_id=$2)
						)
					)
					or (
						y.visibility=$6
						and exists(
							select 1
							from yurubo_visibility_groups yvg
							join friend_groups fg on fg.id=yvg.group_id and fg.owner_user_id=y.owner_user_id
							join friend_group_members fgm on fgm.group_id=fg.id
							where yvg.yurubo_id=y.id and fgm.friend_user_id=$2
						)
					)
				)
			)
		)
		on conflict (yurubo_id,user_id) do update set reaction_type=excluded.reaction_type, updated_at=now()`, reaction.YuruboID, reaction.UserID, reaction.ReactionType, contracts.StatusOpen, contracts.VisibilityFriends, contracts.VisibilityGroup)
	if err != nil {
		return false, mapPostgresYuruboError(err)
	}
	return tag.RowsAffected() > 0, nil
}
func (r *PostgresRepository) ApproveReaction(ctx context.Context, _ string, ownerUserID, yuruboID, participantID string) (bool, error) {
	if r.pool == nil {
		return false, errors.New("postgres pool is not configured")
	}
	tag, err := r.pool.Exec(ctx, `update yurubo_reactions set reaction_type=$4,updated_at=now() where yurubo_id=$1 and user_id=$2 and exists(select 1 from yurubos y where y.id=$1 and y.owner_user_id=$3)`, yuruboID, participantID, ownerUserID, contracts.ReactionTypeAvailable)
	if err != nil {
		return false, mapPostgresYuruboError(err)
	}
	return tag.RowsAffected() > 0, nil
}
func (r *PostgresRepository) DeleteReaction(ctx context.Context, _ string, yuruboID, userID string) error {
	if r.pool == nil {
		return errors.New("postgres pool is not configured")
	}
	_, err := r.pool.Exec(ctx, `delete from yurubo_reactions where yurubo_id=$1 and user_id=$2`, yuruboID, userID)
	return err
}
func (r *PostgresRepository) exists(ctx context.Context, sql string, args ...any) (bool, error) {
	if r.pool == nil {
		return false, errors.New("postgres pool is not configured")
	}
	var ok bool
	err := r.pool.QueryRow(ctx, sql, args...).Scan(&ok)
	return ok, err
}

const yuruboSelectSQL = `select y.id::text,y.wish_item_id::text,y.owner_user_id::text,y.title,y.body,y.category,y.place_text,y.place_lat,y.place_lng,y.time_label,y.starts_at,y.ends_at,y.status,y.visibility,y.expires_at,y.created_at,y.updated_at,o.id::text,o.user_id,o.display_name,o.character_key,o.avatar_url,o.is_plus from yurubos y join profiles o on o.id=y.owner_user_id`

func yuruboReturningSQL(prefix string) string {
	return prefix + ` id::text,wish_item_id::text,owner_user_id::text,title,body,category,place_text,place_lat,place_lng,time_label,starts_at,ends_at,status,visibility,expires_at,created_at,updated_at,(select id::text from profiles where id=owner_user_id),(select user_id from profiles where id=owner_user_id),(select display_name from profiles where id=owner_user_id),(select character_key from profiles where id=owner_user_id),(select avatar_url from profiles where id=owner_user_id),(select is_plus from profiles where id=owner_user_id)`
}

func scanYuruboJoined(row pgx.Row) (map[string]any, error) {
	var id, owner, title, body, cat, place, timeLabel, status, vis string
	var wish pgtype.Text
	var lat, lng pgtype.Float8
	var starts, ends, expires pgtype.Timestamptz
	var created, updated time.Time
	var oid, ouid, oname, ock string
	var oavatar pgtype.Text
	var oplus bool
	if err := row.Scan(&id, &wish, &owner, &title, &body, &cat, &place, &lat, &lng, &timeLabel, &starts, &ends, &status, &vis, &expires, &created, &updated, &oid, &ouid, &oname, &ock, &oavatar, &oplus); err != nil {
		return nil, err
	}
	m := map[string]any{"id": id, "owner_user_id": owner, "title": title, "body": body, "category": cat, "place_text": place, "time_label": timeLabel, "status": status, "visibility": vis, "created_at": created.UTC().Format(time.RFC3339Nano), "updated_at": updated.UTC().Format(time.RFC3339Nano), "owner": map[string]any{"id": oid, "user_id": ouid, "display_name": oname, "character_key": ock, "is_plus": oplus}}
	if wish.Valid {
		m["wish_item_id"] = wish.String
	}
	if lat.Valid {
		m["place_lat"] = lat.Float64
	}
	if lng.Valid {
		m["place_lng"] = lng.Float64
	}
	if starts.Valid {
		m["starts_at"] = starts.Time.UTC().Format(time.RFC3339Nano)
	}
	if ends.Valid {
		m["ends_at"] = ends.Time.UTC().Format(time.RFC3339Nano)
	}
	if expires.Valid {
		m["expires_at"] = expires.Time.UTC().Format(time.RFC3339Nano)
	}
	if oavatar.Valid {
		m["owner"].(map[string]any)["avatar_url"] = oavatar.String
	}
	return m, nil
}
func mapPostgresYuruboError(err error) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23503":
			return UserError{Kind: ErrorKindInvalidInput, Message: "related record not found"}
		case "23514", "22P02":
			return UserError{Kind: ErrorKindInvalidInput, Message: "invalid yurubo"}
		}
	}
	return err
}
