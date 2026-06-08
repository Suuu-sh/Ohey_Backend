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
	blocked        bool
	pendingRequest map[string]any
	createdRequest map[string]any
	updatedRequest map[string]any
	favoriteRow    map[string]any
	friendshipRow  map[string]any
	statusesDate   string
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

func (f *fakeRepository) DeleteFriendship(context.Context, string, string, string) (map[string]any, error) {
	f.calls = append(f.calls, "delete")
	return f.friendshipRow, nil
}

func (f *fakeRepository) FriendshipExists(context.Context, string, string, string) (bool, error) {
	f.calls = append(f.calls, "exists")
	return f.alreadyFriend, nil
}

func (f *fakeRepository) BlockExistsBetweenUsers(context.Context, string, string, string) (bool, error) {
	f.calls = append(f.calls, "block")
	return f.blocked, nil
}

func (f *fakeRepository) ListPendingFriendRequests(context.Context, string, string, RequestDirection) ([]map[string]any, error) {
	f.calls = append(f.calls, "list_requests")
	return []map[string]any{{"id": testRequestID}}, nil
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

type fakePublisher struct {
	events []DomainEvent
}

func (f *fakePublisher) Publish(_ context.Context, _ string, event DomainEvent) {
	f.events = append(f.events, event)
}

func TestListFriendsAttachesStatus(t *testing.T) {
	repo := &fakeRepository{friendships: []map[string]any{{"id": "friendship"}}}
	usecase := NewUsecase(Dependencies{Repository: repo})

	rows, err := usecase.ListFriends(context.Background(), ListInput{AuthToken: testAuthToken, UserID: testUserID, Date: "2026-05-24"})
	if err != nil {
		t.Fatalf("ListFriends returned error: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %#v", rows)
	}
	if want := []string{"list", "statuses"}; !reflect.DeepEqual(repo.calls, want) {
		t.Fatalf("calls = %v, want %v", repo.calls, want)
	}
	if repo.statusesDate != "2026-05-24" {
		t.Fatalf("date = %q", repo.statusesDate)
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

func TestGetFriendRequestStatusIncludesPendingRequestID(t *testing.T) {
	repo := &fakeRepository{
		pendingRequest: map[string]any{
			"id":           testRequestID,
			"from_user_id": testUserID,
			"to_user_id":   otherUserID,
		},
	}
	usecase := NewUsecase(Dependencies{Repository: repo})

	status, err := usecase.GetFriendRequestStatus(context.Background(), FriendInput{AuthToken: testAuthToken, UserID: testUserID, FriendID: otherUserID})
	if err != nil {
		t.Fatalf("GetFriendRequestStatus returned error: %v", err)
	}
	if status.RequestState != "outgoing" || status.RequestID != testRequestID {
		t.Fatalf("status = %#v", status)
	}
}

func TestListFriendRequestsDefaultsToAll(t *testing.T) {
	repo := &fakeRepository{}
	usecase := NewUsecase(Dependencies{Repository: repo})

	rows, err := usecase.ListFriendRequests(context.Background(), ListFriendRequestsInput{AuthToken: testAuthToken, UserID: testUserID})
	if err != nil {
		t.Fatalf("ListFriendRequests returned error: %v", err)
	}
	if len(rows) != 1 || rows[0]["id"] != testRequestID {
		t.Fatalf("rows = %#v", rows)
	}
	if want := []string{"list_requests"}; !reflect.DeepEqual(repo.calls, want) {
		t.Fatalf("calls = %v, want %v", repo.calls, want)
	}
}

func TestDeleteFriendshipRequiresExistingRow(t *testing.T) {
	repo := &fakeRepository{}
	usecase := NewUsecase(Dependencies{Repository: repo})

	_, err := usecase.DeleteFriendship(context.Background(), FriendInput{AuthToken: testAuthToken, UserID: testUserID, FriendID: otherUserID})
	assertUserError(t, err, ErrorKindNotFound, "friendship not found")
}

func TestCreateFriendRequestRejectsExistingFriendBeforeInsert(t *testing.T) {
	repo := &fakeRepository{alreadyFriend: true}
	usecase := NewUsecase(Dependencies{Repository: repo})

	_, err := usecase.CreateFriendRequest(context.Background(), CreateFriendRequestInput{AuthToken: testAuthToken, FromUserID: testUserID, ToUserID: otherUserID})
	assertUserError(t, err, ErrorKindConflict, "already friends")
	if want := []string{"block", "exists"}; !reflect.DeepEqual(repo.calls, want) {
		t.Fatalf("calls = %v, want %v", repo.calls, want)
	}
}

func TestCreateFriendRequestCreatesNotification(t *testing.T) {
	repo := &fakeRepository{}
	publisher := &fakePublisher{}
	usecase := NewUsecase(Dependencies{Repository: repo, Publisher: publisher})

	row, err := usecase.CreateFriendRequest(context.Background(), CreateFriendRequestInput{AuthToken: testAuthToken, FromUserID: testUserID, ToUserID: otherUserID})
	if err != nil {
		t.Fatalf("CreateFriendRequest returned error: %v", err)
	}
	if row["id"] != testRequestID {
		t.Fatalf("row = %#v", row)
	}
	if len(publisher.events) != 1 || publisher.events[0].Kind != EventFriendRequestCreated {
		t.Fatalf("events = %#v, want friend request created", publisher.events)
	}
}

func TestUpdateFriendRequestAcceptedCreatesFriendshipAndNotification(t *testing.T) {
	repo := &fakeRepository{updatedRequest: map[string]any{"id": testRequestID, "from_user_id": otherUserID, "to_user_id": testUserID, "status": "accepted"}}
	publisher := &fakePublisher{}
	usecase := NewUsecase(Dependencies{Repository: repo, Publisher: publisher})

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
	if len(publisher.events) != 1 || publisher.events[0].Kind != EventFriendRequestAccepted {
		t.Fatalf("events = %#v, want friend request accepted", publisher.events)
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
