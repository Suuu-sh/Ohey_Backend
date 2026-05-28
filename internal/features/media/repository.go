package media

import "context"

type StorageRepository interface {
	CreateSignedUploadURL(ctx context.Context, target UploadTarget) (UploadURL, error)
	CreateSignedDisplayURL(ctx context.Context, bucket, objectPath string, expiresInSeconds int) (string, error)
	ListObjects(ctx context.Context, bucket, prefix string, limit int) ([]StorageObject, error)
}
