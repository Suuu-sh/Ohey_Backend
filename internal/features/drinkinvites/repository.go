package drinkinvites

import (
	"context"
	"time"
)

type Repository interface {
	ListTodayReservations(ctx context.Context, authToken, userID, inviteDate string) ([]map[string]any, error)
	ListIncomingPending(ctx context.Context, authToken, userID, inviteDate string) ([]map[string]any, error)
	ListOutgoingActive(ctx context.Context, authToken, userID, inviteDate string) ([]map[string]any, error)
	DailyStatus(ctx context.Context, authToken, userID, statusDate string) (string, error)
	FindActiveInviteBetweenUsersForDate(ctx context.Context, authToken, fromUserID, toUserID, inviteDate string) (*ExistingInvite, error)
	CreateInvite(ctx context.Context, authToken string, invite NewInvite) (map[string]any, error)
	UpdatePendingInviteStatus(ctx context.Context, authToken, inviteID, recipientUserID string, status InviteStatus, respondedAt time.Time) (map[string]any, error)
}

type EventPublisher interface {
	Publish(ctx context.Context, authToken string, event DomainEvent)
}
