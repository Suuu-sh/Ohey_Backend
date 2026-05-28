package httpapi

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/yota/nomo/backend/internal/features/dailystatuses"
	"github.com/yota/nomo/backend/internal/features/drinkinvites"
	"github.com/yota/nomo/backend/internal/features/drinklogs"
	"github.com/yota/nomo/backend/internal/features/friendgroups"
	"github.com/yota/nomo/backend/internal/features/friends"
	"github.com/yota/nomo/backend/internal/features/homefeed"
	"github.com/yota/nomo/backend/internal/features/notifications"
	"github.com/yota/nomo/backend/internal/features/profiles"
	"github.com/yota/nomo/backend/internal/features/usersafety"
)

type ProfileSaveRequest struct {
	UserID       string `json:"user_id"`
	DisplayName  string `json:"display_name"`
	Gender       string `json:"gender"`
	CharacterKey string `json:"character_key"`
	AvatarURL    string `json:"avatar_url"`
}

type FriendIDRequest struct {
	FriendID string `json:"friend_id"`
	ToUserID string `json:"to_user_id"`
}

type FriendRequestUpdateRequest struct {
	Status string `json:"status"`
}

type DrinkInviteRequest struct {
	ToUserID   string `json:"to_user_id"`
	InviteDate string `json:"invite_date"`
}

type DrinkInviteUpdateRequest struct {
	Status string `json:"status"`
}

type DrinkLogReportRequest struct {
	Reason string `json:"reason"`
}

type UserSafetyUserRequest struct {
	TargetUserID  string `json:"target_user_id"`
	BlockedUserID string `json:"blocked_user_id"`
	MutedUserID   string `json:"muted_user_id"`
	UserID        string `json:"user_id"`
}

type FeedHiddenDrinkLogRequest struct {
	DrinkLogID string `json:"drink_log_id"`
}

func (r *router) upsertProfile(w http.ResponseWriter, req *http.Request, authToken string) {
	var input ProfileSaveRequest
	if !decodeJSONBody(w, req, &input) {
		return
	}
	row, err := r.profileUsecase().BootstrapProfile(req.Context(), profiles.BootstrapUsecaseInput{
		AuthToken:  authToken,
		AuthUserID: req.Header.Get("X-Nomo-User-ID"),
		Request: profiles.BootstrapRequest{
			UserID:       input.UserID,
			DisplayName:  input.DisplayName,
			Gender:       input.Gender,
			CharacterKey: input.CharacterKey,
			AvatarURL:    input.AvatarURL,
		},
	})
	if err != nil {
		writeProfileError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, row)
}

func (r *router) getProfileByUserID(w http.ResponseWriter, req *http.Request, authToken string) {
	profile, err := r.profileUsecase().GetProfileByUserID(req.Context(), profiles.GetByUserIDInput{
		AuthToken: authToken,
		UserID:    req.PathValue("user_id"),
	})
	if err != nil {
		writeProfileError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, profile)
}

func (r *router) createFriendship(w http.ResponseWriter, req *http.Request, authToken string) {
	var input FriendIDRequest
	if !decodeJSONBody(w, req, &input) {
		return
	}
	friendID := strings.TrimSpace(input.FriendID)
	if friendID == "" {
		friendID = strings.TrimSpace(input.ToUserID)
	}
	row, err := r.friendsUsecase(req).CreateFriendship(req.Context(), friends.FriendInput{
		AuthToken: authToken,
		UserID:    req.Header.Get("X-Nomo-User-ID"),
		FriendID:  friendID,
	})
	if err != nil {
		writeFriendsError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, row)
}

func (r *router) deleteFriendship(w http.ResponseWriter, req *http.Request, authToken string) {
	row, err := r.friendsUsecase(req).DeleteFriendship(req.Context(), friends.FriendInput{
		AuthToken: authToken,
		UserID:    req.Header.Get("X-Nomo-User-ID"),
		FriendID:  req.PathValue("id"),
	})
	if err != nil {
		writeFriendsError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, row)
}

