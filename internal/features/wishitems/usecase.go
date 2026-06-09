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
	AuthToken    string
	ViewerUserID string
	ProfileID    string
	Limit        string
}

type CreateInput struct {
	AuthToken   string
	OwnerUserID string
	Body        CreateBody
}

type UpdateInput struct {
	AuthToken   string
	WishItemID  string
	OwnerUserID string
	Body        UpdateBody
}

type DeleteInput struct {
	AuthToken   string
	WishItemID  string
	OwnerUserID string
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
	if repo, ok := u.repository.(interface {
		ListProfileWishItemsForViewer(context.Context, string, string, string, int) ([]map[string]any, error)
	}); ok {
		viewerUserID, err := CleanUUID(input.ViewerUserID, "viewer user id")
		if err != nil {
			return nil, err
		}
		return repo.ListProfileWishItemsForViewer(ctx, input.AuthToken, viewerUserID, profileID, CleanLimit(input.Limit, 30))
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

func (u *Usecase) UpdateWishItem(ctx context.Context, input UpdateInput) (map[string]any, error) {
	update, err := NewWishItemUpdate(input.WishItemID, input.OwnerUserID, input.Body)
	if err != nil {
		return nil, err
	}
	row, err := u.repository.UpdateWishItem(ctx, input.AuthToken, update)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, UserError{Kind: ErrorKindNotFound, Message: "wish item not found"}
	}
	return row, nil
}

func (u *Usecase) DeleteWishItem(ctx context.Context, input DeleteInput) (map[string]any, error) {
	wishItemID, err := CleanUUID(input.WishItemID, "wish item id")
	if err != nil {
		return nil, err
	}
	ownerUserID, err := CleanUUID(input.OwnerUserID, "owner user id")
	if err != nil {
		return nil, err
	}
	row, err := u.repository.DeleteWishItem(ctx, input.AuthToken, wishItemID, ownerUserID)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, UserError{Kind: ErrorKindNotFound, Message: "wish item not found"}
	}
	return row, nil
}
