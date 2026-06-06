package homefeed

import (
	"context"
	"time"
)

type Repository interface {
	VisibleFeedUserIDs(ctx context.Context, authToken, userID string) ([]string, error)
	HiddenMemoryIDs(ctx context.Context, authToken, userID string) (map[string]bool, error)
	HiddenUserIDs(ctx context.Context, authToken, userID string) (map[string]bool, error)
	ListMemories(ctx context.Context, authToken string, ownerUserIDs []string, limit int, before time.Time) ([]map[string]any, error)
	ListOfficialMemories(ctx context.Context, authToken string, limit int, before time.Time) ([]map[string]any, error)
}
