package notifications

import (
	"context"
	"errors"
	"strings"
	"time"
)

const defaultActorName = "フレンズ"

type Dependencies struct {
	Repository Repository
	PushSender PushSender
	Logger     Logger
	Now        func() time.Time
}

type Usecase struct {
	repository Repository
	pushSender PushSender
	logger     Logger
	now        func() time.Time
}

func NewUsecase(deps Dependencies) *Usecase {
	now := deps.Now
	if now == nil {
		now = time.Now
	}
	return &Usecase{repository: deps.Repository, pushSender: deps.PushSender, logger: deps.Logger, now: now}
}

type ListInput struct {
	AuthToken string
	UserID    string
	Date      string
}

type MarkReadInput struct {
	AuthToken string
	UserID    string
}

type CreateSystemInput struct {
	Title            string
	Message          string
	RecipientUserIDs []string
	SendToAll        bool
	SystemKey        string
}

type CreateSystemResult struct {
	RecipientCount int `json:"recipient_count"`
	CreatedCount   int `json:"created_count"`
}

func (u *Usecase) ListNotifications(ctx context.Context, input ListInput) ([]map[string]any, error) {
	userID, err := CleanUUID(input.UserID, "user id")
	if err != nil {
		return nil, err
	}
	if err := u.CreateTodayReservationReminders(ctx, input.AuthToken, userID, input.Date); err != nil {
		u.warn("failed to create today reservation reminder notifications", KindTodayReservationReminder, err)
	}
	return u.repository.ListNotifications(ctx, input.AuthToken, userID, 50)
}

func (u *Usecase) MarkAllRead(ctx context.Context, input MarkReadInput) (int, error) {
	userID, err := CleanUUID(input.UserID, "user id")
	if err != nil {
		return 0, err
	}
	return u.repository.MarkAllRead(ctx, input.AuthToken, userID, u.now().UTC())
}

func (u *Usecase) CreateSystemNotifications(ctx context.Context, input CreateSystemInput) (CreateSystemResult, error) {
	title := ShortText(input.Title, 80)
	message := ShortText(input.Message, 500)
	if title == "" {
		return CreateSystemResult{}, UserError{Kind: ErrorKindInvalidInput, Message: "title is required"}
	}
	if message == "" {
		return CreateSystemResult{}, UserError{Kind: ErrorKindInvalidInput, Message: "message is required"}
	}
	recipientIDs, err := u.systemRecipientIDs(ctx, input)
	if err != nil {
		return CreateSystemResult{}, err
	}
	if len(recipientIDs) == 0 {
		return CreateSystemResult{}, UserError{Kind: ErrorKindInvalidInput, Message: "recipient_user_ids or send_to_all is required"}
	}
	createdCount := 0
	for _, recipientID := range recipientIDs {
		created, err := u.repository.CreateNotification(ctx, Notification{
			RecipientUserID: recipientID,
			Kind:            KindSystem,
			Title:           title,
			Message:         message,
			SystemKey:       strings.TrimSpace(input.SystemKey),
		})
		if err != nil {
			return CreateSystemResult{}, err
		}
		if created {
			createdCount++
		}
	}
	return CreateSystemResult{RecipientCount: len(recipientIDs), CreatedCount: createdCount}, nil
}

