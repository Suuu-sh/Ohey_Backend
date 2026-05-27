package drinklogs

import (
	"context"
	"strings"
	"time"
)

type Dependencies struct {
	Repository  Repository
	Notifier    Notifier
	Now         func() time.Time
	RandomFloat func() float64
}

type Usecase struct {
	repository  Repository
	notifier    Notifier
	now         func() time.Time
	randomFloat func() float64
}

func NewUsecase(deps Dependencies) *Usecase {
	now := deps.Now
	if now == nil {
		now = time.Now
	}
	randomFloat := deps.RandomFloat
	if randomFloat == nil {
		randomFloat = RandomFloat64
	}
	return &Usecase{repository: deps.Repository, notifier: deps.Notifier, now: now, randomFloat: randomFloat}
}

type ListInput struct {
	AuthToken string
	UserID    string
}

type CreateInput struct {
	AuthToken             string
	OwnerUserID           string
	DrankAt               *time.Time
	DrankOn               string
	TimezoneOffsetMinutes *int
	PlaceName             string
	PlaceLat              *float64
	PlaceLng              *float64
	Memo                  string
	CaptionY              *float64
	PhotoPath             string
	FriendIDs             []string
	ClientRequestedRarity string
}

type DeleteInput struct {
	AuthToken   string
	LogID       string
	OwnerUserID string
}

type LikeInput struct {
	AuthToken string
	LogID     string
	UserID    string
}

type ReportInput struct {
	AuthToken      string
	LogID          string
	ReporterUserID string
	Reason         string
}

func (u *Usecase) ListDrinkLogs(ctx context.Context, input ListInput) ([]map[string]any, error) {
	userID, err := CleanUUID(input.UserID, "user id")
	if err != nil {
		return nil, err
	}
	visibleUserIDs, err := u.repository.VisibleFeedUserIDs(ctx, input.AuthToken, userID)
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
	hiddenIDs, err := u.repository.HiddenDrinkLogIDs(ctx, input.AuthToken, userID)
	if err != nil {
		return nil, err
	}
	rows = AppendUniqueRows(rows, officialRows...)
	rows = HideRowsByID(rows, hiddenIDs)
	AttachLikeState(rows, userID)
	SortRowsByDrankAtDesc(rows)
	return rows, nil
}

func (u *Usecase) CreateDrinkLog(ctx context.Context, input CreateInput) (map[string]any, error) {
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

	drankAt := u.now()
	if input.DrankAt != nil {
		drankAt = *input.DrankAt
	}
	start, end, err := DrinkLogDayWindow(DayWindowInput{
		DrankOn:               input.DrankOn,
		TimezoneOffsetMinutes: input.TimezoneOffsetMinutes,
	}, drankAt)
	if err != nil {
		return nil, err
	}
	exists, err := u.repository.HasDrinkLogInWindow(ctx, input.AuthToken, ownerUserID, start, end)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, UserError{Kind: ErrorKindConflict, Message: "投稿は1日1回までです"}
	}
	photoPath, err := CleanUserPhotoPath(ownerUserID, input.PhotoPath)
	if err != nil {
		return nil, err
	}
	row, err := u.repository.CreateDrinkLog(ctx, input.AuthToken, NewDrinkLog{
		OwnerUserID:  ownerUserID,
		DrankAt:      drankAt,
		PlaceName:    strings.TrimSpace(input.PlaceName),
		PlaceLat:     input.PlaceLat,
		PlaceLng:     input.PlaceLng,
		Memo:         strings.TrimSpace(input.Memo),
		CaptionY:     CleanCaptionY(input.CaptionY),
		PhotoPath:    photoPath,
		MarkerRarity: MarkerRarityForPhotoPath(photoPath, u.randomFloat),
		IsOfficial:   false,
	})
	if err != nil {
		return nil, err
	}
	logID, _ := row["id"].(string)
	if logID == "" {
		return nil, UserError{Kind: ErrorKindUpstream, Message: "drink log insert returned no id"}
	}
	if err := u.repository.CreateDrinkLogFriendLinks(ctx, input.AuthToken, logID, friendIDs); err != nil {
		return nil, err
	}
	if u.notifier != nil {
		u.notifier.DrinkLogTagged(ctx, input.AuthToken, logID, ownerUserID, friendIDs)
	}
	return row, nil
}

func (u *Usecase) DeleteDrinkLog(ctx context.Context, input DeleteInput) (map[string]any, error) {
	logID, err := CleanUUID(input.LogID, "drink log id")
	if err != nil {
		return nil, err
	}
	ownerUserID, err := CleanUUID(input.OwnerUserID, "owner user id")
	if err != nil {
		return nil, err
	}
	row, err := u.repository.DeleteOwnedDrinkLog(ctx, input.AuthToken, logID, ownerUserID)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, UserError{Kind: ErrorKindNotFound, Message: "drink log not found"}
	}
	return row, nil
}

func (u *Usecase) LikeDrinkLog(ctx context.Context, input LikeInput) (LikeState, error) {
	logID, userID, err := cleanLikeInput(input)
	if err != nil {
		return LikeState{}, err
	}
	created, err := u.repository.CreateLike(ctx, input.AuthToken, logID, userID)
	if err != nil {
		return LikeState{}, err
	}
	if created && u.notifier != nil {
		u.notifier.DrinkLogLiked(ctx, input.AuthToken, logID, userID)
	}
	return u.repository.LikeState(ctx, input.AuthToken, logID, userID)
}

func (u *Usecase) UnlikeDrinkLog(ctx context.Context, input LikeInput) (LikeState, error) {
	logID, userID, err := cleanLikeInput(input)
	if err != nil {
		return LikeState{}, err
	}
	if err := u.repository.DeleteLike(ctx, input.AuthToken, logID, userID); err != nil {
		return LikeState{}, err
	}
	return u.repository.LikeState(ctx, input.AuthToken, logID, userID)
}

func (u *Usecase) ReportDrinkLog(ctx context.Context, input ReportInput) (ReportResult, error) {
	logID, err := CleanUUID(input.LogID, "drink log id")
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
	ownerUserID, err := u.repository.DrinkLogOwnerUserID(ctx, input.AuthToken, logID)
	if err != nil {
		return ReportResult{}, err
	}
	if ownerUserID == "" {
		return ReportResult{}, UserError{Kind: ErrorKindNotFound, Message: "drink log not found"}
	}
	if ownerUserID == reporterUserID {
		return ReportResult{}, UserError{Kind: ErrorKindForbidden, Message: "cannot report your own drink log"}
	}
	existing, err := u.repository.FindReport(ctx, input.AuthToken, logID, reporterUserID)
	if err != nil {
		return ReportResult{}, err
	}
	if existing != nil {
		return ReportResult{Created: false, Body: NewReportBody(*existing, true)}, nil
	}
	report := Report{DrinkLogID: logID, ReporterUserID: reporterUserID, Reason: reason, Status: ModerationStatusPending}
	if err := u.repository.CreateReport(ctx, input.AuthToken, report); err != nil {
		return ReportResult{}, err
	}
	return ReportResult{Created: true, Body: NewReportBody(report, false)}, nil
}

func cleanLikeInput(input LikeInput) (string, string, error) {
	logID, err := CleanUUID(input.LogID, "drink log id")
	if err != nil {
		return "", "", err
	}
	userID, err := CleanUUID(input.UserID, "user id")
	if err != nil {
		return "", "", err
	}
	return logID, userID, nil
}
