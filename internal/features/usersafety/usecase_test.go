package usersafety

import (
	"context"
	"testing"
)

const (
	testAuthToken = "access-token"
	testUserID    = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	otherUserID   = "bbbbbbbb-cccc-dddd-eeee-ffffffffffff"
	testMemoryID  = "11111111-2222-3333-4444-555555555555"
)

type fakeRepository struct {
	blocked UserRelation
	report  UserReport
	hidden  HiddenMemory
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
func (f *fakeRepository) ReportUser(_ context.Context, _ string, report UserReport) (map[string]any, error) {
	f.report = report
	return map[string]any{"reported_user_id": report.ReportedUserID, "reason": report.Reason}, nil
}
func (f *fakeRepository) HideMemory(_ context.Context, _ string, hidden HiddenMemory) (map[string]any, error) {
	f.hidden = hidden
	return map[string]any{"memory_id": hidden.MemoryID}, nil
}
func (f *fakeRepository) UnhideMemory(context.Context, string, HiddenMemory) error { return nil }
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

func TestReportUserCleansReason(t *testing.T) {
	repo := &fakeRepository{}
	usecase := NewUsecase(Dependencies{Repository: repo})

	row, err := usecase.ReportUser(context.Background(), UserTargetInput{
		AuthToken:    testAuthToken,
		ActorUserID:  testUserID,
		TargetUserID: otherUserID,
		Reason:       "  SPAM ",
	})
	if err != nil {
		t.Fatalf("ReportUser returned error: %v", err)
	}
	if repo.report.ReporterUserID != testUserID || repo.report.ReportedUserID != otherUserID || repo.report.Reason != "spam" {
		t.Fatalf("report = %#v", repo.report)
	}
	if row["reported_user_id"] != otherUserID || row["reason"] != "spam" {
		t.Fatalf("row = %#v", row)
	}
}

func TestReportUserRejectsInvalidReason(t *testing.T) {
	usecase := NewUsecase(Dependencies{Repository: &fakeRepository{}})

	_, err := usecase.ReportUser(context.Background(), UserTargetInput{
		AuthToken:    testAuthToken,
		ActorUserID:  testUserID,
		TargetUserID: otherUserID,
		Reason:       "bad_reason",
	})
	assertUserError(t, err, ErrorKindInvalidInput, "report reason is invalid")
}

func TestHideMemoryCleansIDs(t *testing.T) {
	repo := &fakeRepository{}
	usecase := NewUsecase(Dependencies{Repository: repo})

	_, err := usecase.HideMemory(context.Background(), MemoryInput{AuthToken: testAuthToken, UserID: testUserID, MemoryID: testMemoryID})
	if err != nil {
		t.Fatalf("HideMemory returned error: %v", err)
	}
	if repo.hidden.UserID != testUserID || repo.hidden.MemoryID != testMemoryID {
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
