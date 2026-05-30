package httpapi

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/yota/ohey/backend/internal/features/dailystatuses"
	"github.com/yota/ohey/backend/internal/features/friendgroups"
	"github.com/yota/ohey/backend/internal/features/friends"
	"github.com/yota/ohey/backend/internal/features/homefeed"
	"github.com/yota/ohey/backend/internal/features/invites"
	"github.com/yota/ohey/backend/internal/features/memories"
	"github.com/yota/ohey/backend/internal/features/notifications"
	"github.com/yota/ohey/backend/internal/features/profiles"
	"github.com/yota/ohey/backend/internal/features/usersafety"
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

type InviteRequest struct {
	InviteeUserID string `json:"invitee_user_id"`
	ScheduledDate string `json:"scheduled_date"`
}

type InviteUpdateRequest struct {
	Status string `json:"status"`
}

type MemoryReportRequest struct {
	Reason string `json:"reason"`
}

type UserSafetyUserRequest struct {
	TargetUserID  string `json:"target_user_id"`
	BlockedUserID string `json:"blocked_user_id"`
	MutedUserID   string `json:"muted_user_id"`
	UserID        string `json:"user_id"`
	Reason        string `json:"reason"`
}

type FeedHiddenMemoryRequest struct {
	MemoryID string `json:"memory_id"`
}

func (r *router) upsertProfile(w http.ResponseWriter, req *http.Request, authToken string) {
	var input ProfileSaveRequest
	if !decodeJSONBody(w, req, &input) {
		return
	}
	row, err := r.profileUsecase().BootstrapProfile(req.Context(), profiles.BootstrapUsecaseInput{
		AuthToken:  authToken,
		AuthUserID: req.Header.Get("X-Ohey-User-ID"),
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
		UserID:    req.Header.Get("X-Ohey-User-ID"),
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
		UserID:    req.Header.Get("X-Ohey-User-ID"),
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
		UserID:    req.Header.Get("X-Ohey-User-ID"),
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
		UserID:    req.Header.Get("X-Ohey-User-ID"),
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
		UserID:    req.Header.Get("X-Ohey-User-ID"),
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
		UserID:    req.Header.Get("X-Ohey-User-ID"),
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
		FromUserID: req.Header.Get("X-Ohey-User-ID"),
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
		UserID:    req.Header.Get("X-Ohey-User-ID"),
		Status:    input.Status,
	})
	if err != nil {
		writeFriendsError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, row)
}

func (r *router) likeMemory(w http.ResponseWriter, req *http.Request, authToken string) {
	state, err := r.memoryUsecase(req).LikeMemory(req.Context(), memories.LikeInput{
		AuthToken: authToken,
		MemoryID:  req.PathValue("id"),
		UserID:    req.Header.Get("X-Ohey-User-ID"),
	})
	if err != nil {
		writeMemoryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, state)
}

func (r *router) unlikeMemory(w http.ResponseWriter, req *http.Request, authToken string) {
	state, err := r.memoryUsecase(req).UnlikeMemory(req.Context(), memories.LikeInput{
		AuthToken: authToken,
		MemoryID:  req.PathValue("id"),
		UserID:    req.Header.Get("X-Ohey-User-ID"),
	})
	if err != nil {
		writeMemoryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, state)
}

func (r *router) reportMemory(w http.ResponseWriter, req *http.Request, authToken string) {
	var input MemoryReportRequest
	if !decodeJSONBody(w, req, &input) {
		return
	}
	if !r.enforceRateLimit(w, req, rateLimitReportMemory) {
		return
	}
	result, err := r.memoryUsecase(req).ReportMemory(req.Context(), memories.ReportInput{
		AuthToken:      authToken,
		MemoryID:       req.PathValue("id"),
		ReporterUserID: req.Header.Get("X-Ohey-User-ID"),
		Reason:         input.Reason,
	})
	if err != nil {
		writeMemoryError(w, err)
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
		ActorUserID:  req.Header.Get("X-Ohey-User-ID"),
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
		UserID:    req.Header.Get("X-Ohey-User-ID"),
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
		ActorUserID:  req.Header.Get("X-Ohey-User-ID"),
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
		ActorUserID:  req.Header.Get("X-Ohey-User-ID"),
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
		UserID:    req.Header.Get("X-Ohey-User-ID"),
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
		ActorUserID:  req.Header.Get("X-Ohey-User-ID"),
		TargetUserID: req.PathValue("id"),
	})
	if err != nil {
		writeUserSafetyError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"unmuted": true})
}

