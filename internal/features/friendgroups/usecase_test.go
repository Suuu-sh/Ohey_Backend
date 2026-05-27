package friendgroups

import (
	"context"
	"reflect"
	"testing"
)

const (
	testAuthToken = "access-token"
	testUserID    = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	friendUserID  = "bbbbbbbb-cccc-dddd-eeee-ffffffffffff"
)

type fakeRepository struct {
	friendships map[string]bool
	saved       []FriendGroup
}

func (f *fakeRepository) ListGroups(context.Context, string, string) ([]FriendGroup, error) {
	return f.saved, nil
}

func (f *fakeRepository) FriendshipExists(_ context.Context, _ string, _ string, friendUserID string) (bool, error) {
	if f.friendships == nil {
		return true, nil
	}
	return f.friendships[friendUserID], nil
}

func (f *fakeRepository) SaveGroups(_ context.Context, _ string, _ string, groups []FriendGroup) ([]FriendGroup, error) {
	f.saved = groups
	return groups, nil
}

func TestSaveFriendGroupsNormalizesLegacyFriendIdsAndDeduplicates(t *testing.T) {
	repo := &fakeRepository{}
	usecase := NewUsecase(Dependencies{Repository: repo})

	groups, err := usecase.SaveFriendGroups(context.Background(), SaveInput{
		AuthToken: testAuthToken,
		UserID:    testUserID,
		Body: SaveInputBody{Groups: []GroupInput{{
			ID:        " 12345 ",
			Name:      "  飲み友  ",
			FriendIds: []string{friendUserID, friendUserID},
		}}},
	})
	if err != nil {
		t.Fatalf("SaveFriendGroups returned error: %v", err)
	}
	if len(groups) != 1 || groups[0].ID != "12345" || groups[0].Name != "飲み友" {
		t.Fatalf("groups = %#v", groups)
	}
	if !reflect.DeepEqual(groups[0].FriendIDs, []string{friendUserID}) || !reflect.DeepEqual(groups[0].FriendIds, []string{friendUserID}) {
		t.Fatalf("friend ids = %#v / %#v", groups[0].FriendIDs, groups[0].FriendIds)
	}
}

func TestSaveFriendGroupsRejectsNonFriendMember(t *testing.T) {
	repo := &fakeRepository{friendships: map[string]bool{friendUserID: false}}
	usecase := NewUsecase(Dependencies{Repository: repo})

	_, err := usecase.SaveFriendGroups(context.Background(), SaveInput{
		AuthToken: testAuthToken,
		UserID:    testUserID,
		Body:      SaveInputBody{Groups: []GroupInput{{ID: "group1", Name: "Group", FriendIDs: []string{friendUserID}}}},
	})
	assertUserError(t, err, ErrorKindForbidden, "friend_ids must be existing friends")
}

func TestSaveFriendGroupsRejectsDuplicateGroupID(t *testing.T) {
	usecase := NewUsecase(Dependencies{Repository: &fakeRepository{}})

	_, err := usecase.SaveFriendGroups(context.Background(), SaveInput{
		AuthToken: testAuthToken,
		UserID:    testUserID,
		Body: SaveInputBody{Groups: []GroupInput{
			{ID: "group1", Name: "A", FriendIDs: []string{friendUserID}},
			{ID: "group1", Name: "B", FriendIDs: []string{friendUserID}},
		}},
	})
	assertUserError(t, err, ErrorKindInvalidInput, "group id is duplicated")
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