func (r *router) listFriendGroups(w http.ResponseWriter, req *http.Request, authToken string) {
	groups, err := r.friendGroupsUsecase().ListFriendGroups(req.Context(), friendgroups.AuthInput{
		AuthToken: authToken,
		UserID:    req.Header.Get("X-Nomo-User-ID"),
	})
	if err != nil {
		writeFriendGroupsError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, groups)
}

func (r *router) saveFriendGroups(w http.ResponseWriter, req *http.Request, authToken string) {
	var input friendgroups.SaveInputBody
	if !decodeJSONBody(w, req, &input) {
		return
	}
	groups, err := r.friendGroupsUsecase().SaveFriendGroups(req.Context(), friendgroups.SaveInput{
		AuthToken: authToken,
		UserID:    req.Header.Get("X-Nomo-User-ID"),
		Body:      input,
	})
	if err != nil {
		writeFriendGroupsError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, groups)
}

func (r *router) getFriendRequestStatus(w http.ResponseWriter, req *http.Request, authToken string) {
	status, err := r.friendsUsecase(req).GetFriendRequestStatus(req.Context(), friends.FriendInput{
		AuthToken: authToken,
		UserID:    req.Header.Get("X-Nomo-User-ID"),
		FriendID:  req.URL.Query().Get("friend_id"),
	})
	if err != nil {
		writeFriendsError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (r *router) listFriendRequests(w http.ResponseWriter, req *http.Request, authToken string) {
	rows, err := r.friendsUsecase(req).ListFriendRequests(req.Context(), friends.ListFriendRequestsInput{
		AuthToken: authToken,
		UserID:    req.Header.Get("X-Nomo-User-ID"),
		Direction: req.URL.Query().Get("direction"),
	})
	if err != nil {
		writeFriendsError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (r *router) createFriendRequest(w http.ResponseWriter, req *http.Request, authToken string) {
	var input FriendIDRequest
	if !decodeJSONBody(w, req, &input) {
		return
	}
	if !r.enforceRateLimit(w, req, rateLimitCreateFriendRequest) {
		return
	}
	row, err := r.friendsUsecase(req).CreateFriendRequest(req.Context(), friends.CreateFriendRequestInput{
		AuthToken:  authToken,
		FromUserID: req.Header.Get("X-Nomo-User-ID"),
		ToUserID:   input.ToUserID,
		FriendID:   input.FriendID,
	})
	if err != nil {
		writeFriendsError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, row)
}

func (r *router) updateFriendRequest(w http.ResponseWriter, req *http.Request, authToken string) {
	var input FriendRequestUpdateRequest
	if !decodeJSONBody(w, req, &input) {
		return
	}
	row, err := r.friendsUsecase(req).UpdateFriendRequest(req.Context(), friends.UpdateFriendRequestInput{
		AuthToken: authToken,
		RequestID: req.PathValue("id"),
		UserID:    req.Header.Get("X-Nomo-User-ID"),
		Status:    input.Status,
	})
	if err != nil {
		writeFriendsError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, row)
}

func (r *router) likeDrinkLog(w http.ResponseWriter, req *http.Request, authToken string) {
	state, err := r.drinkLogUsecase(req).LikeDrinkLog(req.Context(), drinklogs.LikeInput{
		AuthToken: authToken,
		LogID:     req.PathValue("id"),
		UserID:    req.Header.Get("X-Nomo-User-ID"),
	})
	if err != nil {
		writeDrinkLogError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, state)
}

func (r *router) unlikeDrinkLog(w http.ResponseWriter, req *http.Request, authToken string) {
	state, err := r.drinkLogUsecase(req).UnlikeDrinkLog(req.Context(), drinklogs.LikeInput{
		AuthToken: authToken,
		LogID:     req.PathValue("id"),
		UserID:    req.Header.Get("X-Nomo-User-ID"),
	})
	if err != nil {
		writeDrinkLogError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, state)
}

func (r *router) reportDrinkLog(w http.ResponseWriter, req *http.Request, authToken string) {
	var input DrinkLogReportRequest
	if !decodeJSONBody(w, req, &input) {
		return
	}
	if !r.enforceRateLimit(w, req, rateLimitReportDrinkLog) {
		return
	}
	result, err := r.drinkLogUsecase(req).ReportDrinkLog(req.Context(), drinklogs.ReportInput{
		AuthToken:      authToken,
		LogID:          req.PathValue("id"),
		ReporterUserID: req.Header.Get("X-Nomo-User-ID"),
		Reason:         input.Reason,
	})
	if err != nil {
		writeDrinkLogError(w, err)
		return
	}
	if result.Created {
		writeJSON(w, http.StatusCreated, result.Body)
		return
	}
	writeJSON(w, http.StatusOK, result.Body)
}

func (r *router) blockUser(w http.ResponseWriter, req *http.Request, authToken string) {
	var input UserSafetyUserRequest
	if !decodeJSONBody(w, req, &input) {
		return
	}
	if !r.enforceRateLimit(w, req, rateLimitBlockUser) {
		return
	}
	row, err := r.userSafetyUsecase().BlockUser(req.Context(), usersafety.UserTargetInput{
		AuthToken:    authToken,
		ActorUserID:  req.Header.Get("X-Nomo-User-ID"),
		TargetUserID: firstNonEmpty(input.BlockedUserID, input.TargetUserID, input.UserID),
	})
	if err != nil {
		writeUserSafetyError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, row)
}

func (r *router) listBlockedUsers(w http.ResponseWriter, req *http.Request, authToken string) {
	rows, err := r.userSafetyUsecase().ListBlockedUsers(req.Context(), usersafety.ListInput{
		AuthToken: authToken,
		UserID:    req.Header.Get("X-Nomo-User-ID"),
	})
	if err != nil {
		writeUserSafetyError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (r *router) unblockUser(w http.ResponseWriter, req *http.Request, authToken string) {
	err := r.userSafetyUsecase().UnblockUser(req.Context(), usersafety.UserTargetInput{
		AuthToken:    authToken,
		ActorUserID:  req.Header.Get("X-Nomo-User-ID"),
		TargetUserID: req.PathValue("id"),
	})
	if err != nil {
		writeUserSafetyError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"unblocked": true})
}

func (r *router) muteUser(w http.ResponseWriter, req *http.Request, authToken string) {
	var input UserSafetyUserRequest
	if !decodeJSONBody(w, req, &input) {
		return
	}
	if !r.enforceRateLimit(w, req, rateLimitMuteUser) {
		return
	}
	row, err := r.userSafetyUsecase().MuteUser(req.Context(), usersafety.UserTargetInput{
		AuthToken:    authToken,
		ActorUserID:  req.Header.Get("X-Nomo-User-ID"),
		TargetUserID: firstNonEmpty(input.MutedUserID, input.TargetUserID, input.UserID),
	})
	if err != nil {
		writeUserSafetyError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, row)
}

func (r *router) listMutedUsers(w http.ResponseWriter, req *http.Request, authToken string) {
	rows, err := r.userSafetyUsecase().ListMutedUsers(req.Context(), usersafety.ListInput{
		AuthToken: authToken,
		UserID:    req.Header.Get("X-Nomo-User-ID"),
	})
	if err != nil {
		writeUserSafetyError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (r *router) unmuteUser(w http.ResponseWriter, req *http.Request, authToken string) {
	err := r.userSafetyUsecase().UnmuteUser(req.Context(), usersafety.UserTargetInput{
		AuthToken:    authToken,
		ActorUserID:  req.Header.Get("X-Nomo-User-ID"),
		TargetUserID: req.PathValue("id"),
	})
	if err != nil {
		writeUserSafetyError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"unmuted": true})
}

func (r *router) hideDrinkLogFromFeed(w http.ResponseWriter, req *http.Request, authToken string) {
	var input FeedHiddenDrinkLogRequest
	if !decodeJSONBody(w, req, &input) {
		return
	}
	row, err := r.userSafetyUsecase().HideDrinkLog(req.Context(), usersafety.DrinkLogInput{
		AuthToken:  authToken,
		UserID:     req.Header.Get("X-Nomo-User-ID"),
		DrinkLogID: input.DrinkLogID,
	})
	if err != nil {
		writeUserSafetyError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, row)
}

func (r *router) unhideDrinkLogFromFeed(w http.ResponseWriter, req *http.Request, authToken string) {
	err := r.userSafetyUsecase().UnhideDrinkLog(req.Context(), usersafety.DrinkLogInput{
		AuthToken:  authToken,
		UserID:     req.Header.Get("X-Nomo-User-ID"),
		DrinkLogID: req.PathValue("id"),
	})
	if err != nil {
		writeUserSafetyError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"unhidden": true})
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func (r *router) listHomeFeed(w http.ResponseWriter, req *http.Request, authToken string) {
	rows, err := r.homeFeedUsecase().ListHomeFeed(req.Context(), homefeed.ListInput{
		AuthToken: authToken,
		UserID:    req.Header.Get("X-Nomo-User-ID"),
		Limit:     req.URL.Query().Get("limit"),
		Cursor:    req.URL.Query().Get("cursor"),
	})
	if err != nil {
		writeHomeFeedError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (r *router) listNotifications(w http.ResponseWriter, req *http.Request, authToken string) {
	rows, err := r.notificationUsecase(req).ListNotifications(req.Context(), notifications.ListInput{
		AuthToken: authToken,
		UserID:    req.Header.Get("X-Nomo-User-ID"),
		Date:      dateOnlyParam(req, "date"),
	})
	if err != nil {
		writeNotificationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (r *router) markNotificationsRead(w http.ResponseWriter, req *http.Request, authToken string) {
	updatedCount, err := r.notificationUsecase(req).MarkAllRead(req.Context(), notifications.MarkReadInput{
		AuthToken: authToken,
		UserID:    req.Header.Get("X-Nomo-User-ID"),
	})
	if err != nil {
		writeNotificationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"updated_count": updatedCount})
}

func (r *router) deleteOwnAccount(w http.ResponseWriter, req *http.Request, _ string) {
	userID := strings.TrimSpace(req.Header.Get("X-Nomo-User-ID"))
	if r.deps.AdminSupabase == nil || strings.TrimSpace(r.deps.Config.SupabaseServiceRoleKey) == "" {
		writeError(w, http.StatusServiceUnavailable, "account deletion is temporarily unavailable")
		return
	}
	if err := r.deps.AdminSupabase.AdminDeleteUser(req.Context(), userID); err != nil {
		writeSupabaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": userID})
}

func (r *router) listTodayReservations(w http.ResponseWriter, req *http.Request, authToken string) {
	rows, err := r.drinkInviteUsecase(req).ListTodayReservations(req.Context(), drinkinvites.ListInput{
		AuthToken:  authToken,
		UserID:     req.Header.Get("X-Nomo-User-ID"),
		InviteDate: dateOnlyParam(req, "date"),
	})
	if err != nil {
		writeSupabaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (r *router) listIncomingPendingInvites(w http.ResponseWriter, req *http.Request, authToken string) {
	rows, err := r.drinkInviteUsecase(req).ListIncomingPending(req.Context(), drinkinvites.ListInput{
		AuthToken:  authToken,
		UserID:     req.Header.Get("X-Nomo-User-ID"),
		InviteDate: dateOnlyParam(req, "date"),
	})
	if err != nil {
		writeSupabaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (r *router) listOutgoingActiveInvites(w http.ResponseWriter, req *http.Request, authToken string) {
	rows, err := r.drinkInviteUsecase(req).ListOutgoingActive(req.Context(), drinkinvites.ListInput{
		AuthToken:  authToken,
		UserID:     req.Header.Get("X-Nomo-User-ID"),
		InviteDate: dateOnlyParam(req, "date"),
	})
	if err != nil {
		writeSupabaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (r *router) createDrinkInvite(w http.ResponseWriter, req *http.Request, authToken string) {
	var input DrinkInviteRequest
	if !decodeJSONBody(w, req, &input) {
		return
	}
	if !r.enforceRateLimit(w, req, rateLimitCreateDrinkInvite) {
		return
	}
	row, err := r.drinkInviteUsecase(req).CreateDrinkInvite(req.Context(), drinkinvites.CreateInput{
		AuthToken:  authToken,
		FromUserID: req.Header.Get("X-Nomo-User-ID"),
		ToUserID:   input.ToUserID,
		InviteDate: input.InviteDate,
	})
	if err != nil {
		writeDrinkInviteError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, row)
}

func (r *router) updateDrinkInvite(w http.ResponseWriter, req *http.Request, authToken string) {
	inviteID := req.PathValue("id")
	if _, err := drinkinvites.CleanUUID(inviteID, "drink invite id"); err != nil {
		writeDrinkInviteError(w, err)
		return
	}
	var input DrinkInviteUpdateRequest
	if !decodeJSONBody(w, req, &input) {
		return
	}
	row, err := r.drinkInviteUsecase(req).UpdateDrinkInvite(req.Context(), drinkinvites.UpdateInput{
		AuthToken:       authToken,
		InviteID:        inviteID,
		RecipientUserID: req.Header.Get("X-Nomo-User-ID"),
		Status:          input.Status,
	})
	if err != nil {
		writeDrinkInviteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, row)
}

func (r *router) drinkInviteUsecase(req *http.Request) *drinkinvites.Usecase {
	return drinkinvites.NewUsecase(drinkinvites.Dependencies{
		Repository: drinkinvites.NewSupabaseRepository(r.deps.Supabase),
		Publisher:  drinkInviteEventPublisher{router: r, req: req},
	})
}

func (r *router) friendsUsecase(req *http.Request) *friends.Usecase {
	return friends.NewUsecase(friends.Dependencies{
		Repository: friends.NewSupabaseRepository(r.deps.Supabase, r.deps.AdminSupabase, r.deps.Config.SupabaseServiceRoleKey),
		Publisher:  friendRequestEventPublisher{router: r, req: req},
		Logger:     r.deps.Logger,
	})
}

type friendRequestEventPublisher struct {
	router *router
	req    *http.Request
}

func (p friendRequestEventPublisher) Publish(ctx context.Context, authToken string, event friends.DomainEvent) {
	if p.router == nil || p.req == nil {
		return
	}
	row := event.RequestRow()
	switch event.Kind {
	case friends.EventFriendRequestCreated:
		p.router.enqueueAndProcessNotificationOutboxEvent(ctx, authToken, notificationOutboxEvent{
			EventKind:       string(event.Kind),
			AggregateType:   "friend_request",
			AggregateID:     event.Request.ID,
			ActorUserID:     event.Request.FromUserID,
			RecipientUserID: event.Request.ToUserID,
			Payload:         row,
		})
	case friends.EventFriendRequestAccepted:
		p.router.enqueueAndProcessNotificationOutboxEvent(ctx, authToken, notificationOutboxEvent{
			EventKind:       string(event.Kind),
			AggregateType:   "friend_request",
			AggregateID:     event.Request.ID,
			ActorUserID:     event.Request.ToUserID,
			RecipientUserID: event.Request.FromUserID,
			Payload:         row,
		})
	}
}

type drinkInviteEventPublisher struct {
	router *router
	req    *http.Request
}

func (p drinkInviteEventPublisher) Publish(_ context.Context, authToken string, event drinkinvites.DomainEvent) {
	if p.router == nil || p.req == nil {
		return
	}
	row := event.InviteRow()
	switch event.Kind {
	case drinkinvites.EventDrinkInviteCreated:
		p.router.enqueueAndProcessNotificationOutboxEvent(p.req.Context(), authToken, notificationOutboxEvent{
			EventKind:       string(event.Kind),
			AggregateType:   "drink_invite",
			AggregateID:     event.Invite.ID,
			ActorUserID:     event.Invite.FromUserID,
			RecipientUserID: event.Invite.ToUserID,
			Payload:         row,
		})
	case drinkinvites.EventDrinkInviteAccepted:
		p.router.enqueueAndProcessNotificationOutboxEvent(p.req.Context(), authToken, notificationOutboxEvent{
			EventKind:       string(event.Kind),
			AggregateType:   "drink_invite",
			AggregateID:     event.Invite.ID,
			ActorUserID:     event.Invite.ToUserID,
			RecipientUserID: event.Invite.FromUserID,
			Payload:         row,
		})
	}
}

func writeDrinkInviteError(w http.ResponseWriter, err error) {
	if kind, ok := drinkinvites.ErrorKindOf(err); ok {
		switch kind {
		case drinkinvites.ErrorKindInvalidInput:
			writeError(w, http.StatusBadRequest, err.Error())
		case drinkinvites.ErrorKindConflict:
			writeError(w, http.StatusConflict, err.Error())
		case drinkinvites.ErrorKindNotFound:
			writeError(w, http.StatusNotFound, err.Error())
		default:
			writeError(w, http.StatusBadRequest, err.Error())
		}
		return
	}
	writeSupabaseError(w, err)
}

func writeFriendsError(w http.ResponseWriter, err error) {
	if kind, ok := friends.ErrorKindOf(err); ok {
		switch kind {
		case friends.ErrorKindInvalidInput:
			writeError(w, http.StatusBadRequest, err.Error())
		case friends.ErrorKindConflict:
			writeError(w, http.StatusConflict, err.Error())
		case friends.ErrorKindNotFound:
			writeError(w, http.StatusNotFound, err.Error())
		default:
			writeError(w, http.StatusBadRequest, err.Error())
		}
		return
	}
	writeSupabaseError(w, err)
}

func (r *router) drinkLogUsecase(req *http.Request) *drinklogs.Usecase {
	return drinklogs.NewUsecase(drinklogs.Dependencies{
		Repository:   drinklogs.NewSupabaseRepository(r.deps.Supabase),
		Publisher:    drinkLogEventPublisher{router: r, req: req},
		MediaCleaner: r.drinkLogPhotoCleaner(),
		Logger:       r.deps.Logger,
	})
}

type drinkLogEventPublisher struct {
	router *router
	req    *http.Request
}

func (p drinkLogEventPublisher) Publish(ctx context.Context, authToken string, event drinklogs.DomainEvent) {
	if p.router == nil || p.req == nil {
		return
	}
	payload := map[string]any{
		"log_id":        event.LogID,
		"owner_user_id": event.OwnerUserID,
		"actor_user_id": event.ActorUserID,
	}
	if len(event.FriendIDs) > 0 {
		payload["friend_ids"] = event.FriendIDs
	}
	if event.ReportReason != "" {
		payload["reason"] = string(event.ReportReason)
		payload["status"] = string(event.ModerationStatus)
	}
	switch event.Kind {
	case drinklogs.EventDrinkLogTagged:
		p.router.enqueueAndProcessNotificationOutboxEvent(ctx, authToken, notificationOutboxEvent{
			EventKind:     string(event.Kind),
			AggregateType: "drink_log",
			AggregateID:   event.LogID,
			ActorUserID:   event.ActorUserID,
			Payload:       payload,
		})
	case drinklogs.EventDrinkLogLiked:
		p.router.enqueueAndProcessNotificationOutboxEvent(ctx, authToken, notificationOutboxEvent{
			EventKind:     string(event.Kind),
			AggregateType: "drink_log",
			AggregateID:   event.LogID,
			ActorUserID:   event.ActorUserID,
			Payload:       payload,
		})
	case drinklogs.EventDrinkLogReported:
		p.router.enqueueAndProcessNotificationOutboxEvent(ctx, authToken, notificationOutboxEvent{
			EventKind:     string(event.Kind),
			AggregateType: "drink_log",
			AggregateID:   event.LogID,
			ActorUserID:   event.ActorUserID,
			Payload:       payload,
		})
	}
}

func writeDrinkLogError(w http.ResponseWriter, err error) {
	if kind, ok := drinklogs.ErrorKindOf(err); ok {
		switch kind {
		case drinklogs.ErrorKindInvalidInput:
			writeError(w, http.StatusBadRequest, err.Error())
		case drinklogs.ErrorKindForbidden:
			writeError(w, http.StatusForbidden, err.Error())
		case drinklogs.ErrorKindConflict:
			writeError(w, http.StatusConflict, err.Error())
		case drinklogs.ErrorKindNotFound:
			writeError(w, http.StatusNotFound, err.Error())
		case drinklogs.ErrorKindUpstream:
			writeError(w, http.StatusBadGateway, "upstream service error")
		default:
			writeError(w, http.StatusBadRequest, err.Error())
		}
		return
	}
	writeSupabaseError(w, err)
}

func (r *router) userSafetyUsecase() *usersafety.Usecase {
	return usersafety.NewUsecase(usersafety.Dependencies{
		Repository: usersafety.NewSupabaseRepository(r.deps.Supabase, r.deps.AdminSupabase, r.deps.Config.SupabaseServiceRoleKey),
	})
}

func writeUserSafetyError(w http.ResponseWriter, err error) {
	if kind, ok := usersafety.ErrorKindOf(err); ok {
		switch kind {
		case usersafety.ErrorKindInvalidInput:
			writeError(w, http.StatusBadRequest, err.Error())
		case usersafety.ErrorKindForbidden:
			writeError(w, http.StatusForbidden, err.Error())
		default:
			writeError(w, http.StatusBadRequest, err.Error())
		}
		return
	}
	writeSupabaseError(w, err)
}

func (r *router) friendGroupsUsecase() *friendgroups.Usecase {
	return friendgroups.NewUsecase(friendgroups.Dependencies{
		Repository: friendgroups.NewSupabaseRepository(r.deps.Supabase),
	})
}

func writeFriendGroupsError(w http.ResponseWriter, err error) {
	if kind, ok := friendgroups.ErrorKindOf(err); ok {
		switch kind {
		case friendgroups.ErrorKindInvalidInput:
			writeError(w, http.StatusBadRequest, err.Error())
		case friendgroups.ErrorKindForbidden:
			writeError(w, http.StatusForbidden, err.Error())
		default:
			writeError(w, http.StatusBadRequest, err.Error())
		}
		return
	}
	writeSupabaseError(w, err)
}

func (r *router) homeFeedUsecase() *homefeed.Usecase {
	return homefeed.NewUsecase(homefeed.Dependencies{
		Repository: homefeed.NewSupabaseRepository(r.deps.Supabase),
	})
}

func writeHomeFeedError(w http.ResponseWriter, err error) {
	if kind, ok := homefeed.ErrorKindOf(err); ok {
		switch kind {
		case homefeed.ErrorKindInvalidInput:
			writeError(w, http.StatusBadRequest, err.Error())
		default:
			writeError(w, http.StatusBadRequest, err.Error())
		}
		return
	}
	writeSupabaseError(w, err)
}

func (r *router) profileUsecase() *profiles.Usecase {
	return profiles.NewUsecase(profiles.Dependencies{
		Repository: profiles.NewSupabaseRepository(r.deps.Supabase),
	})
}

func writeProfileError(w http.ResponseWriter, err error) {
	if kind, ok := profiles.ErrorKindOf(err); ok {
		switch kind {
		case profiles.ErrorKindInvalidInput:
			writeError(w, http.StatusBadRequest, err.Error())
		case profiles.ErrorKindNotFound:
			writeError(w, http.StatusNotFound, err.Error())
		default:
			writeError(w, http.StatusBadRequest, err.Error())
		}
		return
	}
	writeSupabaseError(w, err)
}

func (r *router) dailyStatusUsecase() *dailystatuses.Usecase {
	return dailystatuses.NewUsecase(dailystatuses.Dependencies{
		Repository: dailystatuses.NewSupabaseRepository(r.deps.Supabase),
	})
}

func writeDailyStatusError(w http.ResponseWriter, err error) {
	if kind, ok := dailystatuses.ErrorKindOf(err); ok {
		switch kind {
		case dailystatuses.ErrorKindInvalidInput:
			writeError(w, http.StatusBadRequest, err.Error())
		default:
			writeError(w, http.StatusBadRequest, err.Error())
		}
		return
	}
	writeSupabaseError(w, err)
}

func dateOnlyParam(req *http.Request, name string) string {
	value, errMessage := cleanDateOnlyOrToday(req.URL.Query().Get(name), name)
	if errMessage != "" {
		return time.Now().Format(time.DateOnly)
	}
	return value
}

func isValidDailyStatus(status string) bool {
	switch status {
	case "unselected",
		"can_drink_today",
		"non_alcohol",
		"liver_rest",
		"has_plans":
		return true
	default:
		return false
	}
}