func (r *router) reportUser(w http.ResponseWriter, req *http.Request, authToken string) {
	var input UserSafetyUserRequest
	if !decodeJSONBody(w, req, &input) {
		return
	}
	if !r.enforceRateLimit(w, req, rateLimitReportUser) {
		return
	}
	row, err := r.userSafetyUsecase().ReportUser(req.Context(), usersafety.UserTargetInput{
		AuthToken:    authToken,
		ActorUserID:  req.Header.Get("X-Ohey-User-ID"),
		TargetUserID: firstNonEmpty(input.TargetUserID, input.UserID),
		Reason:       input.Reason,
	})
	if err != nil {
		writeUserSafetyError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, row)
}

func (r *router) hideMemoryFromFeed(w http.ResponseWriter, req *http.Request, authToken string) {
	var input FeedHiddenMemoryRequest
	if !decodeJSONBody(w, req, &input) {
		return
	}
	row, err := r.userSafetyUsecase().HideMemory(req.Context(), usersafety.MemoryInput{
		AuthToken: authToken,
		UserID:    req.Header.Get("X-Ohey-User-ID"),
		MemoryID:  input.MemoryID,
	})
	if err != nil {
		writeUserSafetyError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, row)
}

func (r *router) unhideMemoryFromFeed(w http.ResponseWriter, req *http.Request, authToken string) {
	err := r.userSafetyUsecase().UnhideMemory(req.Context(), usersafety.MemoryInput{
		AuthToken: authToken,
		UserID:    req.Header.Get("X-Ohey-User-ID"),
		MemoryID:  req.PathValue("id"),
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
		UserID:    req.Header.Get("X-Ohey-User-ID"),
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
		UserID:    req.Header.Get("X-Ohey-User-ID"),
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
		UserID:    req.Header.Get("X-Ohey-User-ID"),
	})
	if err != nil {
		writeNotificationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"updated_count": updatedCount})
}

