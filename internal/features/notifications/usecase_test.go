package notifications

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
	thirdUserID   = "cccccccc-dddd-eeee-ffff-000000000000"
	testMemoryID  = "11111111-2222-3333-4444-555555555555"
	testRequestID = "22222222-3333-4444-5555-666666666666"
	testInviteID  = "33333333-4444-5555-6666-777777777777"
)

type fakeRepository struct {
	created          []Notification
	listRows         []map[string]any
	updatedCount     int
	displayNames     map[string]string
	memoryOwnerID    string
	invites          []Invite
	allProfileIDs    []string
	pushTokens       []string
	deletedTokens    []string
	createReturn     []bool
	createCalls      int
	listCalled       bool
	markReadUserID   string
	pushTokenQueries []string
}

func (f *fakeRepository) CreateNotification(_ context.Context, notification Notification) (bool, error) {
	created := true
	if f.createCalls < len(f.createReturn) {
		created = f.createReturn[f.createCalls]
	}
	f.createCalls++
	f.created = append(f.created, notification)
	return created, nil
}

func (f *fakeRepository) ListNotifications(context.Context, string, string, int) ([]map[string]any, error) {
	f.listCalled = true
	return f.listRows, nil
}

func (f *fakeRepository) MarkAllRead(_ context.Context, _ string, recipientUserID string, _ time.Time) (int, error) {
	f.markReadUserID = recipientUserID
	return f.updatedCount, nil
}

func (f *fakeRepository) DisplayName(_ context.Context, _ string, userID string) (string, error) {
	if f.displayNames != nil {
		return f.displayNames[userID], nil
	}
	return "Actor", nil
}

func (f *fakeRepository) MemoryOwnerUserID(context.Context, string, string) (string, error) {
	return f.memoryOwnerID, nil
}

func (f *fakeRepository) TodayAcceptedInvites(context.Context, string, string, string) ([]Invite, error) {
	return f.invites, nil
}

func (f *fakeRepository) AllProfileIDs(context.Context) ([]string, error) {
	return f.allProfileIDs, nil
}

func (f *fakeRepository) VisibleYuruboRecipientIDs(context.Context, string, string, string, []string) ([]string, error) {
	return f.allProfileIDs, nil
}

func (f *fakeRepository) PushTokens(_ context.Context, recipientUserID string) ([]string, error) {
	f.pushTokenQueries = append(f.pushTokenQueries, recipientUserID)
	return f.pushTokens, nil
}

func (f *fakeRepository) DeletePushToken(_ context.Context, token string) error {
	f.deletedTokens = append(f.deletedTokens, token)
	return nil
}

type fakePushSender struct {
	sent []map[string]string
	err  error
}

func (f *fakePushSender) Send(_ context.Context, token, title, body string, data map[string]string) error {
	f.sent = append(f.sent, map[string]string{"token": token, "title": title, "body": body, "kind": data["kind"], "memory_id": data["memory_id"]})
	return f.err
}

type fakeInvalidPushTokenError struct{}

func (fakeInvalidPushTokenError) Error() string {
	return "invalid push token"
}

func (fakeInvalidPushTokenError) InvalidPushToken() bool {
	return true
}

func TestNotifyFriendRequestReceivedCreatesProductCopy(t *testing.T) {
	repo := &fakeRepository{displayNames: map[string]string{testUserID: "太郎"}}
	usecase := NewUsecase(Dependencies{Repository: repo})

	usecase.NotifyFriendRequestReceived(context.Background(), testAuthToken, map[string]any{
		"id":           testRequestID,
		"from_user_id": testUserID,
		"to_user_id":   otherUserID,
	})

	if len(repo.created) != 1 {
		t.Fatalf("created = %d, want 1", len(repo.created))
	}
	n := repo.created[0]
	if n.Kind != KindFriendRequestReceived || n.RecipientUserID != otherUserID || n.ActorUserID != testUserID || n.FriendRequestID != testRequestID {
		t.Fatalf("notification = %#v", n)
	}
	if n.Title != "フレンズ申請が届きました" || n.Message != "太郎さんからフレンズ申請が届きました。" {
		t.Fatalf("copy = %q / %q", n.Title, n.Message)
	}
}

