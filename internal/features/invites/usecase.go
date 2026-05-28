package invites

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
	AuthToken     string
	UserID        string
	ScheduledDate string
}

type CreateInput struct {
	AuthToken     string
	InviterUserID string
	InviteeUserID string
	ScheduledDate string
}

type UpdateInput struct {
	AuthToken       string
	InviteID        string
	RecipientUserID string
	Status          string
}

func (u *Usecase) ListTodayReservations(ctx context.Context, input ListInput) ([]map[string]any, error) {
	return u.repository.ListTodayReservations(ctx, input.AuthToken, input.UserID, input.ScheduledDate)
}

func (u *Usecase) ListIncomingPending(ctx context.Context, input ListInput) ([]map[string]any, error) {
	return u.repository.ListIncomingPending(ctx, input.AuthToken, input.UserID, input.ScheduledDate)
}

func (u *Usecase) ListOutgoingActive(ctx context.Context, input ListInput) ([]map[string]any, error) {
	return u.repository.ListOutgoingActive(ctx, input.AuthToken, input.UserID, input.ScheduledDate)
}

func (u *Usecase) CreateInvite(ctx context.Context, input CreateInput) (map[string]any, error) {
	inviterUserID, err := CleanUUID(input.InviterUserID, "inviter_user_id")
	if err != nil {
		return nil, err
	}
	inviteeUserID, err := CleanUUID(input.InviteeUserID, "invitee_user_id")
	if err != nil {
		return nil, err
	}
	if err := ValidateNewInvite(inviterUserID, inviteeUserID); err != nil {
		return nil, err
	}
	scheduledDate, err := CleanDateOnlyOrToday(input.ScheduledDate, "scheduled_date", u.now())
	if err != nil {
		return nil, err
	}
	blocked, err := u.repository.BlockExistsBetweenUsers(ctx, input.AuthToken, inviterUserID, inviteeUserID)
	if err != nil {
		return nil, err
	}
	if blocked {
		return nil, UserError{Kind: ErrorKindConflict, Message: "blocked users cannot be invited"}
	}

	dailyStatus, err := u.repository.DailyStatus(ctx, input.AuthToken, inviteeUserID, scheduledDate)
	if err != nil {
		return nil, err
	}
	if message := BlockedDailyStatusMessage(dailyStatus); message != "" {
		return nil, UserError{Kind: ErrorKindConflict, Message: message}
	}

	existing, err := u.repository.FindActiveInviteBetweenUsersForDate(ctx, input.AuthToken, inviterUserID, inviteeUserID, scheduledDate)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, UserError{Kind: ErrorKindConflict, Message: ExistingInviteConflictMessage(existing.Status)}
	}

	row, err := u.repository.CreateInvite(ctx, input.AuthToken, NewInvite{
		InviterUserID: inviterUserID,
		InviteeUserID: inviteeUserID,
		ScheduledDate: scheduledDate,
	})
	if err != nil {
		return nil, err
	}
	if u.publisher != nil {
		if event, ok := NewInviteCreatedEvent(row); ok {
			u.publisher.Publish(ctx, input.AuthToken, event)
		}
	}
	return row, nil
}

func (u *Usecase) UpdateInvite(ctx context.Context, input UpdateInput) (map[string]any, error) {
	inviteID, err := CleanUUID(input.InviteID, "invite id")
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
		return nil, UserError{Kind: ErrorKindNotFound, Message: "invite not found"}
	}
	if status == InviteStatusAccepted && u.publisher != nil {
		if event, ok := NewInviteAcceptedEvent(row); ok {
			u.publisher.Publish(ctx, input.AuthToken, event)
		}
	}
	return row, nil
}
