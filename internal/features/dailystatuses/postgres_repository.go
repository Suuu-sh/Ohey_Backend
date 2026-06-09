package dailystatuses

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepository struct{ pool *pgxpool.Pool }

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) GetDailyStatus(ctx context.Context, _ string, userID, statusDate string) ([]map[string]any, error) {
	if r.pool == nil {
		return nil, errors.New("postgres pool is not configured")
	}
	rows, err := r.pool.Query(ctx, `select user_id::text, status_date, status, updated_at from daily_statuses where user_id = $1 and status_date = $2`, userID, statusDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanDailyStatusRows(rows)
}

func (r *PostgresRepository) ListMonthlyStatuses(ctx context.Context, _ string, userID, startDate, endDate string) ([]map[string]any, error) {
	if r.pool == nil {
		return nil, errors.New("postgres pool is not configured")
	}
	rows, err := r.pool.Query(ctx, `select user_id::text, status_date, status, updated_at from daily_statuses where user_id = $1 and status_date >= $2 and status_date < $3 order by status_date asc`, userID, startDate, endDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanDailyStatusRows(rows)
}

func (r *PostgresRepository) FriendshipExists(ctx context.Context, _ string, userID, friendID string) (bool, error) {
	if r.pool == nil {
		return false, errors.New("postgres pool is not configured")
	}
	var exists bool
	err := r.pool.QueryRow(ctx, `select exists (select 1 from friendships where (user_a_id = $1 and user_b_id = $2) or (user_a_id = $2 and user_b_id = $1))`, userID, friendID).Scan(&exists)
	return exists, err
}

func (r *PostgresRepository) UpsertDailyStatus(ctx context.Context, _ string, status DailyStatus) ([]map[string]any, error) {
	if r.pool == nil {
		return nil, errors.New("postgres pool is not configured")
	}
	row := r.pool.QueryRow(ctx, `insert into daily_statuses (user_id, status_date, status, updated_at) values ($1, $2, $3, now()) on conflict (user_id, status_date) do update set status = excluded.status, updated_at = now() returning user_id::text, status_date, status, updated_at`, status.UserID, status.StatusDate, string(status.Status))
	item, err := scanDailyStatusRow(row)
	if err != nil {
		return nil, mapPostgresDailyStatusError(err)
	}
	return []map[string]any{item}, nil
}

func scanDailyStatusRows(rows pgx.Rows) ([]map[string]any, error) {
	out := []map[string]any{}
	for rows.Next() {
		item, err := scanDailyStatusRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func scanDailyStatusRow(row pgx.Row) (map[string]any, error) {
	var userID string
	var statusDate time.Time
	var status string
	var updatedAt time.Time
	if err := row.Scan(&userID, &statusDate, &status, &updatedAt); err != nil {
		return nil, err
	}
	return map[string]any{"user_id": userID, "status_date": statusDate.Format(time.DateOnly), "status": status, "updated_at": updatedAt.UTC().Format(time.RFC3339Nano)}, nil
}

func mapPostgresDailyStatusError(err error) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23503":
			return UserError{Kind: ErrorKindInvalidInput, Message: "profile not found"}
		case "23514", "22P02":
			return UserError{Kind: ErrorKindInvalidInput, Message: "invalid daily status"}
		}
	}
	return err
}