func TestNotifyMemoryTaggedDeduplicatesRecipientsAndSkipsOwner(t *testing.T) {
	repo := &fakeRepository{}
	usecase := NewUsecase(Dependencies{Repository: repo})

	usecase.NotifyMemoryTagged(context.Background(), testAuthToken, testMemoryID, testUserID, []string{otherUserID, otherUserID, testUserID, thirdUserID})

	if len(repo.created) != 2 {
		t.Fatalf("created = %d, want 2", len(repo.created))
	}
	got := []string{repo.created[0].RecipientUserID, repo.created[1].RecipientUserID}
	want := []string{otherUserID, thirdUserID}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("recipients = %v, want %v", got, want)
	}
}

func TestInvalidPushTokenIsDeletedAfterSendFailure(t *testing.T) {
	repo := &fakeRepository{
		createReturn: []bool{true},
		pushTokens:   []string{"bad-token"},
	}
	push := &fakePushSender{err: fakeInvalidPushTokenError{}}
	usecase := NewUsecase(Dependencies{Repository: repo, PushSender: push})

	usecase.NotifyFriendRequestReceived(context.Background(), testAuthToken, map[string]any{
		"id":           testRequestID,
		"from_user_id": otherUserID,
		"to_user_id":   testUserID,
	})

	if len(repo.deletedTokens) != 1 || repo.deletedTokens[0] != "bad-token" {
		t.Fatalf("deletedTokens = %#v, want bad-token", repo.deletedTokens)
	}
}

func TestListNotificationsCreatesTodayReminderThenLists(t *testing.T) {
	repo := &fakeRepository{
		displayNames: map[string]string{otherUserID: "花子"},
		invites: []Invite{{
			ID:            testInviteID,
			InviterUserID: otherUserID,
			InviteeUserID: testUserID,
			ScheduledDate: "2026-05-23",
		}},
		listRows: []map[string]any{{"id": "notification"}},
	}
	usecase := NewUsecase(Dependencies{Repository: repo, Now: func() time.Time {
		return time.Date(2026, 5, 23, 10, 0, 0, 0, time.UTC)
	}})

	rows, err := usecase.ListNotifications(context.Background(), ListInput{AuthToken: testAuthToken, UserID: testUserID, Date: "2026-05-23"})
	if err != nil {
		t.Fatalf("ListNotifications returned error: %v", err)
	}
	if len(rows) != 1 || !repo.listCalled {
		t.Fatalf("rows/list = %#v/%v", rows, repo.listCalled)
	}
	if len(repo.created) != 1 {
		t.Fatalf("created = %d, want 1", len(repo.created))
	}
	n := repo.created[0]
	if n.Kind != KindTodayReservationReminder || n.NotificationDate != "2026-05-23" || n.Message != "花子さんとの予定が今日あります。" {
		t.Fatalf("reminder = %#v", n)
	}
}

func TestCreateSystemNotificationsValidatesAndDeduplicatesRecipients(t *testing.T) {
	repo := &fakeRepository{allProfileIDs: []string{otherUserID, thirdUserID}}
	usecase := NewUsecase(Dependencies{Repository: repo})

	result, err := usecase.CreateSystemNotifications(context.Background(), CreateSystemInput{
		Title:            " Title ",
		Message:          " Message ",
		RecipientUserIDs: []string{otherUserID, otherUserID},
		SendToAll:        true,
		SystemKey:        " release ",
	})
	if err != nil {
		t.Fatalf("CreateSystemNotifications returned error: %v", err)
	}
	if result.RecipientCount != 2 || result.CreatedCount != 2 {
		t.Fatalf("result = %#v", result)
	}
	if len(repo.created) != 2 {
		t.Fatalf("created = %d, want 2", len(repo.created))
	}
	if repo.created[0].Title != "Title" || repo.created[0].Message != "Message" || repo.created[0].SystemKey != "release" {
		t.Fatalf("system copy = %#v", repo.created[0])
	}
}

func TestCreateSystemNotificationsRejectsInvalidRecipient(t *testing.T) {
	repo := &fakeRepository{}
	usecase := NewUsecase(Dependencies{Repository: repo})

	_, err := usecase.CreateSystemNotifications(context.Background(), CreateSystemInput{
		Title:            "Title",
		Message:          "Message",
		RecipientUserIDs: []string{"bad"},
	})
	assertUserError(t, err, ErrorKindInvalidInput, "recipient user id must be a valid UUID")
	if len(repo.created) != 0 {
		t.Fatalf("created = %d, want 0", len(repo.created))
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
