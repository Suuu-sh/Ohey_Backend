package friends

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
	testRequestID = "22222222-3333-4444-5555-666666666666"
)

type fakeRepository struct {
	calls          []string
	friendships    []map[string]any
	alreadyFriend  bool
	pendingRequest map[string]any
	createdRequest map[string]any
	updatedRequest map[string]any
	favoriteRow    map[string]any
	friendshipRow  map[string]any
	statusesDate   string
	drinkStatsUser string
}

func (f *fakeRepository) ListFriendships(context.Context, string, string) ([]map[string]any, error) {
	f.calls = append(f.calls, "list")
	return f.friendships, nil
}

func (f *fakeRepository) AttachTodayStatuses(_ context.Context, _ string, _ []map[string]any, date string) error {
	f.calls = append(f.calls, "statuses")
	f.statusesDate = date
	return nil
}

func (f *fakeRepository) AttachDrinkStats(_ context.Context, _ string, currentUserID string, _ []map[string]any) error {
	f.calls = append(f.calls, "stats")
	f.drinkStatsUser = currentUserID
	return nil
}

func (f *fakeRepository) UpdateFriendFavorite(context.Context, string, string, string, bool) (map[string]any, error) {
	f.calls = append(f.calls, "favorite")
	return f.favoriteRow, nil
}

func (f *fakeRepository) UpsertFriendshipPair(context.Context, string, string, string) (map[string]any, error) {
	f.calls = append(f.calls, "upsert")
	if f.friendshipRow != nil {
		return f.friendshipRow, nil
	}
	return map[string]any{"id": "friendship"}, nil
}

func (f *fakeRepository) FriendshipExists(context.Context, string, string, string) (bool, error) {
	f.calls = append(f.calls, "exists")
	return f.alreadyFriend, nil
}

func (f *fakeRepository) PendingFriendRequestBetween(context.Context, string, string, string) (map[string]any, error) {
	f.calls = append(f.calls, "pending")
	return f.pendingRequest, nil
}

func (f *fakeRepository) CreateFriendRequest(context.Context, string, string, string) (map[string]any, error) {
	f.calls = append(f.calls, "create_request")
	if f.createdRequest != nil {
		return f.createdRequest, nil
	}
	return map[string]any{"id": testRequestID, "from_user_id": testUserID, "to_user_id": otherUserID, "status": "pending"}, nil
}

func (f *fakeRepository) UpdatePendingFriendRequestStatus(context.Context, string, string, string, RequestStatus, time.Time) (map[string]any, error) {
	f.calls = append(f.calls, "update_request")
	return f.updatedRequest, nil
}

type fakeNotifier struct {
	received int
	accepted int
}

func (f *fakeNotifier) FriendRequestReceived(context.Context, string, map[string]any) {
	f.received++
}

func (f *fakeNotifier) FriendRequestAccepted(context.Context, string, map[string]any) {
	f.accepted++
}

func TestListFriendsAttachesStatusAndStats(t *testing.T) {
	repo := &fakeRepository{friendships: []map[string]any{{"id": "friendship"}}}
	usecase := NewUsecase(Dependencies{Repository: repo})

	rows, err := usecase.ListFriends(context.Background(), ListInput{AuthToken: testAuthToken, UserID: testUserID, Date: "2026-05-24"})
	if err != nil {
		t.Fatalf("ListFriends returned error: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %#v", rows)
	}
	if want := []string{"list", "statuses", "stats"}; !reflect.DeepEqual(repo.calls, want) {
		t.Fatalf("calls = %v, want %v", repo.calls, want)
	}
	if repo.statusesDate != "2026-05-24" || repo.drinkStatsUser != testUserID {
		t.Fatalf("date/user = %q/%q", repo.statusesDate, repo.drinkStatsUser)
	}
}

func TestGetFriendRequestStatusSelf(t *testing.T) {
	repo := &fakeRepository{}
	usecase := NewUsecase(Dependencies{Repository: repo})

	status, err := usecase.GetFriendRequestStatus(context.Background(), FriendInput{AuthToken: testAuthToken, UserID: testUserID, FriendID: testUserID})
	if err != nil {
		t.Fatalf("GetFriendRequestStatus returned error: %v", err)
	}
	if status.AlreadyFriend || status.RequestState != "self" {
		t.Fatalf("status = %#v", status)
	}
	if len(repo.calls) != 0 {
		t.Fatalf("calls = %v, want none", repo.calls)
	}
}

func TestCreateFriendRequestRejectsExistingFriendBeforeInsert(t *testing.T) {
	repo := &fakeRepository{alreadyFriend: true}
	usecase := NewUsecase(Dependencies{Repository: repo})

	_, err := usecase.CreateFriendRequest(context.Background(), CreateFriendRequestInput{AuthToken: testAuthToken, FromUserID: testUserID, ToUserID: otherUserID})
	assertUserError(t, err, ErrorKindConflict, "already friends")
	if want := []string{"exists"}; !reflect.DeepEqual(repo.calls, want) {
		t.Fatalf("calls = %v, want %v", repo.calls, want)
	}
}

func TestCreateFriendRequestCreatesNotification(t *testing.T) {
	repo := &fakeRepository{}
	notifier := &fakeNotifier{}
	usecase := NewUsecase(Dependencies{Repository: repo, Notifier: notifier})

	row, err := usecase.CreateFriendRequest(context.Background(), CreateFriendRequestInput{AuthToken: testAuthToken, FromUserID: testUserID, ToUserID: otherUserID})
	if err != nil {
		t.Fatalf("CreateFriendRequest returned error: %v", err)
	}
	if row["id"] != testRequestID {
		t.Fatalf("row = %#v", row)
	}
	if notifier.received != 1 {
		t.Fatalf("received notifications = %d, want 1", notifier.received)
	}
}

func TestUpdateFriendRequestAcceptedCreatesFriendshipAndNotification(t *testing.T) {
	repo := &fakeRepository{updatedRequest: map[string]any{"id": testRequestID, "from_user_id": otherUserID, "to_user_id": testUserID, "status": "accepted"}}
	notifier := &fakeNotifier{}
	usecase := NewUsecase(Dependencies{Repository: repo, Notifier: notifier})

	row, err := usecase.UpdateFriendRequest(context.Background(), UpdateFriendRequestInput{AuthToken: testAuthToken, RequestID: testRequestID, UserID: testUserID, Status: "accepted"})
	if err != nil {
		t.Fatalf("UpdateFriendRequest returned error: %v", err)
	}
	if row["status"] != "accepted" {
		t.Fatalf("row = %#v", row)
	}
	if want := []string{"update_request", "upsert"}; !reflect.DeepEqual(repo.calls, want) {
		t.Fatalf("calls = %v, want %v", repo.calls, want)
	}
	if notifier.accepted != 1 {
		t.Fatalf("accepted notifications = %d, want 1", notifier.accepted)
	}
}

func TestUpdateFriendRequestNotFound(t *testing.T) {
	repo := &fakeRepository{}
	usecase := NewUsecase(Dependencies{Repository: repo})

	_, err := usecase.UpdateFriendRequest(context.Background(), UpdateFriendRequestInput{AuthToken: testAuthToken, RequestID: testRequestID, UserID: testUserID, Status: "rejected"})
	assertUserError(t, err, ErrorKindNotFound, "friend request not found")
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
