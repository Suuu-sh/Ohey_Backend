package media

import (
	"context"
	"time"
)

type Dependencies struct {
	Storage      StorageRepository
	Now          func() time.Time
	RandomSuffix func() string
}

type Usecase struct {
	storage      StorageRepository
	now          func() time.Time
	randomSuffix func() string
}

func NewUsecase(deps Dependencies) *Usecase {
	now := deps.Now
	if now == nil {
		now = time.Now
	}
	randomSuffix := deps.RandomSuffix
	if randomSuffix == nil {
		randomSuffix = RandomSuffix
	}
	return &Usecase{storage: deps.Storage, now: now, randomSuffix: randomSuffix}
}

func (u *Usecase) CreateUploadURL(ctx context.Context, input UploadRequest) (UploadURL, error) {
	target, err := NewUploadTarget(input, u.now(), u.randomSuffix)
	if err != nil {
		return UploadURL{}, err
	}
	return u.storage.CreateSignedUploadURL(ctx, target)
}

func (u *Usecase) CreateDisplayURL(ctx context.Context, input DisplayURLRequest) (DisplayURL, error) {
	if _, err := CleanUUID(input.UserID, "user id"); err != nil {
		return DisplayURL{}, err
	}
	path, err := CleanMemoryPhotoPath(input.Path)
	if err != nil {
		return DisplayURL{}, err
	}
	signedURL, err := u.storage.CreateSignedDisplayURL(ctx, PhotoBucket, path, DisplayURLTTLSeconds)
	if err != nil {
		return DisplayURL{}, err
	}
	return DisplayURL{Bucket: PhotoBucket, Path: path, SignedURL: signedURL, ExpiresIn: DisplayURLTTLSeconds}, nil
}
