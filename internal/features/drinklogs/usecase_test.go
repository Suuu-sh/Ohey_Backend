package drinklogs

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
	testLogID     = "11111111-2222-3333-4444-555555555555"
)

type fakeRepository struct {
	calls []string

	visibleUserIDs []string
	logs           []map[string]any
	officialLogs   []map[string]any
	hasDailyLog    bool
	friendships    map[string]bool
	createdLog     map[string]any
	deletedLog     map[string]any
	likeCreated    bool
	likeState      LikeState
	hiddenIDs      map[string]bool
	report         *Report
	reportOwnerID  string
	createdReport  Report

	newLog NewDrinkLog
	links  []string
}

func (f *fakeRepository) VisibleFeedUserIDs(context.Context, string, string) ([]string, error) {
	f.calls = append(f.calls, "visible")
	if f.visibleUserIDs != nil {
		return f.visibleUserIDs, nil
	}
	return []string{testUserID}, nil
}

func (f *fakeRepository) ListDrinkLogs(context.Context, string, []string) ([]map[string]any, error) {
	f.calls = append(f.calls, "list_logs")
	return f.logs, nil
}

func (f *fakeRepository) ListOfficialDrinkLogs(context.Context, string) ([]map[string]any, error) {
	f.calls = append(f.calls, "list_official")
	return f.officialLogs, nil
}

func (f *fakeRepository) HasDrinkLogInWindow(context.Context, string, string, time.Time, time.Time) (bool, error) {
	f.calls = append(f.calls, "daily_limit")
	return f.hasDailyLog, nil
}

func (f *fakeRepository) FriendshipExists(_ context.Context, _ string, _ string, friendID string) (bool, error) {
	f.calls = append(f.calls, "friendship")
	if f.friendships == nil {
		return true, nil
	}
	return f.friendships[friendID], nil
}

func (f *fakeRepository) CreateDrinkLog(_ context.Context, _ string, log NewDrinkLog) (map[string]any, error) {
	f.calls = append(f.calls, "create")
	f.newLog = log
	if f.createdLog != nil {
		return f.createdLog, nil
	}
	return map[string]any{"id": testLogID, "owner_user_id": log.OwnerUserID, "marker_rarity": string(log.MarkerRarity)}, nil
}

func (f *fakeRepository) CreateDrinkLogFriendLinks(_ context.Context, _ string, _ string, friendIDs []string) error {
	f.calls = append(f.calls, "links")
	f.links = friendIDs
	return nil
}

func (f *fakeRepository) DeleteOwnedDrinkLog(context.Context, string, string, string) (map[string]any, error) {
	f.calls = append(f.calls, "delete")
	return f.deletedLog, nil
}

func (f *fakeRepository) CreateLike(context.Context, string, string, string) (bool, error) {
	f.calls = append(f.calls, "create_like")
	return f.likeCreated, nil
}

func (f *fakeRepository) DeleteLike(context.Context, string, string, string) error {
	f.calls = append(f.calls, "delete_like")
	return nil
}

func (f *fakeRepository) LikeState(context.Context, string, string, string) (LikeState, error) {
	f.calls = append(f.calls, "like_state")
	return f.likeState, nil
}

func (f *fakeRepository) HiddenDrinkLogIDs(context.Context, string, string) (map[string]bool, error) {
	f.calls = append(f.calls, "hidden_reports")
	if f.hiddenIDs != nil {
		return f.hiddenIDs, nil
	}
	return map[string]bool{}, nil
}

