package homefeed

import (
	"context"
	"sort"
	"time"
)

type Dependencies struct {
	Repository Repository
	Now        func() time.Time
}

type Usecase struct {
	repository Repository
	now        func() time.Time
}

func NewUsecase(deps Dependencies) *Usecase {
	now := deps.Now
	if now == nil {
		now = time.Now
	}
	return &Usecase{repository: deps.Repository, now: now}
}

type ListInput struct {
	AuthToken string
	UserID    string
}

func (u *Usecase) ListHomeFeed(ctx context.Context, input ListInput) ([]map[string]any, error) {
	userID, err := CleanUUID(input.UserID, "user id")
	if err != nil {
		return nil, err
	}
	visibleUserIDs, err := u.repository.VisibleFeedUserIDs(ctx, input.AuthToken, userID)
	if err != nil {
		return nil, err
	}
	hiddenIDs, err := u.repository.HiddenDrinkLogIDs(ctx, input.AuthToken, userID)
	if err != nil {
		return nil, err
	}
	rows, err := u.repository.ListDrinkLogs(ctx, input.AuthToken, visibleUserIDs)
	if err != nil {
		return nil, err
	}
	officialRows, err := u.repository.ListOfficialDrinkLogs(ctx, input.AuthToken)
	if err != nil {
		return nil, err
	}
	rows = appendUniqueRows(rows, officialRows...)
	attachLikeState(rows, userID)
	rows = HideReportedRows(rows, hiddenIDs)
	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		item, ok := BuildFeedItem(row, userID)
		if !ok {
			continue
		}
		items = append(items, AttachFeedItem(row, item))
	}
	sort.SliceStable(items, func(i, j int) bool {
		return RowTime(items[i]).After(RowTime(items[j]))
	})
	return items, nil
}

func appendUniqueRows(rows []map[string]any, extraRows ...map[string]any) []map[string]any {
	seen := make(map[string]bool, len(rows)+len(extraRows))
	for _, row := range rows {
		if id, _ := row["id"].(string); id != "" {
			seen[id] = true
		}
	}
	for _, row := range extraRows {
		id, _ := row["id"].(string)
		if id != "" && seen[id] {
			continue
		}
		if id != "" {
			seen[id] = true
		}
		rows = append(rows, row)
	}
	return rows
}

func attachLikeState(rows []map[string]any, userID string) {
	for _, row := range rows {
		rawLikes, _ := row["drink_log_likes"].([]any)
		row["like_count"] = len(rawLikes)
		likedByMe := false
		for _, rawLike := range rawLikes {
			like, ok := rawLike.(map[string]any)
			if ok && like["user_id"] == userID {
				likedByMe = true
				break
			}
		}
		row["liked_by_me"] = likedByMe
	}
}
