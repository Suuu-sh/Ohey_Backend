package invites

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
	testInviteID  = "33333333-4444-5555-6666-777777777777"
)

type fakeRepository struct {
	calls         []string
	dailyStatus   string
	blocked       bool
	alreadyFriend bool
	existing      *ExistingInvite
	created       map[string]any
	updated       map[string]any

	createdInvite NewInvite
	updatedInvite struct {
		inviteID        string
		recipientUserID string
		status          InviteStatus
		respondedAt     time.Time
	}
}

func (f *fakeRepository) ListTodayReservations(context.Context, string, string, string) ([]map[string]any, error) {
	f.calls = append(f.calls, "list_today")
	return nil, nil
}

func (f *fakeRepository) ListIncomingPending(context.Context, string, string, string) ([]map[string]any, error) {
	f.calls = append(f.calls, "list_incoming")
	return nil, nil
}

func (f *fakeRepository) ListOutgoingActive(context.Context, string, string, string) ([]map[string]any, error) {
	f.calls = append(f.calls, "list_outgoing")
	return nil, nil
}

func (f *fakeRepository) DailyStatus(context.Context, string, string, string) (string, error) {
	f.calls = append(f.calls, "daily_status")
	return f.dailyStatus, nil
}

func (f *fakeRepository) BlockExistsBetweenUsers(context.Context, string, string, string) (bool, error) {
	f.calls = append(f.calls, "block")
	return f.blocked, nil
}

func (f *fakeRepository) FriendshipExists(context.Context, string, string, string) (bool, error) {
	f.calls = append(f.calls, "friendship")
	return f.alreadyFriend, nil
}

func (f *fakeRepository) FindActiveInviteBetweenUsersForDate(context.Context, string, string, string, string) (*ExistingInvite, error) {
	f.calls = append(f.calls, "find_active")
	return f.existing, nil
}

func (f *fakeRepository) CreateInvite(_ context.Context, _ string, invite NewInvite) (map[string]any, error) {
	f.calls = append(f.calls, "create")
	f.createdInvite = invite
	if f.created != nil {
		return f.created, nil
	}
	return map[string]any{
		"id":              testInviteID,
		"inviter_user_id": invite.InviterUserID,
		"invitee_user_id": invite.InviteeUserID,
		"scheduled_date":  invite.ScheduledDate,
		"activity_label":  invite.ActivityLabel,
		"status":          string(InviteStatusPending),
	}, nil
}

func (f *fakeRepository) UpdatePendingInviteStatus(_ context.Context, _ string, inviteID, recipientUserID string, status InviteStatus, respondedAt time.Time) (map[string]any, error) {
	f.calls = append(f.calls, "update")
	f.updatedInvite.inviteID = inviteID
	f.updatedInvite.recipientUserID = recipientUserID
	f.updatedInvite.status = status
	f.updatedInvite.respondedAt = respondedAt
	return f.updated, nil
}

type fakePublisher struct {
	events []DomainEvent
}

func (f *fakePublisher) Publish(_ context.Context, _ string, event DomainEvent) {
	f.events = append(f.events, event)
}

func TestCreateInviteRejectsSelfInviteBeforeRepositoryAccess(t *testing.T) {
	repo := &fakeRepository{}
	usecase := NewUsecase(Dependencies{Repository: repo})

	_, err := usecase.CreateInvite(context.Background(), CreateInput{
		AuthToken:     testAuthToken,
		InviterUserID: testUserID,
		InviteeUserID: testUserID,
		ScheduledDate: "2026-05-23",
	})

	assertUserError(t, err, ErrorKindInvalidInput, "cannot invite yourself")
	if len(repo.calls) != 0 {
		t.Fatalf("repository calls = %v, want none", repo.calls)
	}
}

func TestCreateInviteBlocksHasPlansDailyStatus(t *testing.T) {
	repo := &fakeRepository{dailyStatus: "has_plans", alreadyFriend: true}
	usecase := NewUsecase(Dependencies{Repository: repo})

	_, err := usecase.CreateInvite(context.Background(), CreateInput{
		AuthToken:     testAuthToken,
		InviterUserID: testUserID,
		InviteeUserID: otherUserID,
		ScheduledDate: "2026-05-23",
		ActivityLabel: "  焼肉に行く  ",
	})

	assertUserError(t, err, ErrorKindConflict, "相手に予定があるため今日は誘えません。")
	if want := []string{"block", "friendship", "daily_status"}; !reflect.DeepEqual(repo.calls, want) {
		t.Fatalf("repository calls = %v, want %v", repo.calls, want)
	}
}

func TestCreateInviteRejectsNonFriendBeforeDailyStatusLookup(t *testing.T) {
	repo := &fakeRepository{}
	usecase := NewUsecase(Dependencies{Repository: repo})

	_, err := usecase.CreateInvite(context.Background(), CreateInput{
		AuthToken:     testAuthToken,
		InviterUserID: testUserID,
		InviteeUserID: otherUserID,
		ScheduledDate: "2026-05-23",
	})

	assertUserError(t, err, ErrorKindForbidden, "friendship is required")
	if want := []string{"block", "friendship"}; !reflect.DeepEqual(repo.calls, want) {
		t.Fatalf("repository calls = %v, want %v", repo.calls, want)
	}
}

func TestCreateInviteRejectsExistingAcceptedInvite(t *testing.T) {
	repo := &fakeRepository{alreadyFriend: true, existing: &ExistingInvite{ID: testInviteID, Status: InviteStatusAccepted}}
	usecase := NewUsecase(Dependencies{Repository: repo})

	_, err := usecase.CreateInvite(context.Background(), CreateInput{
		AuthToken:     testAuthToken,
		InviterUserID: testUserID,
		InviteeUserID: otherUserID,
		ScheduledDate: "2026-05-23",
	})

	assertUserError(t, err, ErrorKindConflict, "今日はもう予約済みです。")
	if want := []string{"block", "friendship", "daily_status", "find_active"}; !reflect.DeepEqual(repo.calls, want) {
		t.Fatalf("repository calls = %v, want %v", repo.calls, want)
	}
}

