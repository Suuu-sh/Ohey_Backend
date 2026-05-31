package dailystatuses

import "context"

type Repository interface {
	GetDailyStatus(ctx context.Context, authToken, userID, statusDate string) ([]map[string]any, error)
	ListMonthlyStatuses(ctx context.Context, authToken, userID, startDate, endDate string) ([]map[string]any, error)
	FriendshipExists(ctx context.Context, authToken, userID, friendID string) (bool, error)
	UpsertDailyStatus(ctx context.Context, authToken string, status DailyStatus) ([]map[string]any, error)
}
