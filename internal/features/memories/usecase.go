package memories

import (
	"context"
	"strings"
	"time"
)

type Dependencies struct {
	Repository Repository
	Publisher  EventPublisher
	Now        func() time.Time
}

type Usecase struct {
	repository Repository
	publisher  EventPublisher
	now        func() time.Time
}

func NewUsecase(deps Dependencies) *Usecase {
	now := deps.Now
	if now == nil {
		now = time.Now
	}
	return &Usecase{repository: deps.Repository, publisher: deps.Publisher, now: now}
}

type ListInput struct {
	AuthToken string
	UserID    string
}

type CreateInput struct {
	AuthToken             string
	OwnerUserID           string
	HappenedAt            *time.Time
	HappenedOn            string
	TimezoneOffsetMinutes *int
	PlaceName             string
	PlaceLat              *float64
	PlaceLng              *float64
	Memo                  string
	FriendIDs             []string
}

type DeleteInput struct {
	AuthToken   string
	MemoryID    string
	OwnerUserID string
}

type ReportInput struct {
	AuthToken      string
	MemoryID       string
	ReporterUserID string
	Reason         string
}

func (u *Usecase) ListMemories(ctx context.Context, input ListInput) ([]map[string]any, error) {
	userID, err := CleanUUID(input.UserID, "user id")
	if err != nil {
		return nil, err
	}
	visibleUserIDs, err := u.repository.VisibleFeedUserIDs(ctx, input.AuthToken, userID)
	if err != nil {
		return nil, err
	}
	hiddenUserIDs, err := u.repository.HiddenUserIDs(ctx, input.AuthToken, userID)
	if err != nil {
		return nil, err
	}
	visibleUserIDs = ExcludeHiddenUserIDs(visibleUserIDs, hiddenUserIDs)
	rows, err := u.repository.ListMemories(ctx, input.AuthToken, visibleUserIDs)
	if err != nil {
		return nil, err
	}
	officialRows, err := u.repository.ListOfficialMemories(ctx, input.AuthToken)
	if err != nil {
		return nil, err
	}
	hiddenIDs, err := u.repository.HiddenMemoryIDs(ctx, input.AuthToken, userID)
	if err != nil {
		return nil, err
	}
	rows = AppendUniqueRows(rows, officialRows...)
	rows = HideRowsByID(rows, hiddenIDs)
	rows = HideRowsByOwner(rows, hiddenUserIDs)
	SortRowsByHappenedAtDesc(rows)
	return rows, nil
}

func (u *Usecase) CreateMemory(ctx context.Context, input CreateInput) (map[string]any, error) {
	ownerUserID, err := CleanUUID(input.OwnerUserID, "owner user id")
	if err != nil {
		return nil, err
	}
	friendIDs, err := CleanUUIDs(input.FriendIDs, "friend id")
	if err != nil {
		return nil, err
	}
	for _, friendID := range friendIDs {
		if friendID == ownerUserID {
			return nil, UserError{Kind: ErrorKindForbidden, Message: "friend_ids must be existing friends"}
		}
		ok, err := u.repository.FriendshipExists(ctx, input.AuthToken, ownerUserID, friendID)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, UserError{Kind: ErrorKindForbidden, Message: "friend_ids must be existing friends"}
		}
	}

	happenedAt := u.now()
	if input.HappenedAt != nil {
		happenedAt = *input.HappenedAt
	}
	start, end, err := MemoryDayWindow(DayWindowInput{
		HappenedOn:            input.HappenedOn,
		TimezoneOffsetMinutes: input.TimezoneOffsetMinutes,
	}, happenedAt)
	if err != nil {
		return nil, err
	}
	exists, err := u.repository.HasMemoryInWindow(ctx, input.AuthToken, ownerUserID, start, end)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, UserError{Kind: ErrorKindConflict, Message: "投稿は1日1つまでです"}
	}
	row, err := u.repository.CreateMemory(ctx, input.AuthToken, NewMemory{
		OwnerUserID: ownerUserID,
		HappenedAt:  happenedAt,
		PlaceName:   strings.TrimSpace(input.PlaceName),
		PlaceLat:    input.PlaceLat,
		PlaceLng:    input.PlaceLng,
		Memo:        strings.TrimSpace(input.Memo),
		IsOfficial:  false,
	})
	if err != nil {
		return nil, err
	}
	memoryID, _ := row["id"].(string)
	if memoryID == "" {
		return nil, UserError{Kind: ErrorKindUpstream, Message: "memory insert returned no id"}
	}
	if err := u.repository.CreateMemoryFriendLinks(ctx, input.AuthToken, memoryID, friendIDs); err != nil {
		return nil, err
	}
	if u.publisher != nil {
		if event, ok := NewMemoryTaggedEvent(memoryID, ownerUserID, friendIDs); ok {
			u.publisher.Publish(ctx, input.AuthToken, event)
		}
	}
	return row, nil
}

func (u *Usecase) DeleteMemory(ctx context.Context, input DeleteInput) (map[string]any, error) {
	memoryID, err := CleanUUID(input.MemoryID, "memory id")
	if err != nil {
		return nil, err
	}
	ownerUserID, err := CleanUUID(input.OwnerUserID, "owner user id")
	if err != nil {
		return nil, err
	}
	row, err := u.repository.DeleteOwnedMemory(ctx, input.AuthToken, memoryID, ownerUserID)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, UserError{Kind: ErrorKindNotFound, Message: "memory not found"}
	}
	return row, nil
}

func (u *Usecase) ReportMemory(ctx context.Context, input ReportInput) (ReportResult, error) {
	memoryID, err := CleanUUID(input.MemoryID, "memory id")
	if err != nil {
		return ReportResult{}, err
	}
	reporterUserID, err := CleanUUID(input.ReporterUserID, "reporter user id")
	if err != nil {
		return ReportResult{}, err
	}
	reason, err := CleanReportReason(input.Reason)
	if err != nil {
		return ReportResult{}, err
	}
	ownerUserID, err := u.repository.MemoryOwnerUserID(ctx, input.AuthToken, memoryID)
	if err != nil {
		return ReportResult{}, err
	}
	if ownerUserID == "" {
		return ReportResult{}, UserError{Kind: ErrorKindNotFound, Message: "memory not found"}
	}
	if ownerUserID == reporterUserID {
		return ReportResult{}, UserError{Kind: ErrorKindForbidden, Message: "cannot report your own memory"}
	}
	existing, err := u.repository.FindReport(ctx, input.AuthToken, memoryID, reporterUserID)
	if err != nil {
		return ReportResult{}, err
	}
	if existing != nil {
		return ReportResult{Created: false, Body: NewReportBody(*existing, true)}, nil
	}
	report := Report{MemoryID: memoryID, ReporterUserID: reporterUserID, Reason: reason, Status: ModerationStatusPending}
	if err := u.repository.CreateReport(ctx, input.AuthToken, report); err != nil {
		return ReportResult{}, err
	}
	if u.publisher != nil {
		if event, ok := NewMemoryReportedEvent(memoryID, ownerUserID, reporterUserID, reason); ok {
			u.publisher.Publish(ctx, input.AuthToken, event)
		}
	}
	return ReportResult{Created: true, Body: NewReportBody(report, false)}, nil
}
