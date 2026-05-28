package homefeed

import (
	"context"
	"testing"
)

const (
	testAuthToken = "access-token"
	testUserID    = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	friendUserID  = "bbbbbbbb-cccc-dddd-eeee-ffffffffffff"
)

type fakeRepository struct {
	visibleUserIDs   []string
	hiddenIDs        map[string]bool
	hiddenUserIDs    map[string]bool
	memories         []map[string]any
	officialMemories []map[string]any
}

func (f *fakeRepository) VisibleFeedUserIDs(context.Context, string, string) ([]string, error) {
	if f.visibleUserIDs != nil {
		return f.visibleUserIDs, nil
	}
	return []string{testUserID, friendUserID}, nil
}

func (f *fakeRepository) HiddenMemoryIDs(context.Context, string, string) (map[string]bool, error) {
	if f.hiddenIDs != nil {
		return f.hiddenIDs, nil
	}
	return map[string]bool{}, nil
}

func (f *fakeRepository) HiddenUserIDs(context.Context, string, string) (map[string]bool, error) {
	if f.hiddenUserIDs != nil {
		return f.hiddenUserIDs, nil
	}
	return map[string]bool{}, nil
}

func (f *fakeRepository) ListMemories(context.Context, string, []string) ([]map[string]any, error) {
	return f.memories, nil
}

func (f *fakeRepository) ListOfficialMemories(context.Context, string) ([]map[string]any, error) {
	return f.officialMemories, nil
}

func TestListHomeFeedShapesDisplayableItemsAndHidesReports(t *testing.T) {
	repo := &fakeRepository{
		hiddenIDs: map[string]bool{"hidden": true},
		memories: []map[string]any{
			{
				"id": "mine", "owner_user_id": testUserID, "happened_at": "2026-05-24T12:00:00Z", "photo_path": "users/me/memories/a.jpg", "caption_y": 0.7,
				"memo": " hello ", "place_name": " bar ", "owner": map[string]any{"display_name": "Me"}, "memory_likes": []any{map[string]any{"user_id": testUserID}},
			},
			{"id": "hidden", "owner_user_id": friendUserID, "happened_at": "2026-05-24T13:00:00Z", "photo_path": "users/friend/memories/a.jpg", "owner": map[string]any{"display_name": "Friend"}},
			{"id": "no-photo", "owner_user_id": friendUserID, "happened_at": "2026-05-24T14:00:00Z", "owner": map[string]any{"display_name": "Friend"}},
		},
		officialMemories: []map[string]any{{"id": "official", "owner_user_id": friendUserID, "happened_at": "2026-05-25T10:00:00Z", "is_official": true}},
	}
	usecase := NewUsecase(Dependencies{Repository: repo})

	items, err := usecase.ListHomeFeed(context.Background(), ListInput{AuthToken: testAuthToken, UserID: testUserID})
	if err != nil {
		t.Fatalf("ListHomeFeed returned error: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("items = %#v, want 2 displayable non-hidden rows", items)
	}
	if items[0]["id"] != "official" {
		t.Fatalf("items sorted = %#v", items)
	}
	feed, ok := items[1]["feed_item"].(FeedItem)
	if !ok {
		t.Fatalf("feed_item = %#v", items[1]["feed_item"])
	}
	if feed.PostKind != "mine" || !feed.OwnedByMe || feed.AuthorName != "Me" || feed.Body != "hello" || feed.Place != "bar" {
		t.Fatalf("feed item = %#v", feed)
	}
	if feed.LikeCount != 1 || !feed.LikedByMe || !feed.CanDelete || feed.CanReport {
		t.Fatalf("feed actions = %#v", feed)
	}
}

func TestListHomeFeedRejectsInvalidUserID(t *testing.T) {
	usecase := NewUsecase(Dependencies{Repository: &fakeRepository{}})

	_, err := usecase.ListHomeFeed(context.Background(), ListInput{AuthToken: testAuthToken, UserID: "bad"})
	assertUserError(t, err, ErrorKindInvalidInput, "user id must be a valid UUID")
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
