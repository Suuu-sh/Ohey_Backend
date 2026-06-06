package memories

import (
	"context"
	"reflect"
	"testing"
	"time"
)

const (
	testAuthToken = "access-token"
	testUserID    = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	otherUserID   = "bbbbbbbb-cccc-dddd-eeee-ffffffffffff"
	testMemoryID  = "11111111-2222-3333-4444-555555555555"
)

type fakeRepository struct {
	calls []string

	visibleUserIDs   []string
	memories         []map[string]any
	officialMemories []map[string]any
	hasDailyMemory   bool
	friendships      map[string]bool
	createdMemory    map[string]any
	deletedMemory    map[string]any
	hiddenIDs        map[string]bool
	hiddenUserIDs    map[string]bool
	report           *Report
	reportOwnerID    string
	createdReport    Report

	newMemory NewMemory
	links     []string
}

func (f *fakeRepository) VisibleFeedUserIDs(context.Context, string, string) ([]string, error) {
	f.calls = append(f.calls, "visible")
	if f.visibleUserIDs != nil {
		return f.visibleUserIDs, nil
	}
	return []string{testUserID}, nil
}

func (f *fakeRepository) ListMemories(context.Context, string, []string) ([]map[string]any, error) {
	f.calls = append(f.calls, "list_memories")
	return f.memories, nil
}

func (f *fakeRepository) ListOfficialMemories(context.Context, string) ([]map[string]any, error) {
	f.calls = append(f.calls, "list_official")
	return f.officialMemories, nil
}

func (f *fakeRepository) HasMemoryInWindow(context.Context, string, string, time.Time, time.Time) (bool, error) {
	f.calls = append(f.calls, "daily_limit")
	return f.hasDailyMemory, nil
}

func (f *fakeRepository) FriendshipExists(_ context.Context, _ string, _ string, friendID string) (bool, error) {
	f.calls = append(f.calls, "friendship")
	if f.friendships == nil {
		return true, nil
	}
	return f.friendships[friendID], nil
}

func (f *fakeRepository) CreateMemory(_ context.Context, _ string, memory NewMemory) (map[string]any, error) {
	f.calls = append(f.calls, "create")
	f.newMemory = memory
	if f.createdMemory != nil {
		return f.createdMemory, nil
	}
	return map[string]any{"id": testMemoryID, "owner_user_id": memory.OwnerUserID}, nil
}

func (f *fakeRepository) CreateMemoryFriendLinks(_ context.Context, _ string, _ string, friendIDs []string) error {
	f.calls = append(f.calls, "links")
	f.links = friendIDs
	return nil
}

func (f *fakeRepository) DeleteOwnedMemory(context.Context, string, string, string) (map[string]any, error) {
	f.calls = append(f.calls, "delete")
	return f.deletedMemory, nil
}

func (f *fakeRepository) HiddenMemoryIDs(context.Context, string, string) (map[string]bool, error) {
	f.calls = append(f.calls, "hidden_reports")
	if f.hiddenIDs != nil {
		return f.hiddenIDs, nil
	}
	return map[string]bool{}, nil
}

func (f *fakeRepository) HiddenUserIDs(context.Context, string, string) (map[string]bool, error) {
	f.calls = append(f.calls, "hidden_users")
	if f.hiddenUserIDs != nil {
		return f.hiddenUserIDs, nil
	}
	return map[string]bool{}, nil
}

func (f *fakeRepository) MemoryOwnerUserID(context.Context, string, string) (string, error) {
	f.calls = append(f.calls, "log_owner")
	return f.reportOwnerID, nil
}

func (f *fakeRepository) FindReport(context.Context, string, string, string) (*Report, error) {
	f.calls = append(f.calls, "find_report")
	return f.report, nil
}

func (f *fakeRepository) CreateReport(_ context.Context, _ string, report Report) error {
	f.calls = append(f.calls, "create_report")
	f.createdReport = report
	return nil
}

type fakePublisher struct {
	events []DomainEvent
}

func (f *fakePublisher) Publish(_ context.Context, _ string, event DomainEvent) {
	f.events = append(f.events, event)
}

func TestCreateMemoryRejectsNonFriendTagBeforeInsert(t *testing.T) {
	repo := &fakeRepository{friendships: map[string]bool{otherUserID: false}}
	usecase := NewUsecase(Dependencies{Repository: repo})

	_, err := usecase.CreateMemory(context.Background(), CreateInput{
		AuthToken:   testAuthToken,
		OwnerUserID: testUserID,
		FriendIDs:   []string{otherUserID},
	})

	assertUserError(t, err, ErrorKindForbidden, "friend_ids must be existing friends")
	if want := []string{"friendship"}; !reflect.DeepEqual(repo.calls, want) {
		t.Fatalf("calls = %v, want %v", repo.calls, want)
	}
}

func TestCreateMemoryRejectsExistingLogOnSameDay(t *testing.T) {
	repo := &fakeRepository{hasDailyMemory: true}
	usecase := NewUsecase(Dependencies{Repository: repo})
	happenedAt := time.Date(2026, 5, 24, 12, 30, 0, 0, time.UTC)
	offset := 9 * 60

	_, err := usecase.CreateMemory(context.Background(), CreateInput{
		AuthToken:             testAuthToken,
		OwnerUserID:           testUserID,
		HappenedAt:            &happenedAt,
		HappenedOn:            "2026-05-24",
		TimezoneOffsetMinutes: &offset,
	})

	assertUserError(t, err, ErrorKindConflict, "投稿は1日1つまでです")
	if want := []string{"daily_limit"}; !reflect.DeepEqual(repo.calls, want) {
		t.Fatalf("calls = %v, want %v", repo.calls, want)
	}
}

