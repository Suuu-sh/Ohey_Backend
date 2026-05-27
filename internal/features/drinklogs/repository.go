package drinklogs

import (
	"context"
	"time"
)

type Repository interface {
	VisibleFeedUserIDs(ctx context.Context, authToken, userID string) ([]string, error)
	ListDrinkLogs(ctx context.Context, authToken string, ownerUserIDs []string) ([]map[string]any, error)
	ListOfficialDrinkLogs(ctx context.Context, authToken string) ([]map[string]any, error)
	HasDrinkLogInWindow(ctx context.Context, authToken, ownerUserID string, start, end time.Time) (bool, error)
	FriendshipExists(ctx context.Context, authToken, userID, friendID string) (bool, error)
	CreateDrinkLog(ctx context.Context, authToken string, log NewDrinkLog) (map[string]any, error)
	CreateDrinkLogFriendLinks(ctx context.Context, authToken, drinkLogID string, friendIDs []string) error
	DeleteOwnedDrinkLog(ctx context.Context, authToken, logID, ownerUserID string) (map[string]any, error)
	CreateLike(ctx context.Context, authToken, logID, userID string) (bool, error)
	DeleteLike(ctx context.Context, authToken, logID, userID string) error
	LikeState(ctx context.Context, authToken, logID, userID string) (LikeState, error)
	HiddenDrinkLogIDs(ctx context.Context, authToken, userID string) (map[string]bool, error)
	DrinkLogOwnerUserID(ctx context.Context, authToken, logID string) (string, error)
	FindReport(ctx context.Context, authToken, logID, reporterUserID string) (*Report, error)
	CreateReport(ctx context.Context, authToken string, report Report) error
}

type Notifier interface {
	DrinkLogTagged(ctx context.Context, authToken, logID, ownerUserID string, friendIDs []string)
	DrinkLogLiked(ctx context.Context, authToken, logID, actorUserID string)
}
