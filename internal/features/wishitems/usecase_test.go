package wishitems

import (
	"context"
	"errors"
	"testing"

	"github.com/yota/ohey/backend/internal/contracts"
)

const testUserID = "11111111-1111-1111-1111-111111111111"

type fakeRepository struct {
	createdItem WishItem
	listLimit   int
	profileID   string
	createErr   error
}

func (r *fakeRepository) ListWishItems(_ context.Context, _ string, _ string, limit int) ([]map[string]any, error) {
	r.listLimit = limit
	return []map[string]any{{"id": "wish-1"}}, nil
}

func (r *fakeRepository) ListProfileWishItems(_ context.Context, _ string, profileID string, limit int) ([]map[string]any, error) {
	r.profileID = profileID
	r.listLimit = limit
	return []map[string]any{{"id": "wish-2"}}, nil
}

func (r *fakeRepository) CreateWishItem(_ context.Context, _ string, item WishItem) (map[string]any, error) {
	r.createdItem = item
	if r.createErr != nil {
		return nil, r.createErr
	}
	return map[string]any{"id": "wish-3", "title": item.Title}, nil
}

func TestCreateWishItemNormalizesDefaults(t *testing.T) {
	repo := &fakeRepository{}
	usecase := NewUsecase(Dependencies{Repository: repo})

	row, err := usecase.CreateWishItem(context.Background(), CreateInput{
		AuthToken:   "token",
		OwnerUserID: testUserID,
		Body: CreateBody{
			Title:     "  行きたい場所  ",
			Note:      " メモ ",
			PlaceText: " 公園 ",
			PlaceURL:  " https://example.com ",
		},
	})
	if err != nil {
		t.Fatalf("CreateWishItem error = %v", err)
	}
	if row["title"] != "行きたい場所" {
		t.Fatalf("row title = %v", row["title"])
	}
	if repo.createdItem.OwnerUserID != testUserID {
		t.Fatalf("owner = %q", repo.createdItem.OwnerUserID)
	}
	if repo.createdItem.Category != contracts.CategoryOther {
		t.Fatalf("category = %q", repo.createdItem.Category)
	}
	if repo.createdItem.Visibility != contracts.VisibilityPrivate {
		t.Fatalf("visibility = %q", repo.createdItem.Visibility)
	}
	if repo.createdItem.Note != "メモ" || repo.createdItem.PlaceText != "公園" || repo.createdItem.PlaceURL != "https://example.com" {
		t.Fatalf("trimmed item = %#v", repo.createdItem)
	}
}

func TestCreateWishItemRejectsInvalidVisibility(t *testing.T) {
	usecase := NewUsecase(Dependencies{Repository: &fakeRepository{}})

	_, err := usecase.CreateWishItem(context.Background(), CreateInput{
		OwnerUserID: testUserID,
		Body:        CreateBody{Title: "title", Visibility: "public"},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	kind, ok := ErrorKindOf(err)
	if !ok || kind != ErrorKindInvalidInput {
		t.Fatalf("kind = %q ok=%v err=%v", kind, ok, err)
	}
}

func TestListWishItemsCleansLimit(t *testing.T) {
	repo := &fakeRepository{}
	usecase := NewUsecase(Dependencies{Repository: repo})

	if _, err := usecase.ListWishItems(context.Background(), ListInput{UserID: testUserID, Limit: "25"}); err != nil {
		t.Fatalf("ListWishItems error = %v", err)
	}
	if repo.listLimit != 25 {
		t.Fatalf("limit = %d", repo.listLimit)
	}
	if _, err := usecase.ListWishItems(context.Background(), ListInput{UserID: testUserID, Limit: "999"}); err != nil {
		t.Fatalf("ListWishItems default error = %v", err)
	}
	if repo.listLimit != 50 {
		t.Fatalf("default limit = %d", repo.listLimit)
	}
}

func TestCreateWishItemPropagatesRepositoryErrors(t *testing.T) {
	repoErr := errors.New("repository failed")
	usecase := NewUsecase(Dependencies{Repository: &fakeRepository{createErr: repoErr}})

	_, err := usecase.CreateWishItem(context.Background(), CreateInput{OwnerUserID: testUserID, Body: CreateBody{Title: "title"}})
	if !errors.Is(err, repoErr) {
		t.Fatalf("err = %v", err)
	}
}