func (f *fakeRepository) DrinkLogOwnerUserID(context.Context, string, string) (string, error) {
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

type fakeNotifier struct {
	tagged int
	liked  int
}

func (f *fakeNotifier) DrinkLogTagged(context.Context, string, string, string, []string) {
	f.tagged++
}

func (f *fakeNotifier) DrinkLogLiked(context.Context, string, string, string) {
	f.liked++
}

func TestCreateDrinkLogRejectsNonFriendTagBeforeInsert(t *testing.T) {
	repo := &fakeRepository{friendships: map[string]bool{otherUserID: false}}
	usecase := NewUsecase(Dependencies{Repository: repo})

	_, err := usecase.CreateDrinkLog(context.Background(), CreateInput{
		AuthToken:   testAuthToken,
		OwnerUserID: testUserID,
		FriendIDs:   []string{otherUserID},
	})

	assertUserError(t, err, ErrorKindForbidden, "friend_ids must be existing friends")
	if want := []string{"friendship"}; !reflect.DeepEqual(repo.calls, want) {
		t.Fatalf("calls = %v, want %v", repo.calls, want)
	}
}

func TestCreateDrinkLogRejectsExistingLogOnSameDay(t *testing.T) {
	repo := &fakeRepository{hasDailyLog: true}
	usecase := NewUsecase(Dependencies{Repository: repo})
	drankAt := time.Date(2026, 5, 24, 12, 30, 0, 0, time.UTC)
	offset := 9 * 60

	_, err := usecase.CreateDrinkLog(context.Background(), CreateInput{
		AuthToken:             testAuthToken,
		OwnerUserID:           testUserID,
		DrankAt:               &drankAt,
		DrankOn:               "2026-05-24",
		TimezoneOffsetMinutes: &offset,
	})

	assertUserError(t, err, ErrorKindConflict, "投稿は1日1回までです")
	if want := []string{"daily_limit"}; !reflect.DeepEqual(repo.calls, want) {
		t.Fatalf("calls = %v, want %v", repo.calls, want)
	}
}

func TestCreateDrinkLogRejectsInvalidPhotoPath(t *testing.T) {
	repo := &fakeRepository{}
	usecase := NewUsecase(Dependencies{Repository: repo})

	_, err := usecase.CreateDrinkLog(context.Background(), CreateInput{
		AuthToken:   testAuthToken,
		OwnerUserID: testUserID,
		PhotoPath:   "users/" + otherUserID + "/drink_logs/photo.jpg",
	})

	assertUserError(t, err, ErrorKindInvalidInput, "photo_path must be an uploaded drink-log photo")
	if want := []string{"daily_limit"}; !reflect.DeepEqual(repo.calls, want) {
		t.Fatalf("calls = %v, want %v", repo.calls, want)
	}
}

func TestCreateDrinkLogAssignsRarityOnBackendAndIgnoresClientRarity(t *testing.T) {
	repo := &fakeRepository{}
	notifier := &fakeNotifier{}
	usecase := NewUsecase(Dependencies{
		Repository:  repo,
		Notifier:    notifier,
		RandomFloat: func() float64 { return 0.005 },
	})

	row, err := usecase.CreateDrinkLog(context.Background(), CreateInput{
		AuthToken:             testAuthToken,
		OwnerUserID:           testUserID,
		PhotoPath:             "users/" + testUserID + "/drink_logs/photo.jpg",
		FriendIDs:             []string{otherUserID, otherUserID},
		ClientRequestedRarity: "secret",
	})
	if err != nil {
		t.Fatalf("CreateDrinkLog returned error: %v", err)
	}
	if row["id"] != testLogID {
		t.Fatalf("row = %#v", row)
	}
	if repo.newLog.MarkerRarity != MarkerRarityUltraRare {
		t.Fatalf("marker rarity = %s, want %s", repo.newLog.MarkerRarity, MarkerRarityUltraRare)
	}
	if repo.newLog.PhotoPath != "users/"+testUserID+"/drink_logs/photo.jpg" {
		t.Fatalf("photo path = %q", repo.newLog.PhotoPath)
	}
	if !reflect.DeepEqual(repo.links, []string{otherUserID}) {
		t.Fatalf("links = %v, want deduplicated friend id", repo.links)
	}
	if notifier.tagged != 1 {
		t.Fatalf("tagged notifications = %d, want 1", notifier.tagged)
	}
}

func TestCreateDrinkLogUsesNormalRarityWithoutPhoto(t *testing.T) {
	repo := &fakeRepository{}
	usecase := NewUsecase(Dependencies{Repository: repo, RandomFloat: func() float64 { return 0 }})

	_, err := usecase.CreateDrinkLog(context.Background(), CreateInput{
		AuthToken:   testAuthToken,
		OwnerUserID: testUserID,
	})
	if err != nil {
		t.Fatalf("CreateDrinkLog returned error: %v", err)
	}
	if repo.newLog.MarkerRarity != MarkerRarityNormal {
		t.Fatalf("marker rarity = %s, want normal", repo.newLog.MarkerRarity)
	}
}

func TestLikeDrinkLogNotifiesOnlyWhenLikeCreated(t *testing.T) {
	for _, tc := range []struct {
		name         string
		created      bool
		wantNotified int
	}{
		{name: "created", created: true, wantNotified: 1},
		{name: "duplicate", created: false, wantNotified: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeRepository{likeCreated: tc.created, likeState: LikeState{LikeCount: 2, LikedByMe: true}}
			notifier := &fakeNotifier{}
			usecase := NewUsecase(Dependencies{Repository: repo, Notifier: notifier})

			state, err := usecase.LikeDrinkLog(context.Background(), LikeInput{AuthToken: testAuthToken, LogID: testLogID, UserID: testUserID})
			if err != nil {
				t.Fatalf("LikeDrinkLog returned error: %v", err)
			}
			if state.LikeCount != 2 || !state.LikedByMe {
				t.Fatalf("state = %#v", state)
			}
			if notifier.liked != tc.wantNotified {
				t.Fatalf("liked notifications = %d, want %d", notifier.liked, tc.wantNotified)
			}
		})
	}
}

