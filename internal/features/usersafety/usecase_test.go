package usersafety

import (
	"context"
	"testing"
)

const (
	testAuthToken = "access-token"
	testUserID    = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	otherUserID   = "bbbbbbbb-cccc-dddd-eeee-ffffffffffff"
	testLogID     = "11111111-2222-3333-4444-555555555555"
)

type fakeRepository struct {
	blocked UserRelation
	hidden  HiddenDrinkLog
	cleaned UserRelation
}

func (f *fakeRepository) ListBlockedUsers(context.Context, string, string) ([]map[string]any, error) {
	return []map[string]any{{"id": otherUserID}}, nil
}
func (f *fakeRepository) BlockUser(_ context.Context, _ string, relation UserRelation) (map[string]any, error) {
	f.blocked = relation
	return map[string]any{"blocked_user_id": relation.TargetUserID}, nil
}
func (f *fakeRepository) UnblockUser(context.Context, string, UserRelation) error { return nil }
func (f *fakeRepository) ListMutedUsers(context.Context, string, string) ([]map[string]any, error) {
	return []map[string]any{{"id": otherUserID}}, nil
}
func (f *fakeRepository) MuteUser(context.Context, string, UserRelation) (map[string]any, error) {
	return nil, nil
}
func (f *fakeRepository) UnmuteUser(context.Context, string, UserRelation) error { return nil }
func (f *fakeRepository) HideDrinkLog(_ context.Context, _ string, hidden HiddenDrinkLog) (map[string]any, error) {
	f.hidden = hidden
	return map[string]any{"drink_log_id": hidden.DrinkLogID}, nil
}
func (f *fakeRepository) UnhideDrinkLog(context.Context, string, HiddenDrinkLog) error { return nil }
func (f *fakeRepository) CleanupBlockedRelations(_ context.Context, relation UserRelation) error {
	f.cleaned = relation
	return nil
}

func TestBlockUserRejectsSelf(t *testing.T) {
	usecase := NewUsecase(Dependencies{Repository: &fakeRepository{}})

	_, err := usecase.BlockUser(context.Background(), UserTargetInput{AuthToken: testAuthToken, ActorUserID: testUserID, TargetUserID: testUserID})
	assertUserError(t, err, ErrorKindForbidden, "target user must be different from yourself")
}

func TestBlockUserCleansRelation(t *testing.T) {
	repo := &fakeRepository{}
	usecase := NewUsecase(Dependencies{Repository: repo})

	_, err := usecase.BlockUser(context.Background(), UserTargetInput{AuthToken: testAuthToken, ActorUserID: testUserID, TargetUserID: otherUserID})
	if err != nil {
		t.Fatalf("BlockUser returned error: %v", err)
	}
	if repo.blocked.ActorUserID != testUserID || repo.blocked.TargetUserID != otherUserID {
		t.Fatalf("blocked = %#v", repo.blocked)
	}
	if repo.cleaned.ActorUserID != testUserID || repo.cleaned.TargetUserID != otherUserID {
		t.Fatalf("cleaned = %#v", repo.cleaned)
	}
}

func TestListBlockedUsersCleansUserID(t *testing.T) {
	usecase := NewUsecase(Dependencies{Repository: &fakeRepository{}})

	rows, err := usecase.ListBlockedUsers(context.Background(), ListInput{AuthToken: testAuthToken, UserID: testUserID})
	if err != nil {
		t.Fatalf("ListBlockedUsers returned error: %v", err)
	}
	if len(rows) != 1 || rows[0]["id"] != otherUserID {
		t.Fatalf("rows = %#v", rows)
	}
}

func TestHideDrinkLogCleansIDs(t *testing.T) {
	repo := &fakeRepository{}
	usecase := NewUsecase(Dependencies{Repository: repo})

	_, err := usecase.HideDrinkLog(context.Background(), DrinkLogInput{AuthToken: testAuthToken, UserID: testUserID, DrinkLogID: testLogID})
	if err != nil {
		t.Fatalf("HideDrinkLog returned error: %v", err)
	}
	if repo.hidden.UserID != testUserID || repo.hidden.DrinkLogID != testLogID {
		t.Fatalf("hidden = %#v", repo.hidden)
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
