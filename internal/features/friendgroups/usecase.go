package friendgroups

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

type AuthInput struct {
	AuthToken string
	UserID    string
}

type SaveInput struct {
	AuthToken string
	UserID    string
	Body      SaveInputBody
}

func (u *Usecase) ListFriendGroups(ctx context.Context, input AuthInput) ([]FriendGroup, error) {
	userID, err := CleanUUID(input.UserID, "user id")
	if err != nil {
		return nil, err
	}
	return u.repository.ListGroups(ctx, input.AuthToken, userID)
}

func (u *Usecase) SaveFriendGroups(ctx context.Context, input SaveInput) ([]FriendGroup, error) {
	userID, err := CleanUUID(input.UserID, "user id")
	if err != nil {
		return nil, err
	}
	groups, err := NormalizeGroups(input.Body.Groups)
	if err != nil {
		return nil, err
	}
	for _, group := range groups {
		for _, friendID := range group.FriendIDs {
			ok, err := u.repository.FriendshipExists(ctx, input.AuthToken, userID, friendID)
			if err != nil {
				return nil, err
			}
			if !ok {
				return nil, UserError{Kind: ErrorKindForbidden, Message: "friend_ids must be existing friends"}
			}
		}
	}
	return u.repository.SaveGroups(ctx, input.AuthToken, userID, groups)
}
