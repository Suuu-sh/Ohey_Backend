package yurubos

import "context"

type Repository interface {
	WishItemExists(ctx context.Context, authToken, ownerUserID, wishItemID string) (bool, error)
	CreateYurubo(ctx context.Context, authToken string, item Yurubo) (map[string]any, error)
	LinkVisibilityGroup(ctx context.Context, authToken, yuruboID, groupID string) error
	UpdateYurubo(ctx context.Context, authToken string, update YuruboUpdate) (map[string]any, error)
	DeleteYurubo(ctx context.Context, authToken, yuruboID, ownerUserID string) (map[string]any, error)
	HiddenYuruboIDs(ctx context.Context, authToken, userID string) (map[string]bool, error)
	ListOpenYurubos(ctx context.Context, authToken string, limit int) ([]map[string]any, error)
	ListReactions(ctx context.Context, authToken string, yuruboIDs []string) ([]map[string]any, error)
	ParticipantProfiles(ctx context.Context, authToken string, userIDs []string) (map[string]map[string]any, error)
	OwnerID(ctx context.Context, authToken, yuruboID string) (string, error)
	VisibilityLabels(ctx context.Context, authToken string, rows []map[string]any) (map[string]string, error)
	UpsertReaction(ctx context.Context, authToken string, reaction Reaction) error
	ApproveReaction(ctx context.Context, authToken, ownerUserID, yuruboID, participantID string) (bool, error)
	DeleteReaction(ctx context.Context, authToken, yuruboID, userID string) error
}
