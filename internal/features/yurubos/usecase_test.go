package yurubos

import (
	"context"
	"testing"

	"github.com/Suuu-sh/Ohey_Backend/internal/contracts"
)

const (
	ownerID       = "11111111-1111-1111-1111-111111111111"
	yuruboID      = "22222222-2222-2222-2222-222222222222"
	wishItemID    = "33333333-3333-3333-3333-333333333333"
	groupID       = "44444444-4444-4444-4444-444444444444"
	participantID = "55555555-5555-5555-5555-555555555555"
	otherUserID   = "66666666-6666-6666-6666-666666666666"
)

type fakeRepository struct {
	wishItemExists bool
	created        Yurubo
	linkedYuruboID string
	linkedGroupID  string
	updated        YuruboUpdate
	deletedYurubo  string
	deletedUser    string
	hidden         map[string]bool
	openRows       []map[string]any
	reactions      []map[string]any
	profiles       map[string]map[string]any
	ownerIDs       map[string]string
	ownerIDCalls   int
	labels         map[string]string
	upserted       Reaction
	approved       bool
}

func (r *fakeRepository) WishItemExists(_ context.Context, _ string, _ string, _ string) (bool, error) {
	return r.wishItemExists, nil
}

func (r *fakeRepository) CreateYurubo(_ context.Context, _ string, item Yurubo) (map[string]any, error) {
	r.created = item
	return map[string]any{"id": yuruboID, "title": item.Title}, nil
}

func (r *fakeRepository) LinkVisibilityGroup(_ context.Context, _ string, yuruboID, groupID string) error {
	r.linkedYuruboID = yuruboID
	r.linkedGroupID = groupID
	return nil
}

func (r *fakeRepository) UpdateYurubo(_ context.Context, _ string, update YuruboUpdate) (map[string]any, error) {
	r.updated = update
	return map[string]any{"id": update.YuruboID, "title": update.Title}, nil
}

func (r *fakeRepository) DeleteYurubo(_ context.Context, _ string, yuruboID, ownerUserID string) (map[string]any, error) {
	r.deletedYurubo = yuruboID
	r.deletedUser = ownerUserID
	return map[string]any{"id": yuruboID}, nil
}

func (r *fakeRepository) HiddenYuruboIDs(_ context.Context, _ string, _ string) (map[string]bool, error) {
	if r.hidden == nil {
		return map[string]bool{}, nil
	}
	return r.hidden, nil
}

func (r *fakeRepository) ListOpenYurubos(_ context.Context, _ string, _ int) ([]map[string]any, error) {
	return r.openRows, nil
}

func (r *fakeRepository) ListReactions(_ context.Context, _ string, _ []string) ([]map[string]any, error) {
	return r.reactions, nil
}

func (r *fakeRepository) ParticipantProfiles(_ context.Context, _ string, _ []string) (map[string]map[string]any, error) {
	if r.profiles == nil {
		return map[string]map[string]any{}, nil
	}
	return r.profiles, nil
}

func (r *fakeRepository) OwnerID(_ context.Context, _ string, yuruboID string) (string, error) {
	r.ownerIDCalls++
	return r.ownerIDs[yuruboID], nil
}

func (r *fakeRepository) VisibilityLabels(_ context.Context, _ string, _ []map[string]any) (map[string]string, error) {
	if r.labels == nil {
		return map[string]string{}, nil
	}
	return r.labels, nil
}

func (r *fakeRepository) UpsertReaction(_ context.Context, _ string, reaction Reaction) error {
	r.upserted = reaction
	return nil
}

func (r *fakeRepository) ApproveReaction(_ context.Context, _ string, _ string, _ string, _ string) (bool, error) {
	return r.approved, nil
}

func (r *fakeRepository) DeleteReaction(_ context.Context, _ string, yuruboID, userID string) error {
	r.deletedYurubo = yuruboID
	r.deletedUser = userID
	return nil
}

func TestCreateYuruboNormalizesAndLinksGroup(t *testing.T) {
	repo := &fakeRepository{wishItemExists: true}
	usecase := NewUsecase(Dependencies{Repository: repo})

	row, err := usecase.CreateYurubo(context.Background(), CreateInput{
		AuthToken:   "token",
		OwnerUserID: ownerID,
		Body: CreateBody{
			Title:      "  ゆるぼタイトル  ",
			Body:       " 本文 ",
			PlaceText:  " 公園 ",
			TimeLabel:  " 午後 ",
			StartsAt:   "2026-06-03",
			Visibility: contracts.VisibilityGroup,
			GroupID:    groupID,
			WishItemID: wishItemID,
		},
	})
	if err != nil {
		t.Fatalf("CreateYurubo error = %v", err)
	}
	if row["id"] != yuruboID {
		t.Fatalf("row = %#v", row)
	}
	if repo.created.Title != "ゆるぼタイトル" || repo.created.Body != "本文" || repo.created.PlaceText != "公園" || repo.created.TimeLabel != "午後" {
		t.Fatalf("created = %#v", repo.created)
	}
	if repo.created.Category != contracts.CategoryOther {
		t.Fatalf("category = %q", repo.created.Category)
	}
	if repo.created.StartsAt == nil || *repo.created.StartsAt != "2026-06-03T00:00:00Z" {
		t.Fatalf("startsAt = %v", repo.created.StartsAt)
	}
	if repo.created.WishItemID == nil || *repo.created.WishItemID != wishItemID {
		t.Fatalf("wish item = %v", repo.created.WishItemID)
	}
	if repo.linkedYuruboID != yuruboID || repo.linkedGroupID != groupID {
		t.Fatalf("link = %q %q", repo.linkedYuruboID, repo.linkedGroupID)
	}
}

