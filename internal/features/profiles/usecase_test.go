package profiles

import (
	"context"
	"testing"
	"time"
)

const (
	testAuthToken = "access-token"
	testUserID    = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
)

type fakeRepository struct {
	profile          *Profile
	profileByUserID  *Profile
	bootstrapPayload map[string]any
	patchPayload     map[string]any
	patchRows        []Profile
}

func (f *fakeRepository) GetByID(context.Context, string, string) (*Profile, error) {
	return f.profile, nil
}

func (f *fakeRepository) GetByUserID(context.Context, string, string) (*Profile, error) {
	return f.profileByUserID, nil
}

func (f *fakeRepository) UpsertBootstrap(_ context.Context, _ string, payload map[string]any) (map[string]any, error) {
	f.bootstrapPayload = payload
	return payload, nil
}

func (f *fakeRepository) PatchByID(_ context.Context, _ string, _ string, payload map[string]any) ([]Profile, error) {
	f.patchPayload = payload
	return f.patchRows, nil
}

func TestBootstrapProfileNormalizesAndDefaults(t *testing.T) {
	repo := &fakeRepository{}
	usecase := NewUsecase(Dependencies{Repository: repo, Now: fixedNow})

	row, err := usecase.BootstrapProfile(context.Background(), BootstrapUsecaseInput{
		AuthToken:  testAuthToken,
		AuthUserID: testUserID,
		Request: BootstrapRequest{
			UserID:      " ohey_user ",
			DisplayName: "  Ohey User  ",
			AvatarURL:   " https://example.test/avatar.png ",
		},
	})
	if err != nil {
		t.Fatalf("BootstrapProfile returned error: %v", err)
	}
	if row["id"] != testUserID || row["user_id"] != "ohey_user" || row["display_name"] != "Ohey User" {
		t.Fatalf("row = %#v", row)
	}
	if row["character_key"] != "avatar" || row["avatar_url"] != "https://example.test/avatar.png" {
		t.Fatalf("normalized row = %#v", row)
	}
	if row["updated_at"] != "2026-05-24T12:30:00Z" {
		t.Fatalf("updated_at = %#v", row["updated_at"])
	}
}

func TestBootstrapProfileRejectsInvalidUserID(t *testing.T) {
	usecase := NewUsecase(Dependencies{Repository: &fakeRepository{}})

	_, err := usecase.BootstrapProfile(context.Background(), BootstrapUsecaseInput{
		AuthToken:  testAuthToken,
		AuthUserID: testUserID,
		Request:    BootstrapRequest{UserID: "bad user id", DisplayName: "Name"},
	})
	assertUserError(t, err, ErrorKindInvalidInput, "user_id must be 3-24 letters, numbers, or underscores")
}

func TestUpdateProfileBuildsSanitizedPatchPayload(t *testing.T) {
	repo := &fakeRepository{patchRows: []Profile{{ID: testUserID, UserID: "valid_user"}}}
	usecase := NewUsecase(Dependencies{Repository: repo, Now: fixedNow})

	rows, err := usecase.UpdateProfile(context.Background(), UpdateInput{
		AuthToken:  testAuthToken,
		AuthUserID: testUserID,
		Body: map[string]any{
			"user_id":       " valid_user ",
			"display_name":  "  Name  ",
			"character_key": " cat ",
			"avatar_url":    nil,
			"ignored":       "value",
		},
	})
	if err != nil {
		t.Fatalf("UpdateProfile returned error: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %#v", rows)
	}
	payload := repo.patchPayload
	if payload["user_id"] != "valid_user" || payload["display_name"] != "Name" || payload["character_key"] != "cat" {
		t.Fatalf("payload = %#v", payload)
	}
	if _, ok := payload["ignored"]; ok {
		t.Fatalf("ignored field leaked into payload: %#v", payload)
	}
	if _, ok := payload["avatar_url"]; !ok || payload["avatar_url"] != nil {
		t.Fatalf("avatar_url nil was not preserved: %#v", payload)
	}
}

func TestGetProfileNotFound(t *testing.T) {
	usecase := NewUsecase(Dependencies{Repository: &fakeRepository{}})

	_, err := usecase.GetProfile(context.Background(), AuthInput{AuthToken: testAuthToken, AuthUserID: testUserID})
	assertUserError(t, err, ErrorKindNotFound, "profile not found")
}

func fixedNow() time.Time {
	return time.Date(2026, 5, 24, 12, 30, 0, 0, time.UTC)
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