func (r *router) deleteOwnAccount(w http.ResponseWriter, req *http.Request, _ string) {
	userID := strings.TrimSpace(req.Header.Get("X-Ohey-User-ID"))
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
	rows, err := r.inviteUsecase(req).ListTodayReservations(req.Context(), invites.ListInput{
		AuthToken:     authToken,
		UserID:        req.Header.Get("X-Ohey-User-ID"),
		ScheduledDate: dateOnlyParam(req, "date"),
	})
	if err != nil {
		writeSupabaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (r *router) listIncomingPendingInvites(w http.ResponseWriter, req *http.Request, authToken string) {
	rows, err := r.inviteUsecase(req).ListIncomingPending(req.Context(), invites.ListInput{
		AuthToken:     authToken,
		UserID:        req.Header.Get("X-Ohey-User-ID"),
		ScheduledDate: dateOnlyParam(req, "date"),
	})
	if err != nil {
		writeSupabaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (r *router) listOutgoingActiveInvites(w http.ResponseWriter, req *http.Request, authToken string) {
	rows, err := r.inviteUsecase(req).ListOutgoingActive(req.Context(), invites.ListInput{
		AuthToken:     authToken,
		UserID:        req.Header.Get("X-Ohey-User-ID"),
		ScheduledDate: dateOnlyParam(req, "date"),
	})
	if err != nil {
		writeSupabaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (r *router) createInvite(w http.ResponseWriter, req *http.Request, authToken string) {
	var input InviteRequest
	if !decodeJSONBody(w, req, &input) {
		return
	}
	if !r.enforceRateLimit(w, req, rateLimitCreateInvite) {
		return
	}
	row, err := r.inviteUsecase(req).CreateInvite(req.Context(), invites.CreateInput{
		AuthToken:     authToken,
		InviterUserID: req.Header.Get("X-Ohey-User-ID"),
		InviteeUserID: input.InviteeUserID,
		ScheduledDate: input.ScheduledDate,
	})
	if err != nil {
		writeInviteError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, row)
}

func (r *router) updateInvite(w http.ResponseWriter, req *http.Request, authToken string) {
	inviteID := req.PathValue("id")
	if _, err := invites.CleanUUID(inviteID, "invite id"); err != nil {
		writeInviteError(w, err)
		return
	}
	var input InviteUpdateRequest
	if !decodeJSONBody(w, req, &input) {
		return
	}
	row, err := r.inviteUsecase(req).UpdateInvite(req.Context(), invites.UpdateInput{
		AuthToken:       authToken,
		InviteID:        inviteID,
		RecipientUserID: req.Header.Get("X-Ohey-User-ID"),
		Status:          input.Status,
	})
	if err != nil {
		writeInviteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, row)
}

func (r *router) inviteUsecase(req *http.Request) *invites.Usecase {
	return invites.NewUsecase(invites.Dependencies{
		Repository: invites.NewSupabaseRepository(r.deps.Supabase),
		Publisher:  inviteEventPublisher{router: r, req: req},
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

type inviteEventPublisher struct {
	router *router
	req    *http.Request
}

func (p inviteEventPublisher) Publish(_ context.Context, authToken string, event invites.DomainEvent) {
	if p.router == nil || p.req == nil {
		return
	}
	row := event.InviteRow()
	switch event.Kind {
	case invites.EventInviteCreated:
		p.router.enqueueAndProcessNotificationOutboxEvent(p.req.Context(), authToken, notificationOutboxEvent{
			EventKind:       string(event.Kind),
			AggregateType:   "invite",
			AggregateID:     event.Invite.ID,
			ActorUserID:     event.Invite.InviterUserID,
			RecipientUserID: event.Invite.InviteeUserID,
			Payload:         row,
		})
	case invites.EventInviteAccepted:
		p.router.enqueueAndProcessNotificationOutboxEvent(p.req.Context(), authToken, notificationOutboxEvent{
			EventKind:       string(event.Kind),
			AggregateType:   "invite",
			AggregateID:     event.Invite.ID,
			ActorUserID:     event.Invite.InviteeUserID,
			RecipientUserID: event.Invite.InviterUserID,
			Payload:         row,
		})
	}
}

func writeInviteError(w http.ResponseWriter, err error) {
	if kind, ok := invites.ErrorKindOf(err); ok {
		switch kind {
		case invites.ErrorKindInvalidInput:
			writeError(w, http.StatusBadRequest, err.Error())
		case invites.ErrorKindConflict:
			writeError(w, http.StatusConflict, err.Error())
		case invites.ErrorKindNotFound:
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

func (r *router) memoryUsecase(req *http.Request) *memories.Usecase {
	return memories.NewUsecase(memories.Dependencies{
		Repository:   memories.NewSupabaseRepository(r.deps.Supabase),
		Publisher:    memoryEventPublisher{router: r, req: req},
		MediaCleaner: r.memoryPhotoCleaner(),
		Logger:       r.deps.Logger,
	})
}

type memoryEventPublisher struct {
	router *router
	req    *http.Request
}

func (p memoryEventPublisher) Publish(ctx context.Context, authToken string, event memories.DomainEvent) {
	if p.router == nil || p.req == nil {
		return
	}
	payload := map[string]any{
		"memory_id":     event.MemoryID,
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
	case memories.EventMemoryTagged:
		p.router.enqueueAndProcessNotificationOutboxEvent(ctx, authToken, notificationOutboxEvent{
			EventKind:     string(event.Kind),
			AggregateType: "memory",
			AggregateID:   event.MemoryID,
			ActorUserID:   event.ActorUserID,
			Payload:       payload,
		})
	case memories.EventMemoryLiked:
		p.router.enqueueAndProcessNotificationOutboxEvent(ctx, authToken, notificationOutboxEvent{
			EventKind:     string(event.Kind),
			AggregateType: "memory",
			AggregateID:   event.MemoryID,
			ActorUserID:   event.ActorUserID,
			Payload:       payload,
		})
	case memories.EventMemoryReported:
		p.router.enqueueAndProcessNotificationOutboxEvent(ctx, authToken, notificationOutboxEvent{
			EventKind:     string(event.Kind),
			AggregateType: "memory",
			AggregateID:   event.MemoryID,
			ActorUserID:   event.ActorUserID,
			Payload:       payload,
		})
	}
}

func writeMemoryError(w http.ResponseWriter, err error) {
	if kind, ok := memories.ErrorKindOf(err); ok {
		switch kind {
		case memories.ErrorKindInvalidInput:
			writeError(w, http.StatusBadRequest, err.Error())
		case memories.ErrorKindForbidden:
			writeError(w, http.StatusForbidden, err.Error())
		case memories.ErrorKindConflict:
			writeError(w, http.StatusConflict, err.Error())
		case memories.ErrorKindNotFound:
			writeError(w, http.StatusNotFound, err.Error())
		case memories.ErrorKindUpstream:
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
		"available",
		"maybe_available",
		"depends_on_time",
		"has_plans":
		return true
	default:
		return false
	}
}