func TestReportDrinkLogCreatesModerationReportAndHidesPost(t *testing.T) {
	repo := &fakeRepository{reportOwnerID: otherUserID}
	usecase := NewUsecase(Dependencies{Repository: repo})

	result, err := usecase.ReportDrinkLog(context.Background(), ReportInput{
		AuthToken:      testAuthToken,
		LogID:          testLogID,
		ReporterUserID: testUserID,
		Reason:         " harassment ",
	})
	if err != nil {
		t.Fatalf("ReportDrinkLog returned error: %v", err)
	}
	if !result.Created || result.Body["duplicate"] != false || result.Body["hidden"] != true || result.Body["reason"] != "harassment" {
		t.Fatalf("result = %#v", result)
	}
	if repo.createdReport.DrinkLogID != testLogID || repo.createdReport.ReporterUserID != testUserID || repo.createdReport.Reason != ReportReasonHarassment {
		t.Fatalf("created report = %#v", repo.createdReport)
	}
	if want := []string{"log_owner", "find_report", "create_report"}; !reflect.DeepEqual(repo.calls, want) {
		t.Fatalf("calls = %v, want %v", repo.calls, want)
	}
}

func TestReportDrinkLogRejectsOwnLog(t *testing.T) {
	repo := &fakeRepository{reportOwnerID: testUserID}
	usecase := NewUsecase(Dependencies{Repository: repo})

	_, err := usecase.ReportDrinkLog(context.Background(), ReportInput{AuthToken: testAuthToken, LogID: testLogID, ReporterUserID: testUserID})
	assertUserError(t, err, ErrorKindForbidden, "cannot report your own drink log")
	if want := []string{"log_owner"}; !reflect.DeepEqual(repo.calls, want) {
		t.Fatalf("calls = %v, want %v", repo.calls, want)
	}
}

func TestReportDrinkLogReturnsDuplicateForExistingReport(t *testing.T) {
	repo := &fakeRepository{reportOwnerID: otherUserID, report: &Report{ID: "report-id", DrinkLogID: testLogID, ReporterUserID: testUserID, Reason: ReportReasonSpam}}
	usecase := NewUsecase(Dependencies{Repository: repo})

	result, err := usecase.ReportDrinkLog(context.Background(), ReportInput{AuthToken: testAuthToken, LogID: testLogID, ReporterUserID: testUserID, Reason: "spam"})
	if err != nil {
		t.Fatalf("ReportDrinkLog returned error: %v", err)
	}
	if result.Created || result.Body["duplicate"] != true || result.Body["status"] != "pending" {
		t.Fatalf("result = %#v", result)
	}
	if want := []string{"log_owner", "find_report"}; !reflect.DeepEqual(repo.calls, want) {
		t.Fatalf("calls = %v, want %v", repo.calls, want)
	}
}

func TestListDrinkLogsExcludesHiddenReportedRows(t *testing.T) {
	repo := &fakeRepository{
		logs: []map[string]any{
			{"id": "visible", "drank_at": "2026-05-24T12:00:00Z", "drink_log_likes": []any{}},
			{"id": "hidden", "drank_at": "2026-05-24T13:00:00Z", "drink_log_likes": []any{}},
		},
		hiddenIDs: map[string]bool{"hidden": true},
	}
	usecase := NewUsecase(Dependencies{Repository: repo})

	rows, err := usecase.ListDrinkLogs(context.Background(), ListInput{AuthToken: testAuthToken, UserID: testUserID})
	if err != nil {
		t.Fatalf("ListDrinkLogs returned error: %v", err)
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
