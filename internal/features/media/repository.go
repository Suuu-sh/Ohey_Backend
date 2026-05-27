package media

import "context"

type StorageRepository interface {
	CreateSignedUploadURL(ctx context.Context, target UploadTarget) (UploadURL, error)
}
