package memories

import (
	"context"
	"time"
)

type Repository interface {
	VisibleFeedUserIDs(ctx context.Context, authToken, userID string) ([]string, error)
	ListMemories(ctx context.Context, authToken string, ownerUserIDs []string) ([]map[string]any, error)
	ListOfficialMemories(ctx context.Context, authToken string) ([]map[string]any, error)
	HasMemoryInWindow(ctx context.Context, authToken, ownerUserID string, start, end time.Time) (bool, error)
	FriendshipExists(ctx context.Context, authToken, userID, friendID string) (bool, error)
	CreateMemory(ctx context.Context, authToken string, memory NewMemory) (map[string]any, error)
	CreateMemoryFriendLinks(ctx context.Context, authToken, memoryID string, friendIDs []string) error
	DeleteOwnedMemory(ctx context.Context, authToken, memoryID, ownerUserID string) (map[string]any, error)
	CreateLike(ctx context.Context, authToken, memoryID, userID string) (bool, error)
	DeleteLike(ctx context.Context, authToken, memoryID, userID string) error
	LikeState(ctx context.Context, authToken, memoryID, userID string) (LikeState, error)
	HiddenMemoryIDs(ctx context.Context, authToken, userID string) (map[string]bool, error)
	HiddenUserIDs(ctx context.Context, authToken, userID string) (map[string]bool, error)
	MemoryOwnerUserID(ctx context.Context, authToken, memoryID string) (string, error)
	FindReport(ctx context.Context, authToken, memoryID, reporterUserID string) (*Report, error)
	CreateReport(ctx context.Context, authToken string, report Report) error
}

type EventPublisher interface {
	Publish(ctx context.Context, authToken string, event DomainEvent)
}

type MediaCleaner interface {
	DeleteMemoryPhoto(ctx context.Context, photoPath string) error
}

type Logger interface {
	Warn(message string, args ...any)
}
