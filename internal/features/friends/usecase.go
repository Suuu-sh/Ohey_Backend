package friends

import (
	"context"
	"strings"
	"time"

	"github.com/yota/ohey/backend/internal/contracts"
)

type Dependencies struct {
	Repository Repository
	Publisher  EventPublisher
	Logger     Logger
	Now        func() time.Time
}

type Usecase struct {
	repository Repository
	publisher  EventPublisher
	logger     Logger
	now        func() time.Time
}

func NewUsecase(deps Dependencies) *Usecase {
	now := deps.Now
	if now == nil {
		now = time.Now
	}
	return &Usecase{repository: deps.Repository, publisher: deps.Publisher, logger: deps.Logger, now: now}
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

type ListFriendRequestsInput struct {
	AuthToken string
	UserID    string
	Direction string
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
	blocked, err := u.repository.BlockExistsBetweenUsers(ctx, input.AuthToken, userID, friendID)
	if err != nil {
		return nil, err
	}
	if blocked {
		return nil, UserError{Kind: ErrorKindConflict, Message: "cannot add blocked user"}
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

func (u *Usecase) DeleteFriendship(ctx context.Context, input FriendInput) (map[string]any, error) {
	userID, friendID, err := cleanFriendPair(input.UserID, input.FriendID, "friend id")
	if err != nil {
		return nil, err
	}
	row, err := u.repository.DeleteFriendship(ctx, input.AuthToken, userID, friendID)
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
		return FriendRequestStatus{AlreadyFriend: false, RequestState: contracts.RelationshipStateSelf}, nil
	}
	alreadyFriend, err := u.repository.FriendshipExists(ctx, input.AuthToken, userID, friendID)
	if err != nil {
		return FriendRequestStatus{}, err
	}
	requestState := contracts.RelationshipStateNone
	requestID := ""
	if !alreadyFriend {
		request, err := u.repository.PendingFriendRequestBetween(ctx, input.AuthToken, userID, friendID)
		if err != nil {
			return FriendRequestStatus{}, err
		}
		if request != nil {
			requestID, _ = request["id"].(string)
			if request["from_user_id"] == userID {
				requestState = contracts.RelationshipStateOutgoing
			} else {
				requestState = contracts.RelationshipStateIncoming
			}
		}
	}
	return FriendRequestStatus{AlreadyFriend: alreadyFriend, RequestState: requestState, RequestID: requestID}, nil
}

func (u *Usecase) ListFriendRequests(ctx context.Context, input ListFriendRequestsInput) ([]map[string]any, error) {
	userID, err := CleanUUID(input.UserID, "user id")
	if err != nil {
		return nil, err
	}
	direction, err := NormalizeRequestDirection(input.Direction)
	if err != nil {
		return nil, err
	}
	return u.repository.ListPendingFriendRequests(ctx, input.AuthToken, userID, direction)
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
	blocked, err := u.repository.BlockExistsBetweenUsers(ctx, input.AuthToken, fromUserID, toUserID)
	if err != nil {
		return nil, err
	}
	if blocked {
		return nil, UserError{Kind: ErrorKindConflict, Message: "cannot send a friend request to blocked user"}
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
	if u.publisher != nil {
		if event, ok := NewFriendRequestCreatedEvent(row); ok {
			u.publisher.Publish(ctx, input.AuthToken, event)
		}
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
			if u.publisher != nil {
				if event, ok := NewFriendRequestAcceptedEvent(row); ok {
					u.publisher.Publish(ctx, input.AuthToken, event)
				}
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