func TestCreateMemoryCreatesMemoRecordAndDeduplicatesFriends(t *testing.T) {
	repo := &fakeRepository{}
	publisher := &fakePublisher{}
	usecase := NewUsecase(Dependencies{Repository: repo, Publisher: publisher})

	row, err := usecase.CreateMemory(context.Background(), CreateInput{
		AuthToken:   testAuthToken,
		OwnerUserID: testUserID,
		PlaceName:   "  Shibuya  ",
		Memo:        "  hello  ",
		FriendIDs:   []string{otherUserID, otherUserID},
	})
	if err != nil {
		t.Fatalf("CreateMemory returned error: %v", err)
	}
	if row["id"] != testMemoryID {
		t.Fatalf("row = %#v", row)
	}
	if repo.newMemory.PlaceName != "Shibuya" || repo.newMemory.Memo != "hello" {
		t.Fatalf("new memory = %#v", repo.newMemory)
	}
	if !reflect.DeepEqual(repo.links, []string{otherUserID}) {
		t.Fatalf("links = %v, want deduplicated friend id", repo.links)
	}
	if len(publisher.events) != 1 || publisher.events[0].Kind != EventMemoryTagged {
		t.Fatalf("events = %#v, want memory tagged", publisher.events)
	}
}

func TestDeleteMemoryReturnsDeletedRow(t *testing.T) {
	repo := &fakeRepository{deletedMemory: map[string]any{"id": testMemoryID}}
	usecase := NewUsecase(Dependencies{Repository: repo})

	row, err := usecase.DeleteMemory(context.Background(), DeleteInput{AuthToken: testAuthToken, MemoryID: testMemoryID, OwnerUserID: testUserID})
	if err != nil {
		t.Fatalf("DeleteMemory returned error: %v", err)
	}
	if row["id"] != testMemoryID {
		t.Fatalf("row = %#v", row)
	}
}

func TestReportMemoryCreatesModerationReportAndHidesPost(t *testing.T) {
	repo := &fakeRepository{reportOwnerID: otherUserID}
	publisher := &fakePublisher{}
	usecase := NewUsecase(Dependencies{Repository: repo, Publisher: publisher})

	result, err := usecase.ReportMemory(context.Background(), ReportInput{
		AuthToken:      testAuthToken,
		MemoryID:       testMemoryID,
		ReporterUserID: testUserID,
		Reason:         " harassment ",
	})
	if err != nil {
		t.Fatalf("ReportMemory returned error: %v", err)
	}
	if !result.Created || result.Body["duplicate"] != false || result.Body["hidden"] != true || result.Body["reason"] != "harassment" {
		t.Fatalf("result = %#v", result)
	}
	if repo.createdReport.MemoryID != testMemoryID || repo.createdReport.ReporterUserID != testUserID || repo.createdReport.Reason != ReportReasonHarassment {
		t.Fatalf("created report = %#v", repo.createdReport)
	}
	if want := []string{"log_owner", "find_report", "create_report"}; !reflect.DeepEqual(repo.calls, want) {
		t.Fatalf("calls = %v, want %v", repo.calls, want)
	}
	if len(publisher.events) != 1 || publisher.events[0].Kind != EventMemoryReported {
		t.Fatalf("events = %#v, want memory reported", publisher.events)
	}
}

func TestReportMemoryRejectsOwnMemory(t *testing.T) {
	repo := &fakeRepository{reportOwnerID: testUserID}
	usecase := NewUsecase(Dependencies{Repository: repo})

	_, err := usecase.ReportMemory(context.Background(), ReportInput{AuthToken: testAuthToken, MemoryID: testMemoryID, ReporterUserID: testUserID})
	assertUserError(t, err, ErrorKindForbidden, "cannot report your own memory")
	if want := []string{"log_owner"}; !reflect.DeepEqual(repo.calls, want) {
		t.Fatalf("calls = %v, want %v", repo.calls, want)
	}
}

func TestReportMemoryReturnsDuplicateForExistingReport(t *testing.T) {
	repo := &fakeRepository{reportOwnerID: otherUserID, report: &Report{ID: "report-id", MemoryID: testMemoryID, ReporterUserID: testUserID, Reason: ReportReasonSpam}}
	usecase := NewUsecase(Dependencies{Repository: repo})

	result, err := usecase.ReportMemory(context.Background(), ReportInput{AuthToken: testAuthToken, MemoryID: testMemoryID, ReporterUserID: testUserID, Reason: "spam"})
	if err != nil {
		t.Fatalf("ReportMemory returned error: %v", err)
	}
	if result.Created || result.Body["duplicate"] != true || result.Body["status"] != "pending" {
		t.Fatalf("result = %#v", result)
	}
	if want := []string{"log_owner", "find_report"}; !reflect.DeepEqual(repo.calls, want) {
		t.Fatalf("calls = %v, want %v", repo.calls, want)
	}
}

func TestListMemoriesExcludesHiddenReportedRows(t *testing.T) {
	repo := &fakeRepository{
		memories: []map[string]any{
			{"id": "visible", "happened_at": "2026-05-24T12:00:00Z"},
			{"id": "hidden", "happened_at": "2026-05-24T13:00:00Z"},
		},
		hiddenIDs: map[string]bool{"hidden": true},
	}
	usecase := NewUsecase(Dependencies{Repository: repo})

	rows, err := usecase.ListMemories(context.Background(), ListInput{AuthToken: testAuthToken, UserID: testUserID})
	if err != nil {
		t.Fatalf("ListMemories returned error: %v", err)
	}
	if len(rows) != 1 || rows[0]["id"] != "visible" {
		t.Fatalf("rows = %#v", rows)
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
