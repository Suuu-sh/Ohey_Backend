package usersafety

import (
	"context"
	"errors"
	"time"

	"github.com/Suuu-sh/Ohey_Backend/internal/contracts"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepository struct{ pool *pgxpool.Pool }

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) ListBlockedUsers(ctx context.Context, _ string, userID string) ([]map[string]any, error) {
	return r.listRelations(ctx, `select ub.blocked_user_id::text,ub.created_at,p.id::text,p.user_id,p.display_name,p.character_key,p.avatar_url,p.is_plus from user_blocks ub left join profiles p on p.id=ub.blocked_user_id where ub.blocker_user_id=$1 order by ub.created_at desc`, userID)
}
func (r *PostgresRepository) BlockUser(ctx context.Context, _ string, rel UserRelation) (map[string]any, error) {
	return r.upsertRelation(ctx, `insert into user_blocks (blocker_user_id,blocked_user_id) values ($1,$2) on conflict (blocker_user_id,blocked_user_id) do update set blocker_user_id=excluded.blocker_user_id returning blocker_user_id::text,blocked_user_id::text,created_at`, "blocker_user_id", "blocked_user_id", rel)
}
func (r *PostgresRepository) UnblockUser(ctx context.Context, _ string, rel UserRelation) error {
	if r.pool == nil {
		return errors.New("postgres pool is not configured")
	}
	_, err := r.pool.Exec(ctx, `delete from user_blocks where blocker_user_id=$1 and blocked_user_id=$2`, rel.ActorUserID, rel.TargetUserID)
	return err
}
func (r *PostgresRepository) ListMutedUsers(ctx context.Context, _ string, userID string) ([]map[string]any, error) {
	return r.listRelations(ctx, `select um.muted_user_id::text,um.created_at,p.id::text,p.user_id,p.display_name,p.character_key,p.avatar_url,p.is_plus from user_mutes um left join profiles p on p.id=um.muted_user_id where um.muter_user_id=$1 order by um.created_at desc`, userID)
}
func (r *PostgresRepository) MuteUser(ctx context.Context, _ string, rel UserRelation) (map[string]any, error) {
	return r.upsertRelation(ctx, `insert into user_mutes (muter_user_id,muted_user_id) values ($1,$2) on conflict (muter_user_id,muted_user_id) do update set muter_user_id=excluded.muter_user_id returning muter_user_id::text,muted_user_id::text,created_at`, "muter_user_id", "muted_user_id", rel)
}
func (r *PostgresRepository) UnmuteUser(ctx context.Context, _ string, rel UserRelation) error {
	if r.pool == nil {
		return errors.New("postgres pool is not configured")
	}
	_, err := r.pool.Exec(ctx, `delete from user_mutes where muter_user_id=$1 and muted_user_id=$2`, rel.ActorUserID, rel.TargetUserID)
	return err
}
func (r *PostgresRepository) ReportUser(ctx context.Context, _ string, report UserReport) (map[string]any, error) {
	if r.pool == nil {
		return nil, errors.New("postgres pool is not configured")
	}
	var reporter, reported, reason, status string
	var created, updated time.Time
	err := r.pool.QueryRow(ctx, `insert into user_reports (reporter_user_id,reported_user_id,reason,status,updated_at) values ($1,$2,$3,$4,now()) on conflict (reporter_user_id,reported_user_id) do update set reason=excluded.reason,status=excluded.status,updated_at=now() returning reporter_user_id::text,reported_user_id::text,reason,status,created_at,updated_at`, report.ReporterUserID, report.ReportedUserID, report.Reason, contracts.StatusPending).Scan(&reporter, &reported, &reason, &status, &created, &updated)
	if err != nil {
		return nil, err
	}
	return map[string]any{"reporter_user_id": reporter, "reported_user_id": reported, "reason": reason, "status": status, "created_at": created.UTC().Format(time.RFC3339Nano), "updated_at": updated.UTC().Format(time.RFC3339Nano)}, nil
}
func (r *PostgresRepository) CleanupBlockedRelations(ctx context.Context, rel UserRelation) error {
	if r.pool == nil {
		return errors.New("postgres pool is not configured")
	}
	now := time.Now().UTC()
	_, err := r.pool.Exec(ctx, `delete from friendships where (user_a_id=$1 and user_b_id=$2) or (user_a_id=$2 and user_b_id=$1); update friend_requests set status=case when from_user_id=$1 then $3 else $4 end, responded_at=$5 where status=$6 and ((from_user_id=$1 and to_user_id=$2) or (from_user_id=$2 and to_user_id=$1)); update invites set status=case when inviter_user_id=$1 then $3 else $4 end, responded_at=$5 where status=$6 and ((inviter_user_id=$1 and invitee_user_id=$2) or (inviter_user_id=$2 and invitee_user_id=$1));`, rel.ActorUserID, rel.TargetUserID, contracts.StatusCancelled, contracts.StatusRejected, now, contracts.StatusPending)
	return err
}

func (r *PostgresRepository) listRelations(ctx context.Context, sql, userID string) ([]map[string]any, error) {
	if r.pool == nil {
		return nil, errors.New("postgres pool is not configured")
	}
	rows, err := r.pool.Query(ctx, sql, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var target string
		var created time.Time
		var id, uid, name, ck string
		var avatar *string
		var plus bool
		if err := rows.Scan(&target, &created, &id, &uid, &name, &ck, &avatar, &plus); err != nil {
			return nil, err
		}
		m := map[string]any{"id": id, "user_id": uid, "display_name": name, "character_key": ck, "is_plus": plus, "target_user_id": target, "created_at": created.UTC().Format(time.RFC3339Nano)}
		if avatar != nil {
			m["avatar_url"] = *avatar
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
func (r *PostgresRepository) upsertRelation(ctx context.Context, sql, actorKey, targetKey string, rel UserRelation) (map[string]any, error) {
	if r.pool == nil {
		return nil, errors.New("postgres pool is not configured")
	}
	var actor, target string
	var created time.Time
	err := r.pool.QueryRow(ctx, sql, rel.ActorUserID, rel.TargetUserID).Scan(&actor, &target, &created)
	if err != nil {
		return nil, err
	}
	return map[string]any{actorKey: actor, targetKey: target, "created_at": created.UTC().Format(time.RFC3339Nano)}, nil
}

var _ Repository = (*PostgresRepository)(nil)
var _ = pgx.ErrNoRows
