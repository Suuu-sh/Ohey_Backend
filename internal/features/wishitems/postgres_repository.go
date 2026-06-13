package wishitems

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

func (r *PostgresRepository) ListWishItems(ctx context.Context, _ string, ownerUserID string, limit int) ([]map[string]any, error) {
	if r.pool == nil {
		return nil, errors.New("postgres pool is not configured")
	}
	rows, err := r.pool.Query(ctx, wishItemSelectSQL+` where owner_user_id = $1 and status = $2 order by created_at desc limit $3`, ownerUserID, contracts.StatusActive, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanWishItemRows(rows)
}

func (r *PostgresRepository) ListProfileWishItems(ctx context.Context, authToken, profileID string, limit int) ([]map[string]any, error) {
	return r.ListProfileWishItemsForViewer(ctx, authToken, "00000000-0000-0000-0000-000000000000", profileID, limit)
}

func (r *PostgresRepository) ListProfileWishItemsForViewer(ctx context.Context, _ string, viewerUserID, profileID string, limit int) ([]map[string]any, error) {
	if r.pool == nil {
		return nil, errors.New("postgres pool is not configured")
	}
	rows, err := r.pool.Query(ctx, wishItemSelectSQL+`
		where owner_user_id = $1
		  and visibility = $2
		  and status = $3
		  and (
			$4::uuid = owner_user_id
			or exists (
				select 1 from friendships f
				where (f.user_a_id = $4::uuid and f.user_b_id = owner_user_id)
				   or (f.user_b_id = $4::uuid and f.user_a_id = owner_user_id)
			)
		  )
		order by created_at desc
		limit $5`, profileID, contracts.VisibilityFriends, contracts.StatusActive, viewerUserID, limit)
	if err != nil {
		return nil, mapPostgresWishItemError(err)
	}
	defer rows.Close()
	return scanWishItemRows(rows)
}

func (r *PostgresRepository) CreateWishItem(ctx context.Context, _ string, item WishItem) (map[string]any, error) {
	if r.pool == nil {
		return nil, errors.New("postgres pool is not configured")
	}
	row := r.pool.QueryRow(ctx, `
		insert into wish_items (owner_user_id, title, note, category, place_text, place_url, visibility, updated_at)
		values ($1, $2, $3, $4, $5, $6, $7, now())
		returning id::text, owner_user_id::text, title, note, category, place_text, place_url, visibility, status, created_at, updated_at`,
		item.OwnerUserID, item.Title, item.Note, item.Category, item.PlaceText, item.PlaceURL, item.Visibility)
	out, err := scanWishItemRow(row)
	if err != nil {
		return nil, mapPostgresWishItemError(err)
	}
	return out, nil
}

func (r *PostgresRepository) UpdateWishItem(ctx context.Context, _ string, update WishItemUpdate) (map[string]any, error) {
	if r.pool == nil {
		return nil, errors.New("postgres pool is not configured")
	}
	row := r.pool.QueryRow(ctx, `
		update wish_items
		set title = $3, note = $4, category = $5, place_text = $6, place_url = $7, visibility = $8, updated_at = now()
		where id = $1 and owner_user_id = $2
		returning id::text, owner_user_id::text, title, note, category, place_text, place_url, visibility, status, created_at, updated_at`,
		update.WishItemID, update.OwnerUserID, update.Title, update.Note, update.Category, update.PlaceText, update.PlaceURL, update.Visibility)
	out, err := scanWishItemRow(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, mapPostgresWishItemError(err)
	}
	return out, nil
}

func (r *PostgresRepository) DeleteWishItem(ctx context.Context, _ string, wishItemID, ownerUserID string) (map[string]any, error) {
	if r.pool == nil {
		return nil, errors.New("postgres pool is not configured")
	}
	row := r.pool.QueryRow(ctx, `
		delete from wish_items
		where id = $1 and owner_user_id = $2
		returning id::text, owner_user_id::text, title, note, category, place_text, place_url, visibility, status, created_at, updated_at`, wishItemID, ownerUserID)
	out, err := scanWishItemRow(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, mapPostgresWishItemError(err)
	}
	return out, nil
}

const wishItemSelectSQL = `select id::text, owner_user_id::text, title, note, category, place_text, place_url, visibility, status, created_at, updated_at from wish_items`

func scanWishItemRows(rows pgx.Rows) ([]map[string]any, error) {
	out := []map[string]any{}
	for rows.Next() {
		item, err := scanWishItemRow(rows)
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

func scanWishItemRow(row pgx.Row) (map[string]any, error) {
	var id, ownerUserID, title, note, category, placeText, placeURL, visibility, status string
	var createdAt, updatedAt time.Time
	if err := row.Scan(&id, &ownerUserID, &title, &note, &category, &placeText, &placeURL, &visibility, &status, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	return map[string]any{"id": id, "owner_user_id": ownerUserID, "title": title, "note": note, "category": category, "place_text": placeText, "place_url": placeURL, "visibility": visibility, "status": status, "created_at": createdAt.UTC().Format(time.RFC3339Nano), "updated_at": updatedAt.UTC().Format(time.RFC3339Nano)}, nil
}

func mapPostgresWishItemError(err error) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23503":
			return UserError{Kind: ErrorKindInvalidInput, Message: "profile not found"}
		case "23514", "22P02":
			return UserError{Kind: ErrorKindInvalidInput, Message: "invalid wish item"}
		}
	}
	return err
}
