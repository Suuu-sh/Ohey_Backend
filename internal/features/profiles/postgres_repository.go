package profiles

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) GetByID(ctx context.Context, _ string, authUserID string) (*Profile, error) {
	return r.getOne(ctx, `
		select id::text, user_id, display_name, character_key, avatar_url, is_plus
		from profiles
		where id = $1
		limit 1`, authUserID)
}

func (r *PostgresRepository) GetByUserID(ctx context.Context, _ string, userID string) (*Profile, error) {
	return r.getOne(ctx, `
		select id::text, user_id, display_name, character_key, avatar_url, is_plus
		from profiles
		where user_id = $1
		limit 1`, userID)
}

func (r *PostgresRepository) GetByClerkUserID(ctx context.Context, _ string, clerkUserID string) (*Profile, error) {
	return r.getOne(ctx, `
		select id::text, user_id, display_name, character_key, avatar_url, is_plus
		from profiles
		where clerk_user_id = $1
		limit 1`, clerkUserID)
}

func (r *PostgresRepository) UpsertBootstrap(ctx context.Context, _ string, payload map[string]any) (map[string]any, error) {
	if r.pool == nil {
		return nil, errors.New("postgres pool is not configured")
	}
	id, _ := payloadString(payload, "id")
	clerkUserID, _ := payloadString(payload, "clerk_user_id")
	userID, _ := payloadString(payload, "user_id")
	displayName, _ := payloadString(payload, "display_name")
	characterKey, _ := payloadString(payload, "character_key")
	avatarURL, _ := payloadNullableString(payload, "avatar_url")
	isPlus, _ := payloadBool(payload, "is_plus")

	var row pgx.Row
	if strings.TrimSpace(id) == "" {
		row = r.pool.QueryRow(ctx, `
			insert into profiles (clerk_user_id, user_id, display_name, character_key, avatar_url, is_plus, updated_at)
			values (nullif($1, ''), $2, $3, $4, $5, $6, now())
			on conflict (id) do update set
				clerk_user_id = excluded.clerk_user_id,
				user_id = excluded.user_id,
				display_name = excluded.display_name,
				character_key = excluded.character_key,
				avatar_url = excluded.avatar_url,
				updated_at = now()
			returning id::text, user_id, display_name, character_key, avatar_url, is_plus`,
			clerkUserID, userID, displayName, characterKey, avatarURL, isPlus)
	} else {
		row = r.pool.QueryRow(ctx, `
			insert into profiles (id, clerk_user_id, user_id, display_name, character_key, avatar_url, is_plus, updated_at)
			values ($1, nullif($2, ''), $3, $4, $5, $6, $7, now())
			on conflict (id) do update set
				clerk_user_id = excluded.clerk_user_id,
				user_id = excluded.user_id,
				display_name = excluded.display_name,
				character_key = excluded.character_key,
				avatar_url = excluded.avatar_url,
				updated_at = now()
			returning id::text, user_id, display_name, character_key, avatar_url, is_plus`,
			id, clerkUserID, userID, displayName, characterKey, avatarURL, isPlus)
	}
	profile, err := scanProfile(row)
	if err != nil {
		return nil, mapPostgresProfileError(err)
	}
	return profileMap(profile), nil
}

func (r *PostgresRepository) PatchByID(ctx context.Context, _ string, authUserID string, payload map[string]any) ([]Profile, error) {
	if r.pool == nil {
		return nil, errors.New("postgres pool is not configured")
	}
	current, err := r.GetByID(ctx, "", authUserID)
	if err != nil || current == nil {
		return nil, mapPostgresProfileError(err)
	}
	userID := current.UserID
	if value, ok := payloadString(payload, "user_id"); ok {
		userID = value
	}
	displayName := current.DisplayName
	if value, ok := payloadString(payload, "display_name"); ok {
		displayName = value
	}
	characterKey := current.CharacterKey
	if value, ok := payloadString(payload, "character_key"); ok {
		characterKey = value
	}
	avatarURL := nullableString(current.AvatarURL)
	if value, ok := payloadNullableString(payload, "avatar_url"); ok {
		avatarURL = value
	}
	profile, err := scanProfile(r.pool.QueryRow(ctx, `
		update profiles
		set user_id = $2,
			display_name = $3,
			character_key = $4,
			avatar_url = $5,
			updated_at = now()
		where id = $1
		returning id::text, user_id, display_name, character_key, avatar_url, is_plus`,
		authUserID, userID, displayName, characterKey, avatarURL))
	if err != nil {
		return nil, mapPostgresProfileError(err)
	}
	return []Profile{*profile}, nil
}

func (r *PostgresRepository) getOne(ctx context.Context, sql string, args ...any) (*Profile, error) {
	if r.pool == nil {
		return nil, errors.New("postgres pool is not configured")
	}
	profile, err := scanProfile(r.pool.QueryRow(ctx, sql, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return profile, nil
}

func scanProfile(row pgx.Row) (*Profile, error) {
	var profile Profile
	var avatar pgtype.Text
	if err := row.Scan(&profile.ID, &profile.UserID, &profile.DisplayName, &profile.CharacterKey, &avatar, &profile.IsPlus); err != nil {
		return nil, err
	}
	if avatar.Valid {
		profile.AvatarURL = avatar.String
	}
	return &profile, nil
}

func payloadString(payload map[string]any, key string) (string, bool) {
	value, ok := payload[key]
	if !ok || value == nil {
		return "", false
	}
	return fmt.Sprint(value), true
}

func payloadNullableString(payload map[string]any, key string) (*string, bool) {
	value, ok := payload[key]
	if !ok || value == nil {
		return nil, ok
	}
	text := fmt.Sprint(value)
	return &text, true
}

func payloadBool(payload map[string]any, key string) (bool, bool) {
	value, ok := payload[key]
	if !ok || value == nil {
		return false, false
	}
	boolValue, ok := value.(bool)
	return boolValue, ok
}

func nullableString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func profileMap(profile *Profile) map[string]any {
	out := map[string]any{
		"id":            profile.ID,
		"user_id":       profile.UserID,
		"display_name":  profile.DisplayName,
		"character_key": profile.CharacterKey,
		"is_plus":       profile.IsPlus,
	}
	if profile.AvatarURL != "" {
		out["avatar_url"] = profile.AvatarURL
	}
	return out
}

func mapPostgresProfileError(err error) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return UserError{Kind: ErrorKindInvalidInput, Message: "profile already exists"}
		case "23514", "22P02":
			return UserError{Kind: ErrorKindInvalidInput, Message: "invalid profile"}
		}
	}
	return err
}
