package friendgroups

import "context"

type Repository interface {
	ListGroups(ctx context.Context, authToken, ownerUserID string) ([]FriendGroup, error)
	FriendshipExists(ctx context.Context, authToken, ownerUserID, friendUserID string) (bool, error)
	SaveGroups(ctx context.Context, authToken, ownerUserID string, groups []FriendGroup) ([]FriendGroup, error)
}