func TestCreateYuruboRejectsMissingWishItem(t *testing.T) {
	usecase := NewUsecase(Dependencies{Repository: &fakeRepository{wishItemExists: false}})

	_, err := usecase.CreateYurubo(context.Background(), CreateInput{
		OwnerUserID: ownerID,
		Body:        CreateBody{Title: "title", WishItemID: wishItemID},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	kind, ok := ErrorKindOf(err)
	if !ok || kind != ErrorKindInvalidInput {
		t.Fatalf("kind = %q ok=%v err=%v", kind, ok, err)
	}
}

func TestListYurubosAttachesReactionAndVisibilitySummaries(t *testing.T) {
	hiddenID := "77777777-7777-7777-7777-777777777777"
	repo := &fakeRepository{
		hidden: map[string]bool{hiddenID: true},
		openRows: []map[string]any{
			{"id": yuruboID, "owner_user_id": ownerID, "visibility": contracts.VisibilityGroup},
			{"id": hiddenID, "visibility": contracts.VisibilityFriends},
		},
		reactions: []map[string]any{
			{"yurubo_id": yuruboID, "user_id": participantID, "reaction_type": contracts.ReactionTypeAvailable},
			{"yurubo_id": yuruboID, "user_id": otherUserID, "reaction_type": contracts.ReactionTypeInterested},
		},
		profiles: map[string]map[string]any{
			participantID: {"id": participantID, "display_name": "参加者"},
			otherUserID:   {"id": otherUserID, "display_name": "検討中"},
		},
		ownerIDs: map[string]string{yuruboID: ownerID},
		labels:   map[string]string{yuruboID: "仲良し"},
	}
	usecase := NewUsecase(Dependencies{Repository: repo})

	rows, err := usecase.ListYurubos(context.Background(), ListInput{UserID: ownerID})
	if err != nil {
		t.Fatalf("ListYurubos error = %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %#v", rows)
	}
	row := rows[0]
	if row["reaction_count"] != 1 || row["reacted_by_me"] != false || row["visibility_label"] != "仲良し" {
		t.Fatalf("summary row = %#v", row)
	}
	participants, ok := row["participants"].([]map[string]any)
	if !ok || len(participants) != 2 {
		t.Fatalf("participants = %#v", row["participants"])
	}
	if participants[0]["reaction_type"] != contracts.ReactionTypeAvailable || participants[1]["reaction_type"] != contracts.ReactionTypeInterested {
		t.Fatalf("participants = %#v", participants)
	}
	if repo.ownerIDCalls != 0 {
		t.Fatalf("OwnerID calls = %d, want 0", repo.ownerIDCalls)
	}
}

func TestReactYuruboDefaultsReactionType(t *testing.T) {
	repo := &fakeRepository{}
	usecase := NewUsecase(Dependencies{Repository: repo})

	state, err := usecase.ReactYurubo(context.Background(), ReactionInput{YuruboID: yuruboID, UserID: ownerID})
	if err != nil {
		t.Fatalf("ReactYurubo error = %v", err)
	}
	if !state.ReactedByMe {
		t.Fatalf("state = %#v", state)
	}
	if repo.upserted.ReactionType != contracts.ReactionTypeInterested {
		t.Fatalf("reaction = %#v", repo.upserted)
	}
}

func TestApproveReactionRequiresOwner(t *testing.T) {
	repo := &fakeRepository{ownerIDs: map[string]string{yuruboID: otherUserID}, approved: true}
	usecase := NewUsecase(Dependencies{Repository: repo})

	_, err := usecase.ApproveReaction(context.Background(), ApprovalInput{YuruboID: yuruboID, OwnerUserID: ownerID, ParticipantID: participantID})
	if err == nil {
		t.Fatal("expected error")
	}
	kind, ok := ErrorKindOf(err)
	if !ok || kind != ErrorKindForbidden {
		t.Fatalf("kind = %q ok=%v err=%v", kind, ok, err)
	}
}
