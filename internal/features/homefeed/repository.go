package homefeed

import "context"

type Repository interface {
	VisibleFeedUserIDs(ctx context.Context, authToken, userID string) ([]string, error)
	HiddenMemoryIDs(ctx context.Context, authToken, userID string) (map[string]bool, error)
	HiddenUserIDs(ctx context.Context, authToken, userID string) (map[string]bool, error)
	ListMemories(ctx context.Context, authToken string, ownerUserIDs []string) ([]map[string]any, error)
	ListOfficialMemories(ctx context.Context, authToken string) ([]map[string]any, error)
}
