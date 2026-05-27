package homefeed

import "context"

type Repository interface {
	VisibleFeedUserIDs(ctx context.Context, authToken, userID string) ([]string, error)
	HiddenDrinkLogIDs(ctx context.Context, authToken, userID string) (map[string]bool, error)
	ListDrinkLogs(ctx context.Context, authToken string, ownerUserIDs []string) ([]map[string]any, error)
	ListOfficialDrinkLogs(ctx context.Context, authToken string) ([]map[string]any, error)
}
