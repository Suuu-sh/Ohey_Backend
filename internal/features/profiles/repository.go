package profiles

import "context"

type Repository interface {
	GetByID(ctx context.Context, authToken, authUserID string) (*Profile, error)
	GetByUserID(ctx context.Context, authToken, userID string) (*Profile, error)
	GetByClerkUserID(ctx context.Context, authToken, clerkUserID string) (*Profile, error)
	UpsertBootstrap(ctx context.Context, authToken string, payload map[string]any) (map[string]any, error)
	PatchByID(ctx context.Context, authToken, authUserID string, payload map[string]any) ([]Profile, error)
}
