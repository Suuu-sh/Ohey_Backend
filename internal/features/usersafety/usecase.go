package usersafety

import "context"

type Dependencies struct {
	Repository Repository
}

type Usecase struct {
	repository Repository
}

func NewUsecase(deps Dependencies) *Usecase {
	return &Usecase{repository: deps.Repository}
}

type UserTargetInput struct {
	AuthToken    string
	ActorUserID  string
	TargetUserID string
	Reason       string
}

type ListInput struct {
	AuthToken string
	UserID    string
}

func (u *Usecase) ListBlockedUsers(ctx context.Context, input ListInput) ([]map[string]any, error) {
	userID, err := CleanUUID(input.UserID, "user id")
	if err != nil {
		return nil, err
	}
	return u.repository.ListBlockedUsers(ctx, input.AuthToken, userID)
}

func (u *Usecase) BlockUser(ctx context.Context, input UserTargetInput) (map[string]any, error) {
	relation, err := cleanUserRelation(input)
	if err != nil {
		return nil, err
	}
	row, err := u.repository.BlockUser(ctx, input.AuthToken, relation)
	if err != nil {
		return nil, err
	}
	if err := u.repository.CleanupBlockedRelations(ctx, relation); err != nil {
		return nil, err
	}
	return row, nil
}

func (u *Usecase) UnblockUser(ctx context.Context, input UserTargetInput) error {
	relation, err := cleanUserRelation(input)
	if err != nil {
		return err
	}
	return u.repository.UnblockUser(ctx, input.AuthToken, relation)
}

func (u *Usecase) ListMutedUsers(ctx context.Context, input ListInput) ([]map[string]any, error) {
	userID, err := CleanUUID(input.UserID, "user id")
	if err != nil {
		return nil, err
	}
	return u.repository.ListMutedUsers(ctx, input.AuthToken, userID)
}

func (u *Usecase) MuteUser(ctx context.Context, input UserTargetInput) (map[string]any, error) {
	relation, err := cleanUserRelation(input)
	if err != nil {
		return nil, err
	}
	return u.repository.MuteUser(ctx, input.AuthToken, relation)
}

func (u *Usecase) UnmuteUser(ctx context.Context, input UserTargetInput) error {
	relation, err := cleanUserRelation(input)
	if err != nil {
		return err
	}
	return u.repository.UnmuteUser(ctx, input.AuthToken, relation)
}

func (u *Usecase) ReportUser(ctx context.Context, input UserTargetInput) (map[string]any, error) {
	relation, err := cleanUserRelation(input)
	if err != nil {
		return nil, err
	}
	reason, err := CleanReportReason(input.Reason)
	if err != nil {
		return nil, err
	}
	return u.repository.ReportUser(ctx, input.AuthToken, UserReport{
		ReporterUserID: relation.ActorUserID,
		ReportedUserID: relation.TargetUserID,
		Reason:         reason,
	})
}

func cleanUserRelation(input UserTargetInput) (UserRelation, error) {
	actorUserID, err := CleanUUID(input.ActorUserID, "user id")
	if err != nil {
		return UserRelation{}, err
	}
	targetUserID, err := CleanUUID(input.TargetUserID, "target user id")
	if err != nil {
		return UserRelation{}, err
	}
	if err := ValidateDifferentUsers(actorUserID, targetUserID); err != nil {
		return UserRelation{}, err
	}
	return UserRelation{ActorUserID: actorUserID, TargetUserID: targetUserID}, nil
}
