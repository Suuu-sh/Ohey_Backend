package usersafety

import "context"

type Repository interface {
	ListBlockedUsers(ctx context.Context, authToken, userID string) ([]map[string]any, error)
	BlockUser(ctx context.Context, authToken string, relation UserRelation) (map[string]any, error)
	UnblockUser(ctx context.Context, authToken string, relation UserRelation) error
	ListMutedUsers(ctx context.Context, authToken, userID string) ([]map[string]any, error)
	MuteUser(ctx context.Context, authToken string, relation UserRelation) (map[string]any, error)
	UnmuteUser(ctx context.Context, authToken string, relation UserRelation) error
	ReportUser(ctx context.Context, authToken string, report UserReport) (map[string]any, error)
	HideMemory(ctx context.Context, authToken string, hidden HiddenMemory) (map[string]any, error)
	UnhideMemory(ctx context.Context, authToken string, hidden HiddenMemory) error
	CleanupBlockedRelations(ctx context.Context, relation UserRelation) error
}
