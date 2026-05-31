package dailystatuses

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
	getDate    string
	monthStart string
	monthEnd   string
	upserted   DailyStatus
}

func (f *fakeRepository) GetDailyStatus(context.Context, string, string, string) ([]map[string]any, error) {
	f.getDate = "called"
	return []map[string]any{{"status": "available"}}, nil
}

func (f *fakeRepository) ListMonthlyStatuses(_ context.Context, _ string, _ string, startDate string, endDate string) ([]map[string]any, error) {
	f.monthStart = startDate
	f.monthEnd = endDate
	return []map[string]any{{"status_date": startDate}}, nil
}

func (f *fakeRepository) FriendshipExists(context.Context, string, string, string) (bool, error) {
	return true, nil
}

func (f *fakeRepository) UpsertDailyStatus(_ context.Context, _ string, _ DailyStatus) ([]map[string]any, error) {
	return nil, nil
}

func TestListMonthlyStatusesUsesCalendarMonthRange(t *testing.T) {
	repo := &fakeRepository{}
	usecase := NewUsecase(Dependencies{Repository: repo})

	rows, err := usecase.ListMonthlyStatuses(context.Background(), MonthInput{AuthToken: testAuthToken, UserID: testUserID, Month: "2026-05"})
	if err != nil {
		t.Fatalf("ListMonthlyStatuses returned error: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %#v", rows)
	}
	if repo.monthStart != "2026-05-01" || repo.monthEnd != "2026-06-01" {
		t.Fatalf("month range = %s..%s", repo.monthStart, repo.monthEnd)
	}
}

func TestUpsertDailyStatusValidatesStatusAndDefaultsDate(t *testing.T) {
	repo := &upsertFakeRepository{}
	now := time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC)
	usecase := NewUsecase(Dependencies{Repository: repo, Now: func() time.Time { return now }})

	_, err := usecase.UpsertDailyStatus(context.Background(), UpsertInput{AuthToken: testAuthToken, UserID: testUserID, Status: " depends_on_time "})
	if err != nil {
		t.Fatalf("UpsertDailyStatus returned error: %v", err)
	}
	if repo.upserted.UserID != testUserID || repo.upserted.StatusDate != "2026-05-28" || repo.upserted.Status != StatusDependsOnTime {
		t.Fatalf("upserted = %#v", repo.upserted)
	}
}

func TestUpsertDailyStatusRejectsInvalidStatus(t *testing.T) {
	usecase := NewUsecase(Dependencies{Repository: &upsertFakeRepository{}})

	_, err := usecase.UpsertDailyStatus(context.Background(), UpsertInput{AuthToken: testAuthToken, UserID: testUserID, Status: "busy"})
	assertUserError(t, err, ErrorKindInvalidInput, "status is invalid")
}

type upsertFakeRepository struct {
	upserted DailyStatus
}

func (f *upsertFakeRepository) GetDailyStatus(context.Context, string, string, string) ([]map[string]any, error) {
	return nil, nil
}

func (f *upsertFakeRepository) ListMonthlyStatuses(context.Context, string, string, string, string) ([]map[string]any, error) {
	return nil, nil
}

func (f *upsertFakeRepository) FriendshipExists(context.Context, string, string, string) (bool, error) {
	return true, nil
}

func (f *upsertFakeRepository) UpsertDailyStatus(_ context.Context, _ string, status DailyStatus) ([]map[string]any, error) {
	f.upserted = status
	return []map[string]any{{"status": string(status.Status)}}, nil
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
