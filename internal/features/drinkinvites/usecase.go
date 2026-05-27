package drinkinvites

import (
	"context"
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
	AuthToken  string
	UserID     string
	InviteDate string
}

type CreateInput struct {
	AuthToken  string
	FromUserID string
	ToUserID   string
	InviteDate string
}

type UpdateInput struct {
	AuthToken       string
	InviteID        string
	RecipientUserID string
	Status          string
}

func (u *Usecase) ListTodayReservations(ctx context.Context, input ListInput) ([]map[string]any, error) {
	return u.repository.ListTodayReservations(ctx, input.AuthToken, input.UserID, input.InviteDate)
}

func (u *Usecase) ListIncomingPending(ctx context.Context, input ListInput) ([]map[string]any, error) {
	return u.repository.ListIncomingPending(ctx, input.AuthToken, input.UserID, input.InviteDate)
}

func (u *Usecase) ListOutgoingActive(ctx context.Context, input ListInput) ([]map[string]any, error) {
	return u.repository.ListOutgoingActive(ctx, input.AuthToken, input.UserID, input.InviteDate)
}

func (u *Usecase) CreateDrinkInvite(ctx context.Context, input CreateInput) (map[string]any, error) {
	fromUserID, err := CleanUUID(input.FromUserID, "from_user_id")
	if err != nil {
		return nil, err
	}
	toUserID, err := CleanUUID(input.ToUserID, "to_user_id")
	if err != nil {
		return nil, err
	}
	if err := ValidateNewInvite(fromUserID, toUserID); err != nil {
		return nil, err
	}
	inviteDate, err := CleanDateOnlyOrToday(input.InviteDate, "invite_date", u.now())
	if err != nil {
		return nil, err
	}

	dailyStatus, err := u.repository.DailyStatus(ctx, input.AuthToken, toUserID, inviteDate)
	if err != nil {
		return nil, err
	}
	if message := BlockedDailyStatusMessage(dailyStatus); message != "" {
		return nil, UserError{Kind: ErrorKindConflict, Message: message}
	}

	existing, err := u.repository.FindActiveInviteBetweenUsersForDate(ctx, input.AuthToken, fromUserID, toUserID, inviteDate)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, UserError{Kind: ErrorKindConflict, Message: ExistingInviteConflictMessage(existing.Status)}
	}

	row, err := u.repository.CreateInvite(ctx, input.AuthToken, NewInvite{
		FromUserID: fromUserID,
		ToUserID:   toUserID,
		InviteDate: inviteDate,
	})
	if err != nil {
		return nil, err
	}
	if u.publisher != nil {
		if event, ok := NewDrinkInviteCreatedEvent(row); ok {
			u.publisher.Publish(ctx, input.AuthToken, event)
		}
	}
	return row, nil
}

func (u *Usecase) UpdateDrinkInvite(ctx context.Context, input UpdateInput) (map[string]any, error) {
	inviteID, err := CleanUUID(input.InviteID, "drink invite id")
	if err != nil {
		return nil, err
	}
	recipientUserID, err := CleanUUID(input.RecipientUserID, "recipient user id")
	if err != nil {
		return nil, err
	}
	status, err := NormalizeResponseStatus(input.Status)
	if err != nil {
		return nil, err
	}

	row, err := u.repository.UpdatePendingInviteStatus(ctx, input.AuthToken, inviteID, recipientUserID, status, u.now().UTC())
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, UserError{Kind: ErrorKindNotFound, Message: "drink invite not found"}
	}
	if status == InviteStatusAccepted && u.publisher != nil {
		if event, ok := NewDrinkInviteAcceptedEvent(row); ok {
			u.publisher.Publish(ctx, input.AuthToken, event)
		}
	}
	return row, nil
}
