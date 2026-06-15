package yurubos

import (
	"context"

	"github.com/Suuu-sh/Ohey_Backend/internal/contracts"
)

type Dependencies struct {
	Repository Repository
	Publisher  EventPublisher
}

type Usecase struct {
	repository Repository
	publisher  EventPublisher
}

func NewUsecase(deps Dependencies) *Usecase {
	return &Usecase{repository: deps.Repository, publisher: deps.Publisher}
}

type ListInput struct {
	AuthToken string
	UserID    string
	Limit     string
}

type CreateInput struct {
	AuthToken   string
	OwnerUserID string
	Body        CreateBody
}

type UpdateInput struct {
	AuthToken   string
	YuruboID    string
	OwnerUserID string
	Body        UpdateBody
}

type DeleteInput struct {
	AuthToken   string
	YuruboID    string
	OwnerUserID string
}

type ReactionInput struct {
	AuthToken string
	YuruboID  string
	UserID    string
	Body      ReactionBody
}

type ApprovalInput struct {
	AuthToken     string
	YuruboID      string
	OwnerUserID   string
	ParticipantID string
}

func (u *Usecase) CreateYurubo(ctx context.Context, input CreateInput) (map[string]any, error) {
	item, groupID, err := NewYurubo(input.OwnerUserID, input.Body)
	if err != nil {
		return nil, err
	}
	if item.WishItemID != nil {
		exists, err := u.repository.WishItemExists(ctx, input.AuthToken, item.OwnerUserID, *item.WishItemID)
		if err != nil {
			return nil, err
		}
		if !exists {
			return nil, UserError{Kind: ErrorKindInvalidInput, Message: "wish item not found"}
		}
	}
	row, err := u.repository.CreateYurubo(ctx, input.AuthToken, item)
	if err != nil {
		return nil, err
	}
	groupIDs := []string{}
	if item.Visibility == contracts.VisibilityGroup {
		yuruboID, _ := row["id"].(string)
		if yuruboID == "" {
			return nil, UserError{Kind: ErrorKindUpstream, Message: "yurubo insert returned no id"}
		}
		linked, err := u.repository.LinkVisibilityGroup(ctx, input.AuthToken, item.OwnerUserID, yuruboID, groupID)
		if err != nil {
			return nil, err
		}
		if !linked {
			return nil, UserError{Kind: ErrorKindInvalidInput, Message: "group not found"}
		}
		groupIDs = append(groupIDs, groupID)
	}
	if u.publisher != nil {
		u.publisher.Publish(ctx, input.AuthToken, DomainEvent{Kind: EventYuruboCreated, Yurubo: item, Row: row, GroupIDs: groupIDs})
	}
	return row, nil
}

func (u *Usecase) UpdateYurubo(ctx context.Context, input UpdateInput) (map[string]any, error) {
	update, err := NewYuruboUpdate(input.YuruboID, input.OwnerUserID, input.Body)
	if err != nil {
		return nil, err
	}
	row, err := u.repository.UpdateYurubo(ctx, input.AuthToken, update)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, UserError{Kind: ErrorKindNotFound, Message: "yurubo not found"}
	}
	return row, nil
}

func (u *Usecase) DeleteYurubo(ctx context.Context, input DeleteInput) (map[string]any, error) {
	yuruboID, err := CleanUUID(input.YuruboID, "yurubo id")
	if err != nil {
		return nil, err
	}
	ownerUserID, err := CleanUUID(input.OwnerUserID, "owner user id")
	if err != nil {
		return nil, err
	}
	row, err := u.repository.DeleteYurubo(ctx, input.AuthToken, yuruboID, ownerUserID)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, UserError{Kind: ErrorKindNotFound, Message: "yurubo not found"}
	}
	return row, nil
}

func (u *Usecase) ListYurubos(ctx context.Context, input ListInput) ([]map[string]any, error) {
	userID, err := CleanUUID(input.UserID, "user id")
	if err != nil {
		return nil, err
	}
	hiddenSet, err := u.repository.HiddenYuruboIDs(ctx, input.AuthToken, userID)
	if err != nil {
		hiddenSet = map[string]bool{}
	}
	var rows []map[string]any
	if repo, ok := u.repository.(interface {
		ListOpenYurubosForViewer(context.Context, string, string, int) ([]map[string]any, error)
	}); ok {
		rows, err = repo.ListOpenYurubosForViewer(ctx, input.AuthToken, userID, CleanLimit(input.Limit, 50))
	} else {
		rows, err = u.repository.ListOpenYurubos(ctx, input.AuthToken, CleanLimit(input.Limit, 50))
	}
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(rows))
	// ListOpenYurubos already selects owner_user_id. Keep it locally and pass it
	// into reactionSummaries to avoid one OwnerID query per yurubo (N+1 DB load).
	ownerIDs := make(map[string]string, len(rows))
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		id, _ := row["id"].(string)
		if id == "" || hiddenSet[id] {
			continue
		}
		if ownerID, _ := row["owner_user_id"].(string); ownerID != "" {
			ownerIDs[id] = ownerID
		}
		ids = append(ids, id)
		out = append(out, row)
	}
	reactionCounts, reactedByMe, myReactionTypes, participants := u.reactionSummaries(ctx, input.AuthToken, ids, userID, ownerIDs)
	visibilityLabels, err := u.repository.VisibilityLabels(ctx, input.AuthToken, out)
	if err != nil {
		visibilityLabels = map[string]string{}
	}
	for _, row := range out {
		id, _ := row["id"].(string)
		row["reaction_count"] = reactionCounts[id]
		row["reacted_by_me"] = reactedByMe[id]
		row["my_reaction_type"] = myReactionTypes[id]
		row["participants"] = participants[id]
		if label := visibilityLabels[id]; label != "" {
			row["visibility_label"] = label
		} else {
			row["visibility_label"] = "全フレンズ"
		}
	}
	return out, nil
}

