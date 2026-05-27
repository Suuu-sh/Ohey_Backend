package notifications

import (
	"context"
	"time"
)

type Repository interface {
	CreateNotification(ctx context.Context, notification Notification) (bool, error)
	ListNotifications(ctx context.Context, authToken, recipientUserID string, limit int) ([]map[string]any, error)
	MarkAllRead(ctx context.Context, authToken, recipientUserID string, readAt time.Time) (int, error)
	DisplayName(ctx context.Context, authToken, userID string) (string, error)
	DrinkLogOwnerUserID(ctx context.Context, authToken, logID string) (string, error)
	TodayAcceptedInvites(ctx context.Context, authToken, userID, date string) ([]DrinkInvite, error)
	AllProfileIDs(ctx context.Context) ([]string, error)
	PushTokens(ctx context.Context, recipientUserID string) ([]string, error)
}

type PushSender interface {
	Send(ctx context.Context, token, title, body string, data map[string]string) error
}

type Logger interface {
	Warn(message string, args ...any)
}