func TestCreateInviteRejectsLongActivityLabelBeforeRepositoryAccess(t *testing.T) {
	repo := &fakeRepository{alreadyFriend: true}
	usecase := NewUsecase(Dependencies{Repository: repo})

	_, err := usecase.CreateInvite(context.Background(), CreateInput{
		AuthToken:     testAuthToken,
		InviterUserID: testUserID,
		InviteeUserID: otherUserID,
		ScheduledDate: "2026-05-23",
		ActivityLabel: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	})

	assertUserError(t, err, ErrorKindInvalidInput, "activity_label must be 40 characters or fewer")
	if len(repo.calls) != 0 {
		t.Fatalf("repository calls = %v, want none", repo.calls)
	}
}

func TestCreateInviteCreatesPendingInviteAndNotifiesRecipient(t *testing.T) {
	repo := &fakeRepository{alreadyFriend: true}
	publisher := &fakePublisher{}
	usecase := NewUsecase(Dependencies{Repository: repo, Publisher: publisher})

	row, err := usecase.CreateInvite(context.Background(), CreateInput{
		AuthToken:     testAuthToken,
		InviterUserID: testUserID,
		InviteeUserID: otherUserID,
		ScheduledDate: "2026-05-23",
		ActivityLabel: "  焼肉に行く  ",
	})
	if err != nil {
		t.Fatalf("CreateInvite returned error: %v", err)
	}
	if row["status"] != string(InviteStatusPending) {
		t.Fatalf("created status = %#v", row["status"])
	}
	if repo.createdInvite.ScheduledDate != "2026-05-23" || repo.createdInvite.InviterUserID != testUserID || repo.createdInvite.InviteeUserID != otherUserID || repo.createdInvite.ActivityLabel != "焼肉に行く" {
		t.Fatalf("created invite = %#v", repo.createdInvite)
	}
	if len(publisher.events) != 1 || publisher.events[0].Kind != EventInviteCreated || publisher.events[0].Invite.InviteeUserID != otherUserID {
		t.Fatalf("events = %#v", publisher.events)
	}
	if want := []string{"block", "friendship", "daily_status", "find_active", "create"}; !reflect.DeepEqual(repo.calls, want) {
		t.Fatalf("repository calls = %v, want %v", repo.calls, want)
	}
}

func TestUpdateInviteAcceptedNotifiesRequester(t *testing.T) {
	respondedAt := time.Date(2026, 5, 23, 12, 34, 56, 0, time.FixedZone("JST", 9*60*60))
	repo := &fakeRepository{updated: map[string]any{"id": testInviteID, "inviter_user_id": otherUserID, "invitee_user_id": testUserID, "scheduled_date": "2026-05-23", "status": string(InviteStatusAccepted)}}
	publisher := &fakePublisher{}
	usecase := NewUsecase(Dependencies{
		Repository: repo,
		Publisher:  publisher,
		Now:        func() time.Time { return respondedAt },
	})

	row, err := usecase.UpdateInvite(context.Background(), UpdateInput{
		AuthToken:       testAuthToken,
		InviteID:        testInviteID,
		RecipientUserID: testUserID,
		Status:          "accepted",
	})
	if err != nil {
		t.Fatalf("UpdateInvite returned error: %v", err)
	}
	if row["id"] != testInviteID {
		t.Fatalf("updated row = %#v", row)
	}
	if repo.updatedInvite.status != InviteStatusAccepted || repo.updatedInvite.inviteID != testInviteID || repo.updatedInvite.recipientUserID != testUserID {
		t.Fatalf("updated invite args = %#v", repo.updatedInvite)
	}
	if got := repo.updatedInvite.respondedAt; !got.Equal(respondedAt.UTC()) {
		t.Fatalf("respondedAt = %s, want %s", got, respondedAt.UTC())
	}
	if len(publisher.events) != 1 || publisher.events[0].Kind != EventInviteAccepted {
		t.Fatalf("events = %#v", publisher.events)
	}
}

func TestUpdateInviteRejectedDoesNotNotify(t *testing.T) {
	repo := &fakeRepository{updated: map[string]any{"id": testInviteID, "inviter_user_id": otherUserID, "invitee_user_id": testUserID, "scheduled_date": "2026-05-23", "status": string(InviteStatusRejected)}}
	publisher := &fakePublisher{}
	usecase := NewUsecase(Dependencies{Repository: repo, Publisher: publisher})

	_, err := usecase.UpdateInvite(context.Background(), UpdateInput{
		AuthToken:       testAuthToken,
		InviteID:        testInviteID,
		RecipientUserID: testUserID,
		Status:          "rejected",
	})
	if err != nil {
		t.Fatalf("UpdateInvite returned error: %v", err)
	}
	if len(publisher.events) != 0 {
		t.Fatalf("events = %#v, want none", publisher.events)
	}
}

func TestUpdateInviteReturnsNotFoundWhenRepositoryUpdatesNoRows(t *testing.T) {
	repo := &fakeRepository{}
	usecase := NewUsecase(Dependencies{Repository: repo})

	_, err := usecase.UpdateInvite(context.Background(), UpdateInput{
		AuthToken:       testAuthToken,
		InviteID:        testInviteID,
		RecipientUserID: testUserID,
		Status:          "accepted",
	})

	assertUserError(t, err, ErrorKindNotFound, "invite not found")
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
