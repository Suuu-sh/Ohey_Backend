package wishitems

import "context"

type Repository interface {
	ListWishItems(ctx context.Context, authToken, ownerUserID string, limit int) ([]map[string]any, error)
	ListProfileWishItems(ctx context.Context, authToken, profileID string, limit int) ([]map[string]any, error)
	CreateWishItem(ctx context.Context, authToken string, item WishItem) (map[string]any, error)
	UpdateWishItem(ctx context.Context, authToken string, update WishItemUpdate) (map[string]any, error)
	DeleteWishItem(ctx context.Context, authToken, wishItemID, ownerUserID string) (map[string]any, error)
}
