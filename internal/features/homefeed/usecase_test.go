package homefeed

import (
	"context"
	"testing"
	"time"
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
	memoryLimit      int
	officialLimit    int
	memoryBefore     time.Time
	officialBefore   time.Time
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

func (f *fakeRepository) ListMemories(_ context.Context, _ string, _ []string, limit int, before time.Time) ([]map[string]any, error) {
	f.memoryLimit = limit
	f.memoryBefore = before
	return f.memories, nil
}

func (f *fakeRepository) ListOfficialMemories(_ context.Context, _ string, limit int, before time.Time) ([]map[string]any, error) {
	f.officialLimit = limit
	f.officialBefore = before
	return f.officialMemories, nil
}

func TestListHomeFeedShapesDisplayableItemsAndHidesReports(t *testing.T) {
	repo := &fakeRepository{
		hiddenIDs: map[string]bool{"hidden": true},
		memories: []map[string]any{
			{
				"id": "mine", "owner_user_id": testUserID, "happened_at": "2026-05-24T12:00:00Z",
				"memo": " hello ", "place_name": " spot ", "owner": map[string]any{"display_name": "Me"},
			},
			{"id": "hidden", "owner_user_id": friendUserID, "happened_at": "2026-05-24T13:00:00Z", "owner": map[string]any{"display_name": "Friend"}},
			{"id": "friend", "owner_user_id": friendUserID, "happened_at": "2026-05-24T14:00:00Z", "owner": map[string]any{"display_name": "Friend"}},
		},
		officialMemories: []map[string]any{{"id": "official", "owner_user_id": friendUserID, "happened_at": "2026-05-25T10:00:00Z", "is_official": true}},
	}
	usecase := NewUsecase(Dependencies{Repository: repo})

	items, err := usecase.ListHomeFeed(context.Background(), ListInput{AuthToken: testAuthToken, UserID: testUserID})
	if err != nil {
		t.Fatalf("ListHomeFeed returned error: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("items = %#v, want 3 displayable non-hidden rows", items)
	}
	if items[0]["id"] != "official" {
		t.Fatalf("items sorted = %#v", items)
	}
	var mine map[string]any
	for _, item := range items {
		if item["id"] == "mine" {
			mine = item
			break
		}
	}
	if mine == nil {
		t.Fatalf("items = %#v, want mine item", items)
	}
	feed, ok := mine["feed_item"].(FeedItem)
	if !ok {
		t.Fatalf("feed_item = %#v", mine["feed_item"])
	}
	if feed.PostKind != "mine" || !feed.OwnedByMe || feed.AuthorName != "Me" || feed.Body != "hello" || feed.Place != "spot" {
		t.Fatalf("feed item = %#v", feed)
	}
	if !feed.CanDelete || feed.CanReport {
		t.Fatalf("feed actions = %#v", feed)
	}
}

func TestListHomeFeedPassesBoundedFetchLimitAndCursorToRepository(t *testing.T) {
	repo := &fakeRepository{}
	usecase := NewUsecase(Dependencies{Repository: repo})

	_, err := usecase.ListHomeFeed(context.Background(), ListInput{
		AuthToken: testAuthToken,
		UserID:    testUserID,
		Limit:     "20",
		Cursor:    "1800000000:last-id",
	})
	if err != nil {
		t.Fatalf("ListHomeFeed returned error: %v", err)
	}
	if repo.memoryLimit != 100 || repo.officialLimit != 100 {
		t.Fatalf("fetch limits = %d/%d, want 100/100", repo.memoryLimit, repo.officialLimit)
	}
	wantBefore := time.Unix(1800000000, 0).UTC()
	if !repo.memoryBefore.Equal(wantBefore) || !repo.officialBefore.Equal(wantBefore) {
		t.Fatalf("before = %v/%v, want %v", repo.memoryBefore, repo.officialBefore, wantBefore)
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
