package wishitems

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

type ListInput struct {
	AuthToken string
	UserID    string
	Limit     string
}

type ProfileListInput struct {
	AuthToken string
	ProfileID string
	Limit     string
}

type CreateInput struct {
	AuthToken   string
	OwnerUserID string
	Body        CreateBody
}

func (u *Usecase) ListWishItems(ctx context.Context, input ListInput) ([]map[string]any, error) {
	userID, err := CleanUUID(input.UserID, "user id")
	if err != nil {
		return nil, err
	}
	return u.repository.ListWishItems(ctx, input.AuthToken, userID, CleanLimit(input.Limit, 50))
}

func (u *Usecase) ListProfileWishItems(ctx context.Context, input ProfileListInput) ([]map[string]any, error) {
	profileID, err := CleanUUID(input.ProfileID, "profile id")
	if err != nil {
		return nil, err
	}
	return u.repository.ListProfileWishItems(ctx, input.AuthToken, profileID, CleanLimit(input.Limit, 30))
}

func (u *Usecase) CreateWishItem(ctx context.Context, input CreateInput) (map[string]any, error) {
	item, err := NewWishItem(input.OwnerUserID, input.Body)
	if err != nil {
		return nil, err
	}
	return u.repository.CreateWishItem(ctx, input.AuthToken, item)
}
