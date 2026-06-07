package friends

import (
	"context"
	"time"
)

type Repository interface {
	ListFriendships(ctx context.Context, authToken, userID string) ([]map[string]any, error)
	AttachTodayStatuses(ctx context.Context, authToken string, rows []map[string]any, date string) error
	UpdateFriendFavorite(ctx context.Context, authToken, userID, friendID string, isFavorite bool) (map[string]any, error)
	UpsertFriendshipPair(ctx context.Context, authToken, userA, userB string) (map[string]any, error)
	DeleteFriendship(ctx context.Context, authToken, userID, friendID string) (map[string]any, error)
	FriendshipExists(ctx context.Context, authToken, userID, friendID string) (bool, error)
	BlockExistsBetweenUsers(ctx context.Context, authToken, userID, friendID string) (bool, error)
	ListPendingFriendRequests(ctx context.Context, authToken, userID string, direction RequestDirection) ([]map[string]any, error)
	PendingFriendRequestBetween(ctx context.Context, authToken, userID, friendID string) (map[string]any, error)
	CreateFriendRequest(ctx context.Context, authToken, fromUserID, toUserID string) (map[string]any, error)
	UpdatePendingFriendRequestStatus(ctx context.Context, authToken, requestID, userID string, status RequestStatus, respondedAt time.Time) (map[string]any, error)
}

type EventPublisher interface {
	Publish(ctx context.Context, authToken string, event DomainEvent)
}

type Logger interface {
	Warn(message string, args ...any)
}
