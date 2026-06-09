package friendgroups

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepository struct{ pool *pgxpool.Pool }

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) ListGroups(ctx context.Context, _ string, ownerUserID string) ([]FriendGroup, error) {
	if r.pool == nil {
		return nil, errors.New("postgres pool is not configured")
	}
	rows, err := r.pool.Query(ctx, `select id::text,client_id,name,sort_order from friend_groups where owner_user_id=$1 order by sort_order asc,created_at asc`, ownerUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	groups := []FriendGroup{}
	rowIDs := []string{}
	for rows.Next() {
		var g FriendGroup
		if err := rows.Scan(&g.RowID, &g.ID, &g.Name, &g.SortOrder); err != nil {
			return nil, err
		}
		groups = append(groups, g)
		rowIDs = append(rowIDs, g.RowID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(groups) == 0 {
		return groups, nil
	}
	memberRows, err := r.pool.Query(ctx, `select group_id::text,friend_user_id::text from friend_group_members where group_id=any($1::uuid[]) order by sort_order asc,created_at asc`, rowIDs)
	if err != nil {
		return nil, err
	}
	defer memberRows.Close()
	members := map[string][]string{}
	for memberRows.Next() {
		var gid, fid string
		if err := memberRows.Scan(&gid, &fid); err != nil {
			return nil, err
		}
		members[gid] = append(members[gid], fid)
	}
	if err := memberRows.Err(); err != nil {
		return nil, err
	}
	for i := range groups {
		groups[i].FriendIDs = members[groups[i].RowID]
		groups[i].FriendIds = groups[i].FriendIDs
	}
	return groups, nil
}
func (r *PostgresRepository) FriendshipExists(ctx context.Context, _ string, ownerUserID, friendUserID string) (bool, error) {
	if r.pool == nil {
		return false, errors.New("postgres pool is not configured")
	}
	var ok bool
	err := r.pool.QueryRow(ctx, `select exists(select 1 from friendships where (user_a_id=$1 and user_b_id=$2) or (user_a_id=$2 and user_b_id=$1))`, ownerUserID, friendUserID).Scan(&ok)
	return ok, err
}
func (r *PostgresRepository) SaveGroups(ctx context.Context, _ string, ownerUserID string, groups []FriendGroup) ([]FriendGroup, error) {
	if r.pool == nil {
		return nil, errors.New("postgres pool is not configured")
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	keep := []string{}
	for _, g := range groups {
		keep = append(keep, g.ID)
	}
	if len(keep) == 0 {
		if _, err := tx.Exec(ctx, `delete from friend_groups where owner_user_id=$1`, ownerUserID); err != nil {
			return nil, err
		}
	} else {
		if _, err := tx.Exec(ctx, `delete from friend_groups where owner_user_id=$1 and not (client_id=any($2::text[]))`, ownerUserID, keep); err != nil {
			return nil, err
		}
	}
	for _, g := range groups {
		var rowID string
		err := tx.QueryRow(ctx, `insert into friend_groups (owner_user_id,client_id,name,sort_order,updated_at) values ($1,$2,$3,$4,now()) on conflict (owner_user_id,client_id) do update set name=excluded.name,sort_order=excluded.sort_order,updated_at=now() returning id::text`, ownerUserID, g.ID, g.Name, g.SortOrder).Scan(&rowID)
		if err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, `delete from friend_group_members where group_id=$1`, rowID); err != nil {
			return nil, err
		}
		for i, fid := range g.FriendIDs {
			if _, err := tx.Exec(ctx, `insert into friend_group_members (group_id,friend_user_id,sort_order) values ($1,$2,$3)`, rowID, fid, i); err != nil {
				return nil, err
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return r.ListGroups(ctx, "", ownerUserID)
}

var _ Repository = (*PostgresRepository)(nil)
var _ = pgx.ErrNoRows
