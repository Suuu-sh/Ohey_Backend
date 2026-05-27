package friends

import (
	"context"
	"strings"
	"time"
)

type Dependencies struct {
	Repository Repository
	Notifier   Notifier
	Logger     Logger
	Now        func() time.Time
}

type Usecase struct {
	repository Repository
	notifier   Notifier
	logger     Logger
	now        func() time.Time
}

func NewUsecase(deps Dependencies) *Usecase {
	now := deps.Now
	if now == nil {
		now = time.Now
	}
	return &Usecase{repository: deps.Repository, notifier: deps.Notifier, logger: deps.Logger, now: now}
}

type ListInput struct {
	AuthToken string
	UserID    string
	Date      string
}

type FriendInput struct {
	AuthToken string
	UserID    string
	FriendID  string
}

type FavoriteInput struct {
	AuthToken  string
	UserID     string
	FriendID   string
	IsFavorite bool
}

type CreateFriendRequestInput struct {
	AuthToken  string
	FromUserID string
	ToUserID   string
	FriendID   string
}

type UpdateFriendRequestInput struct {
	AuthToken string
	RequestID string
	UserID    string
	Status    string
}

func (u *Usecase) ListFriends(ctx context.Context, input ListInput) ([]map[string]any, error) {
	userID, err := CleanUUID(input.UserID, "user id")
	if err != nil {
		return nil, err
	}
	rows, err := u.repository.ListFriendships(ctx, input.AuthToken, userID)
	if err != nil {
		return nil, err
	}
	date := strings.TrimSpace(input.Date)
	if date == "" {
		date = u.now().Format(time.DateOnly)
	}
	if err := u.repository.AttachTodayStatuses(ctx, input.AuthToken, rows, date); err != nil {
		return nil, err
	}
	if err := u.repository.AttachDrinkStats(ctx, input.AuthToken, userID, rows); err != nil {
		if u.logger != nil {
			u.logger.Warn("failed to attach friend drink stats", "error", err)
		}
	}
	return rows, nil
}

func (u *Usecase) CreateFriendship(ctx context.Context, input FriendInput) (map[string]any, error) {
	userID, friendID, err := cleanFriendPair(input.UserID, input.FriendID, "friend_id")
	if err != nil {
		return nil, err
	}
	if friendID == userID {
		return nil, UserError{Kind: ErrorKindInvalidInput, Message: "cannot add yourself as a friend"}
	}
	return u.repository.UpsertFriendshipPair(ctx, input.AuthToken, userID, friendID)
}

func (u *Usecase) UpdateFriendFavorite(ctx context.Context, input FavoriteInput) (map[string]any, error) {
	userID, friendID, err := cleanFriendPair(input.UserID, input.FriendID, "friend id")
	if err != nil {
		return nil, err
	}
	row, err := u.repository.UpdateFriendFavorite(ctx, input.AuthToken, userID, friendID, input.IsFavorite)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, UserError{Kind: ErrorKindNotFound, Message: "friendship not found"}
	}
	return row, nil
}

func (u *Usecase) GetFriendRequestStatus(ctx context.Context, input FriendInput) (FriendRequestStatus, error) {
	userID, friendID, err := cleanFriendPair(input.UserID, input.FriendID, "friend_id")
	if err != nil {
		return FriendRequestStatus{}, err
	}
	if friendID == userID {
		return FriendRequestStatus{AlreadyFriend: false, RequestState: "self"}, nil
	}
	alreadyFriend, err := u.repository.FriendshipExists(ctx, input.AuthToken, userID, friendID)
	if err != nil {
		return FriendRequestStatus{}, err
	}
	requestState := "none"
	if !alreadyFriend {
		request, err := u.repository.PendingFriendRequestBetween(ctx, input.AuthToken, userID, friendID)
		if err != nil {
			return FriendRequestStatus{}, err
		}
		if request != nil {
			if request["from_user_id"] == userID {
				requestState = "outgoing"
			} else {
				requestState = "incoming"
			}
		}
	}
	return FriendRequestStatus{AlreadyFriend: alreadyFriend, RequestState: requestState}, nil
}

func (u *Usecase) CreateFriendRequest(ctx context.Context, input CreateFriendRequestInput) (map[string]any, error) {
	fromUserID, err := CleanUUID(input.FromUserID, "from_user_id")
	if err != nil {
		return nil, err
	}
	toUserID := strings.TrimSpace(input.ToUserID)
	if toUserID == "" {
		toUserID = strings.TrimSpace(input.FriendID)
	}
	toUserID, err = CleanUUID(toUserID, "to_user_id")
	if err != nil {
		return nil, err
	}
	if toUserID == fromUserID {
		return nil, UserError{Kind: ErrorKindInvalidInput, Message: "cannot send a friend request to yourself"}
	}
	alreadyFriend, err := u.repository.FriendshipExists(ctx, input.AuthToken, fromUserID, toUserID)
	if err != nil {
		return nil, err
	}
	if alreadyFriend {
		return nil, UserError{Kind: ErrorKindConflict, Message: "already friends"}
	}
	row, err := u.repository.CreateFriendRequest(ctx, input.AuthToken, fromUserID, toUserID)
	if err != nil {
		return nil, err
	}
	if u.notifier != nil {
		u.notifier.FriendRequestReceived(ctx, input.AuthToken, row)
	}
	return row, nil
}

func (u *Usecase) UpdateFriendRequest(ctx context.Context, input UpdateFriendRequestInput) (map[string]any, error) {
	requestID, err := CleanUUID(input.RequestID, "friend request id")
	if err != nil {
		return nil, err
	}
	userID, err := CleanUUID(input.UserID, "user id")
	if err != nil {
		return nil, err
	}
	status, err := NormalizeRequestStatus(input.Status)
	if err != nil {
		return nil, err
	}
	row, err := u.repository.UpdatePendingFriendRequestStatus(ctx, input.AuthToken, requestID, userID, status, u.now().UTC())
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, UserError{Kind: ErrorKindNotFound, Message: "friend request not found"}
	}
	if status == RequestStatusAccepted {
		request := FriendRequestFromRow(row)
		if request.FromUserID != "" && request.ToUserID != "" {
			if _, err := u.repository.UpsertFriendshipPair(ctx, input.AuthToken, request.FromUserID, request.ToUserID); err != nil {
				return nil, err
			}
			if u.notifier != nil {
				u.notifier.FriendRequestAccepted(ctx, input.AuthToken, row)
			}
		}
	}
	return row, nil
}

func cleanFriendPair(userID, friendID, friendField string) (string, string, error) {
	cleanUserID, err := CleanUUID(userID, "user id")
	if err != nil {
		return "", "", err
	}
	cleanFriendID, err := CleanUUID(friendID, friendField)
	if err != nil {
		return "", "", err
	}
	return cleanUserID, cleanFriendID, nil
}