func (u *Usecase) systemRecipientIDs(ctx context.Context, input CreateSystemInput) ([]string, error) {
	seen := map[string]bool{}
	ids := make([]string, 0, len(input.RecipientUserIDs))
	add := func(id string) {
		if id != "" && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	inputIDs, err := CleanUUIDs(input.RecipientUserIDs, "recipient user id")
	if err != nil {
		return nil, err
	}
	for _, id := range inputIDs {
		add(id)
	}
	if input.SendToAll {
		profileIDs, err := u.repository.AllProfileIDs(ctx)
		if err != nil {
			return nil, err
		}
		for _, id := range profileIDs {
			add(id)
		}
	}
	return ids, nil
}

func (u *Usecase) NotifyFriendRequestReceived(ctx context.Context, authToken string, requestRow map[string]any) error {
	requestID, inviterUserID, inviteeUserID := FriendRequestIDs(requestRow)
	if requestID == "" || inviterUserID == "" || inviteeUserID == "" || inviterUserID == inviteeUserID {
		return nil
	}
	actorName := u.actorName(ctx, authToken, inviterUserID, KindFriendRequestReceived)
	return u.tryCreateAndPush(ctx, authToken, Notification{
		RecipientUserID: inviteeUserID,
		ActorUserID:     inviterUserID,
		FriendRequestID: requestID,
		Kind:            KindFriendRequestReceived,
		Title:           "フレンズ申請が届きました",
		Message:         actorName + "さんからフレンズ申請が届きました。",
	})
}

func (u *Usecase) NotifyFriendRequestAccepted(ctx context.Context, authToken string, requestRow map[string]any) error {
	requestID, inviterUserID, inviteeUserID := FriendRequestIDs(requestRow)
	if requestID == "" || inviterUserID == "" || inviteeUserID == "" || inviterUserID == inviteeUserID {
		return nil
	}
	actorName := u.actorName(ctx, authToken, inviteeUserID, KindFriendRequestAccepted)
	return u.tryCreateAndPush(ctx, authToken, Notification{
		RecipientUserID: inviterUserID,
		ActorUserID:     inviteeUserID,
		FriendRequestID: requestID,
		Kind:            KindFriendRequestAccepted,
		Title:           "フレンズ申請が承認されました",
		Message:         actorName + "さんとフレンズになりました。",
	})
}

func (u *Usecase) NotifyInviteReceived(ctx context.Context, authToken string, inviteRow map[string]any) error {
	invite := InviteFromRow(inviteRow)
	if invite.ID == "" || invite.InviterUserID == "" || invite.InviteeUserID == "" || invite.InviterUserID == invite.InviteeUserID {
		return nil
	}
	actorName := u.actorName(ctx, authToken, invite.InviterUserID, KindInviteReceived)
	return u.tryCreateAndPush(ctx, authToken, Notification{
		RecipientUserID:  invite.InviteeUserID,
		ActorUserID:      invite.InviterUserID,
		InviteID:         invite.ID,
		NotificationDate: DateOrEmpty(invite.ScheduledDate),
		Kind:             KindInviteReceived,
		Title:            "お誘いが届きました",
		Message:          actorName + "さんから" + InvitePlanPhrase(invite, u.now()) + "が届きました。",
	})
}

func (u *Usecase) NotifyInviteAccepted(ctx context.Context, authToken string, inviteRow map[string]any) error {
	invite := InviteFromRow(inviteRow)
	if invite.ID == "" || invite.InviterUserID == "" || invite.InviteeUserID == "" || invite.InviterUserID == invite.InviteeUserID {
		return nil
	}
	actorName := u.actorName(ctx, authToken, invite.InviteeUserID, KindInviteAccepted)
	return u.tryCreateAndPush(ctx, authToken, Notification{
		RecipientUserID:  invite.InviterUserID,
		ActorUserID:      invite.InviteeUserID,
		InviteID:         invite.ID,
		NotificationDate: DateOrEmpty(invite.ScheduledDate),
		Kind:             KindInviteAccepted,
		Title:            "お誘いが承認されました",
		Message:          actorName + "さんが" + InvitePlanPhrase(invite, u.now()) + "を承認しました。",
	})
}

func (u *Usecase) NotifyYuruboCreated(ctx context.Context, authToken string, yuruboRow map[string]any, groupIDs []string) error {
	yuruboID, _ := yuruboRow["id"].(string)
	ownerUserID, _ := yuruboRow["owner_user_id"].(string)
	visibility, _ := yuruboRow["visibility"].(string)
	title, _ := yuruboRow["title"].(string)
	title = ShortText(title, 80)
	if yuruboID == "" || ownerUserID == "" || visibility == "" || visibility == "private" {
		return nil
	}
	recipientIDs, err := u.repository.VisibleYuruboRecipientIDs(ctx, authToken, ownerUserID, visibility, groupIDs)
	if err != nil {
		u.warn("failed to fetch yurubo notification recipients", KindYuruboCreated, err)
		return err
	}
	actorName := u.actorName(ctx, authToken, ownerUserID, KindYuruboCreated)
	message := actorName + "さんがゆるぼしました。"
	if title != "" {
		message = actorName + "さんが「" + title + "」でゆるぼしました。"
	}
	var firstErr error
	for _, recipientID := range recipientIDs {
		if err := u.tryCreateAndPush(ctx, authToken, Notification{
			RecipientUserID: recipientID,
			ActorUserID:     ownerUserID,
			Kind:            KindYuruboCreated,
			Title:           "フレンズがゆるぼしました",
			Message:         message,
			SystemKey:       "yurubo_created:" + yuruboID,
		}); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (u *Usecase) CreateTodayReservationReminders(ctx context.Context, authToken, userID, date string) error {
	if userID == "" {
		return nil
	}
	date = strings.TrimSpace(date)
	if date == "" {
		date = u.now().Format(time.DateOnly)
	}
	invites, err := u.repository.TodayAcceptedInvites(ctx, authToken, userID, date)
	if err != nil {
		return err
	}
	for _, invite := range invites {
		actorUserID := invite.InviterUserID
		if actorUserID == userID {
			actorUserID = invite.InviteeUserID
		}
		if invite.ID == "" || actorUserID == "" || actorUserID == userID {
			continue
		}
		actorName := u.actorName(ctx, authToken, actorUserID, KindTodayReservationReminder)
		if err := u.tryCreateAndPush(ctx, authToken, Notification{
			RecipientUserID:  userID,
			ActorUserID:      actorUserID,
			InviteID:         invite.ID,
			NotificationDate: date,
			Kind:             KindTodayReservationReminder,
			Title:            "今日の予定があります",
			Message:          actorName + "さんとの予定が今日あります。",
		}); err != nil {
			return err
		}
	}
	return nil
}

func (u *Usecase) actorName(ctx context.Context, authToken, userID string, event Kind) string {
	name, err := u.repository.DisplayName(ctx, authToken, userID)
	if err != nil {
		u.warn("failed to fetch notification actor profile", event, err)
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return defaultActorName
	}
	return name
}

func (u *Usecase) tryCreateAndPush(ctx context.Context, authToken string, notification Notification) error {
	created, err := u.repository.CreateNotification(ctx, notification)
	if err != nil {
		u.warn("failed to create notification", notification.Kind, err)
		return err
	}
	if !created || u.pushSender == nil {
		return nil
	}
	tokens, err := u.repository.PushTokens(ctx, notification.RecipientUserID)
	if err != nil {
		u.warn("failed to fetch push tokens", notification.Kind, err)
		return err
	}
	var firstErr error
	for _, token := range tokens {
		if err := u.pushSender.Send(ctx, token, notification.Title, notification.Message, notification.PushData()); err != nil {
			u.warn("failed to send push notification", notification.Kind, err)
			if isInvalidPushTokenError(err) {
				if deleteErr := u.repository.DeletePushToken(ctx, token); deleteErr != nil {
					u.warn("failed to delete invalid push token", notification.Kind, deleteErr)
				}
			}
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

type invalidPushTokenError interface {
	InvalidPushToken() bool
}

func isInvalidPushTokenError(err error) bool {
	var tokenErr invalidPushTokenError
	return errors.As(err, &tokenErr) && tokenErr.InvalidPushToken()
}

func (u *Usecase) warn(message string, event Kind, err error) {
	if u.logger == nil || err == nil {
		return
	}
	u.logger.Warn(message, "event", string(event), "error", err)
}
