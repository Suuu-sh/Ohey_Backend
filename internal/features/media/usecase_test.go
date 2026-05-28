package media

import (
	"context"
	"strings"
	"testing"
	"time"
)

const testUserID = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"

type fakeStorage struct {
	target      UploadTarget
	displayPath string
}

func (f *fakeStorage) CreateSignedUploadURL(_ context.Context, target UploadTarget) (UploadURL, error) {
	f.target = target
	return UploadURL{Bucket: target.Bucket, Path: target.Path, Token: "token", SignedURL: "https://example.test/upload?token=token", ContentType: target.ContentType}, nil
}

func (f *fakeStorage) CreateSignedDisplayURL(_ context.Context, bucket, objectPath string, _ int) (string, error) {
	f.displayPath = bucket + "/" + objectPath
	return "https://example.test/display", nil
}

func (f *fakeStorage) ListObjects(context.Context, string, string, int) ([]StorageObject, error) {
	return nil, nil
}

func TestCreateUploadURLBuildsMemoryPhotoPath(t *testing.T) {
	storage := &fakeStorage{}
	usecase := NewUsecase(Dependencies{
		Storage: storage,
		Now: func() time.Time {
			return time.Date(2026, 5, 24, 12, 30, 0, 123456789, time.UTC)
		},
		RandomSuffix: func() string { return "abcdef" },
	})

	result, err := usecase.CreateUploadURL(context.Background(), UploadRequest{
		Kind:          "memory_photo",
		UserID:        testUserID,
		FileExtension: "jpg",
		ContentType:   "image/jpeg",
	})
	if err != nil {
		t.Fatalf("CreateUploadURL returned error: %v", err)
	}
	wantPath := "users/" + testUserID + "/memories/20260524T123000.123456789_abcdef.jpg"
	if result.Bucket != PhotoBucket || result.Path != wantPath || result.ContentType != "image/jpeg" {
		t.Fatalf("result = %#v, want path %s", result, wantPath)
	}
	if storage.target.Path != wantPath {
		t.Fatalf("target path = %q, want %q", storage.target.Path, wantPath)
	}
}

func TestCreateUploadURLRejectsUnsupportedTypes(t *testing.T) {
	usecase := NewUsecase(Dependencies{Storage: &fakeStorage{}})

	_, err := usecase.CreateUploadURL(context.Background(), UploadRequest{
		Kind:          "memory_photo",
		UserID:        testUserID,
		FileExtension: ".gif",
		ContentType:   "image/gif",
	})
	assertUserError(t, err, ErrorKindInvalidInput, "file_extension is unsupported")
}

func TestCreateUploadURLRejectsMismatchedTypes(t *testing.T) {
	usecase := NewUsecase(Dependencies{Storage: &fakeStorage{}})

	_, err := usecase.CreateUploadURL(context.Background(), UploadRequest{
		Kind:          "memory_photo",
		UserID:        testUserID,
		FileExtension: ".png",
		ContentType:   "image/jpeg",
	})
	assertUserError(t, err, ErrorKindInvalidInput, "content_type does not match file_extension")
}

func TestCreateDisplayURLValidatesMemoryPhotoPath(t *testing.T) {
	storage := &fakeStorage{}
	usecase := NewUsecase(Dependencies{Storage: storage})

	result, err := usecase.CreateDisplayURL(context.Background(), DisplayURLRequest{
		UserID: testUserID,
		Path:   "users/" + testUserID + "/memories/photo.jpg",
	})
	if err != nil {
		t.Fatalf("CreateDisplayURL returned error: %v", err)
	}
	if result.SignedURL != "https://example.test/display" || storage.displayPath != "nomo-photos/users/"+testUserID+"/memories/photo.jpg" {
		t.Fatalf("result = %#v displayPath = %q", result, storage.displayPath)
	}
}

func TestCreateDisplayURLRejectsInvalidPath(t *testing.T) {
	usecase := NewUsecase(Dependencies{Storage: &fakeStorage{}})

	_, err := usecase.CreateDisplayURL(context.Background(), DisplayURLRequest{
		UserID: testUserID,
		Path:   "../secret.jpg",
	})
	assertUserError(t, err, ErrorKindInvalidInput, "path is invalid")
}

func TestEscapedStoragePathKeepsFolderSeparators(t *testing.T) {
	got := escapedStoragePath("nomo-photos", "users/abc/memories/photo 1.jpg")
	if !strings.Contains(got, "/") || got != "nomo-photos/users/abc/memories/photo%201.jpg" {
		t.Fatalf("escaped path = %q", got)
	}
}

func assertUserError(t *testing.T, err error, wantKind ErrorKind, wantMessage string) {
	t.Helper()
	if err == nil {
		t.Fatal("err = nil")
	}
	kind, ok := ErrorKindOf(err)
	if !ok {
		t.Fatalf("err = %T %v, want UserError", err, err)
	}
	if kind != wantKind || err.Error() != wantMessage {
		t.Fatalf("err = (%s, %q), want (%s, %q)", kind, err.Error(), wantKind, wantMessage)
	}
}
