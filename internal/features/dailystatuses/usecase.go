package dailystatuses

import (
	"context"
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

type GetInput struct {
	AuthToken string
	UserID    string
	Date      string
}

type MonthInput struct {
	AuthToken string
	UserID    string
	Month     string
}

type FriendMonthInput struct {
	AuthToken string
	UserID    string
	FriendID  string
	Month     string
}

type UpsertInput struct {
	AuthToken  string
	UserID     string
	StatusDate string
	Status     string
}

func (u *Usecase) GetDailyStatus(ctx context.Context, input GetInput) ([]map[string]any, error) {
	userID, err := CleanUUID(input.UserID, "user id")
	if err != nil {
		return nil, err
	}
	statusDate, err := CleanDateOnlyOrToday(input.Date, "date", u.now())
	if err != nil {
		return nil, err
	}
	return u.repository.GetDailyStatus(ctx, input.AuthToken, userID, statusDate)
}

func (u *Usecase) ListMonthlyStatuses(ctx context.Context, input MonthInput) ([]map[string]any, error) {
	userID, err := CleanUUID(input.UserID, "user id")
	if err != nil {
		return nil, err
	}
	monthRange, err := CleanMonth(input.Month, u.now())
	if err != nil {
		return nil, err
	}
	return u.repository.ListMonthlyStatuses(ctx, input.AuthToken, userID, monthRange.StartDate, monthRange.EndDate)
}

func (u *Usecase) ListFriendMonthlyStatuses(ctx context.Context, input FriendMonthInput) ([]map[string]any, error) {
	userID, err := CleanUUID(input.UserID, "user id")
	if err != nil {
		return nil, err
	}
	friendID, err := CleanUUID(input.FriendID, "friend id")
	if err != nil {
		return nil, err
	}
	monthRange, err := CleanMonth(input.Month, u.now())
	if err != nil {
		return nil, err
	}
	exists, err := u.repository.FriendshipExists(ctx, input.AuthToken, userID, friendID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, UserError{Kind: ErrorKindInvalidInput, Message: "friendship not found"}
	}
	return u.repository.ListMonthlyStatuses(ctx, input.AuthToken, friendID, monthRange.StartDate, monthRange.EndDate)
}

func (u *Usecase) UpsertDailyStatus(ctx context.Context, input UpsertInput) ([]map[string]any, error) {
	userID, err := CleanUUID(input.UserID, "user id")
	if err != nil {
		return nil, err
	}
	statusDate, err := CleanDateOnlyOrToday(input.StatusDate, "status_date", u.now())
	if err != nil {
		return nil, err
	}
	status, err := CleanStatus(input.Status)
	if err != nil {
		return nil, err
	}
	return u.repository.UpsertDailyStatus(ctx, input.AuthToken, DailyStatus{UserID: userID, StatusDate: statusDate, Status: status})
}
