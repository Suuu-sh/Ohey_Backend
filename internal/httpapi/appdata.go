package httpapi

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/yota/nomo/backend/internal/features/drinkinvites"
	"github.com/yota/nomo/backend/internal/features/drinklogs"
	"github.com/yota/nomo/backend/internal/features/friendgroups"
	"github.com/yota/nomo/backend/internal/features/friends"
	"github.com/yota/nomo/backend/internal/features/homefeed"
	"github.com/yota/nomo/backend/internal/features/notifications"
	"github.com/yota/nomo/backend/internal/features/profiles"
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

func (r *router) createFriendRequest(w http.ResponseWriter, req *http.Request, authToken string) {
	var input FriendIDRequest
	if !decodeJSONBody(w, req, &input) {
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

func (r *router) listHomeFeed(w http.ResponseWriter, req *http.Request, authToken string) {
	rows, err := r.homeFeedUsecase().ListHomeFeed(req.Context(), homefeed.ListInput{
		AuthToken: authToken,
		UserID:    req.Header.Get("X-Nomo-User-ID"),
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
		Repository: friends.NewSupabaseRepository(r.deps.Supabase),
		Notifier:   friendRequestNotifier{router: r, req: req},
		Logger:     r.deps.Logger,
	})
}

type friendRequestNotifier struct {
	router *router
	req    *http.Request
}

func (n friendRequestNotifier) FriendRequestReceived(_ context.Context, authToken string, requestRow map[string]any) {
	if n.router == nil || n.req == nil {
		return
	}
	n.router.createFriendRequestReceivedNotification(n.req, authToken, requestRow)
}

func (n friendRequestNotifier) FriendRequestAccepted(_ context.Context, authToken string, requestRow map[string]any) {
	if n.router == nil || n.req == nil {
		return
	}
	n.router.createFriendRequestAcceptedNotification(n.req, authToken, requestRow)
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
		p.router.createDrinkInviteReceivedNotification(p.req, authToken, row)
	case drinkinvites.EventDrinkInviteAccepted:
		p.router.createDrinkInviteAcceptedNotification(p.req, authToken, row)
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
		Repository: drinklogs.NewSupabaseRepository(r.deps.Supabase),
		Notifier:   drinkLogNotifier{router: r, req: req},
	})
}

type drinkLogNotifier struct {
	router *router
	req    *http.Request
}

func (n drinkLogNotifier) DrinkLogTagged(_ context.Context, authToken, logID, ownerUserID string, friendIDs []string) {
	if n.router == nil || n.req == nil {
		return
	}
	n.router.createDrinkLogTaggedNotifications(n.req, authToken, logID, ownerUserID, friendIDs)
}

func (n drinkLogNotifier) DrinkLogLiked(_ context.Context, authToken, logID, actorUserID string) {
	if n.router == nil || n.req == nil {
		return
	}
	n.router.createDrinkLogLikeNotification(n.req, authToken, logID, actorUserID)
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