func (u *Usecase) ReactYurubo(ctx context.Context, input ReactionInput) (ReactionState, error) {
	reaction, err := cleanReactionInput(input)
	if err != nil {
		return ReactionState{}, err
	}
	reacted, err := u.repository.UpsertReaction(ctx, input.AuthToken, reaction)
	if err != nil {
		return ReactionState{}, err
	}
	if !reacted {
		return ReactionState{}, UserError{Kind: ErrorKindNotFound, Message: "yurubo not found"}
	}
	return ReactionState{ReactedByMe: true}, nil
}

func (u *Usecase) ApproveReaction(ctx context.Context, input ApprovalInput) (ApprovalState, error) {
	yuruboID, err := CleanUUID(input.YuruboID, "yurubo id")
	if err != nil {
		return ApprovalState{}, err
	}
	participantID, err := CleanUUID(input.ParticipantID, "user id")
	if err != nil {
		return ApprovalState{}, err
	}
	ownerUserID, err := CleanUUID(input.OwnerUserID, "owner user id")
	if err != nil {
		return ApprovalState{}, err
	}
	ownerID, err := u.repository.OwnerID(ctx, input.AuthToken, yuruboID)
	if err != nil || ownerID == "" || ownerID != ownerUserID {
		return ApprovalState{}, UserError{Kind: ErrorKindForbidden, Message: "only yurubo owner can approve participants"}
	}
	approved, err := u.repository.ApproveReaction(ctx, input.AuthToken, ownerUserID, yuruboID, participantID)
	if err != nil {
		return ApprovalState{}, err
	}
	if !approved {
		return ApprovalState{}, UserError{Kind: ErrorKindNotFound, Message: "pending participation request not found"}
	}
	return ApprovalState{Approved: true}, nil
}

func (u *Usecase) UnreactYurubo(ctx context.Context, input ReactionInput) (ReactionState, error) {
	yuruboID, err := CleanUUID(input.YuruboID, "yurubo id")
	if err != nil {
		return ReactionState{}, err
	}
	userID, err := CleanUUID(input.UserID, "user id")
	if err != nil {
		return ReactionState{}, err
	}
	if err := u.repository.DeleteReaction(ctx, input.AuthToken, yuruboID, userID); err != nil {
		return ReactionState{}, err
	}
	return ReactionState{ReactedByMe: false}, nil
}

func cleanReactionInput(input ReactionInput) (Reaction, error) {
	yuruboID, err := CleanUUID(input.YuruboID, "yurubo id")
	if err != nil {
		return Reaction{}, err
	}
	userID, err := CleanUUID(input.UserID, "user id")
	if err != nil {
		return Reaction{}, err
	}
	reactionType, err := CleanReactionType(input.Body.ReactionType)
	if err != nil {
		return Reaction{}, err
	}
	return Reaction{YuruboID: yuruboID, UserID: userID, ReactionType: reactionType}, nil
}

func (u *Usecase) reactionSummaries(ctx context.Context, authToken string, ids []string, userID string, ownerIDs map[string]string) (map[string]int, map[string]bool, map[string]string, map[string][]map[string]any) {
	// Reactions and participant profiles are fetched in batches. ownerIDs comes
	// from the yurubo list rows, so this function does not perform per-yurubo lookups.
	counts := map[string]int{}
	reactedByMe := map[string]bool{}
	myReactionTypes := map[string]string{}
	participants := map[string][]map[string]any{}
	if len(ids) == 0 {
		return counts, reactedByMe, myReactionTypes, participants
	}
	rows, err := u.repository.ListReactions(ctx, authToken, ids)
	if err != nil {
		return counts, reactedByMe, myReactionTypes, participants
	}
	userIDs := []string{}
	seenUsers := map[string]bool{}
	for _, row := range rows {
		id, _ := row["yurubo_id"].(string)
		if id == "" {
			continue
		}
		reactionType, _ := row["reaction_type"].(string)
		if reactionType == contracts.ReactionTypeAvailable {
			counts[id]++
		}
		actor, _ := row["user_id"].(string)
		if actor == userID {
			reactedByMe[id] = true
			myReactionTypes[id] = reactionType
		}
		if actor != "" && !seenUsers[actor] {
			seenUsers[actor] = true
			userIDs = append(userIDs, actor)
		}
	}
	profilesByID, err := u.repository.ParticipantProfiles(ctx, authToken, userIDs)
	if err != nil {
		profilesByID = map[string]map[string]any{}
	}
	for _, row := range rows {
		id, _ := row["yurubo_id"].(string)
		actor, _ := row["user_id"].(string)
		if id == "" || actor == "" {
			continue
		}
		reactionType, _ := row["reaction_type"].(string)
		// Pending/soft reactions are visible only to the yurubo owner and the actor;
		// approved participants remain visible to everyone who can see the yurubo.
		if reactionType != contracts.ReactionTypeAvailable && ownerIDs[id] != userID && actor != userID {
			continue
		}
		profile := profilesByID[actor]
		if profile == nil {
			profile = map[string]any{"id": actor}
		}
		profileCopy := map[string]any{}
		for key, value := range profile {
			profileCopy[key] = value
		}
		profileCopy["reaction_type"] = reactionType
		participants[id] = append(participants[id], profileCopy)
	}
	return counts, reactedByMe, myReactionTypes, participants
}
