package httpapi

import (
	"net/http"

	"github.com/yota/nomo/backend/internal/features/notifications"
)

func (r *router) notificationUsecase(_ *http.Request) *notifications.Usecase {
	return notifications.NewUsecase(notifications.Dependencies{
		Repository: notifications.NewSupabaseRepository(r.deps.Supabase, r.deps.AdminSupabase, r.deps.Config.SupabaseServiceRoleKey),
		PushSender: r.deps.FCM,
		Logger:     r.deps.Logger,
	})
}

func (r *router) createFriendRequestReceivedNotification(req *http.Request, authToken string, requestRow map[string]any) {
	r.notificationUsecase(req).NotifyFriendRequestReceived(req.Context(), authToken, requestRow)
}

func (r *router) createFriendRequestAcceptedNotification(req *http.Request, authToken string, requestRow map[string]any) {
	r.notificationUsecase(req).NotifyFriendRequestAccepted(req.Context(), authToken, requestRow)
}

func (r *router) createDrinkInviteReceivedNotification(req *http.Request, authToken string, inviteRow map[string]any) {
	r.notificationUsecase(req).NotifyDrinkInviteReceived(req.Context(), authToken, inviteRow)
}

func (r *router) createDrinkInviteAcceptedNotification(req *http.Request, authToken string, inviteRow map[string]any) {
	r.notificationUsecase(req).NotifyDrinkInviteAccepted(req.Context(), authToken, inviteRow)
}

func (r *router) createDrinkLogTaggedNotifications(req *http.Request, authToken, logID, ownerUserID string, friendIDs []string) {
	r.notificationUsecase(req).NotifyDrinkLogTagged(req.Context(), authToken, logID, ownerUserID, friendIDs)
}

func (r *router) createDrinkLogLikeNotification(req *http.Request, authToken, logID, actorUserID string) {
	r.notificationUsecase(req).NotifyDrinkLogLiked(req.Context(), authToken, logID, actorUserID)
}

func (r *router) adminCreateNotification(w http.ResponseWriter, req *http.Request, _ AuthUser) {
	var input AdminCreateSystemNotificationRequest
	if !decodeJSONBody(w, req, &input) {
		return
	}
	result, err := r.notificationUsecase(req).CreateSystemNotifications(req.Context(), notifications.CreateSystemInput{
		Title:            input.Title,
		Message:          input.Message,
		RecipientUserIDs: input.RecipientUserIDs,
		SendToAll:        input.SendToAll,
		SystemKey:        input.SystemKey,
	})
	if err != nil {
		writeNotificationError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func writeNotificationError(w http.ResponseWriter, err error) {
	if kind, ok := notifications.ErrorKindOf(err); ok {
		switch kind {
		case notifications.ErrorKindInvalidInput:
			writeError(w, http.StatusBadRequest, err.Error())
		case notifications.ErrorKindUpstream:
			writeError(w, http.StatusBadGateway, "upstream service error")
		default:
			writeError(w, http.StatusBadRequest, err.Error())
		}
		return
	}
	writeSupabaseError(w, err)
}
