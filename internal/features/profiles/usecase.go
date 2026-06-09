package profiles

import (
	"context"
	"time"
)

type Dependencies struct {
	Repository Repository
	Now        func() time.Time
}

type Usecase struct {
	repository Repository
	now        func() time.Time
}

func NewUsecase(deps Dependencies) *Usecase {
	now := deps.Now
	if now == nil {
		now = time.Now
	}
	return &Usecase{repository: deps.Repository, now: now}
}

type AuthInput struct {
	AuthToken   string
	AuthUserID  string
	ClerkUserID string
}

type GetByUserIDInput struct {
	AuthToken string
	UserID    string
}

type BootstrapRequest struct {
	UserID       string
	DisplayName  string
	CharacterKey string
	AvatarURL    string
}

type BootstrapUsecaseInput struct {
	AuthToken   string
	AuthUserID  string
	ClerkUserID string
	Request     BootstrapRequest
}

type UpdateInput struct {
	AuthToken  string
	AuthUserID string
	Body       map[string]any
}

func (u *Usecase) GetProfile(ctx context.Context, input AuthInput) (*Profile, error) {
	var profile *Profile
	var err error
	if input.ClerkUserID != "" {
		profile, err = u.repository.GetByClerkUserID(ctx, input.AuthToken, input.ClerkUserID)
	} else {
		authUserID, cleanErr := CleanUUID(input.AuthUserID, "user id")
		if cleanErr != nil {
			return nil, cleanErr
		}
		profile, err = u.repository.GetByID(ctx, input.AuthToken, authUserID)
	}
	if err != nil {
		return nil, err
	}
	if profile == nil {
		return nil, UserError{Kind: ErrorKindNotFound, Message: "profile not found"}
	}
	return profile, nil
}

func (u *Usecase) GetProfileByUserID(ctx context.Context, input GetByUserIDInput) (*Profile, error) {
	userID, err := CleanUserID(input.UserID)
	if err != nil {
		return nil, err
	}
	profile, err := u.repository.GetByUserID(ctx, input.AuthToken, userID)
	if err != nil {
		return nil, err
	}
	if profile == nil {
		return nil, UserError{Kind: ErrorKindNotFound, Message: "profile not found"}
	}
	return profile, nil
}

func (u *Usecase) BootstrapProfile(ctx context.Context, input BootstrapUsecaseInput) (map[string]any, error) {
	payload, err := BootstrapPayload(BootstrapInput{
		AuthUserID:   input.AuthUserID,
		ClerkUserID:  input.ClerkUserID,
		UserID:       input.Request.UserID,
		DisplayName:  input.Request.DisplayName,
		CharacterKey: input.Request.CharacterKey,
		AvatarURL:    input.Request.AvatarURL,
		UpdatedAt:    u.now(),
	})
	if err != nil {
		return nil, err
	}
	return u.repository.UpsertBootstrap(ctx, input.AuthToken, payload)
}

func (u *Usecase) UpdateProfile(ctx context.Context, input UpdateInput) ([]Profile, error) {
	authUserID, err := CleanUUID(input.AuthUserID, "user id")
	if err != nil {
		return nil, err
	}
	payload, err := PatchPayload(input.Body, u.now())
	if err != nil {
		return nil, err
	}
	return u.repository.PatchByID(ctx, input.AuthToken, authUserID, payload)
}
