package httpapi

import (
	"context"
	"net/http"

	"github.com/yota/ohey/backend/internal/contracts"
	"github.com/yota/ohey/backend/internal/features/yurubos"
)

func (r *router) yurubosUsecase() *yurubos.Usecase {
	repository := yurubos.NewPostgresRepository(postgresPool(r))
	return yurubos.NewUsecase(yurubos.Dependencies{
		Repository: repository,
		Publisher:  yuruboEventPublisher{router: r},
	})
}

func (r *router) createYurubo(w http.ResponseWriter, req *http.Request, authToken string) {
	var body yurubos.CreateBody
	if !decodeJSONBody(w, req, &body) {
		return
	}
	row, err := r.yurubosUsecase().CreateYurubo(req.Context(), yurubos.CreateInput{
		AuthToken:   authToken,
		OwnerUserID: req.Header.Get("X-Ohey-User-ID"),
		Body:        body,
	})
	if err != nil {
		writeYurubosError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, row)
}

func (r *router) updateYurubo(w http.ResponseWriter, req *http.Request, authToken string) {
	var body yurubos.UpdateBody
	if !decodeJSONBody(w, req, &body) {
		return
	}
	row, err := r.yurubosUsecase().UpdateYurubo(req.Context(), yurubos.UpdateInput{
		AuthToken:   authToken,
		YuruboID:    req.PathValue("id"),
		OwnerUserID: req.Header.Get("X-Ohey-User-ID"),
		Body:        body,
	})
	if err != nil {
		writeYurubosError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, row)
}

func (r *router) deleteYurubo(w http.ResponseWriter, req *http.Request, authToken string) {
	row, err := r.yurubosUsecase().DeleteYurubo(req.Context(), yurubos.DeleteInput{
		AuthToken:   authToken,
		YuruboID:    req.PathValue("id"),
		OwnerUserID: req.Header.Get("X-Ohey-User-ID"),
	})
	if err != nil {
		writeYurubosError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, row)
}

func (r *router) listYurubos(w http.ResponseWriter, req *http.Request, authToken string) {
	rows, err := r.yurubosUsecase().ListYurubos(req.Context(), yurubos.ListInput{
		AuthToken: authToken,
		UserID:    req.Header.Get("X-Ohey-User-ID"),
		Limit:     req.URL.Query().Get("limit"),
	})
	if err != nil {
		writeYurubosError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (r *router) reactYurubo(w http.ResponseWriter, req *http.Request, authToken string) {
	var body yurubos.ReactionBody
	if !decodeJSONBody(w, req, &body) {
		return
	}
	state, err := r.yurubosUsecase().ReactYurubo(req.Context(), yurubos.ReactionInput{
		AuthToken: authToken,
		YuruboID:  req.PathValue("id"),
		UserID:    req.Header.Get("X-Ohey-User-ID"),
		Body:      body,
	})
	if err != nil {
		writeYurubosError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, state)
}

func (r *router) updateYuruboReaction(w http.ResponseWriter, req *http.Request, authToken string) {
	state, err := r.yurubosUsecase().ApproveReaction(req.Context(), yurubos.ApprovalInput{
		AuthToken:     authToken,
		YuruboID:      req.PathValue("id"),
		OwnerUserID:   req.Header.Get("X-Ohey-User-ID"),
		ParticipantID: req.PathValue("user_id"),
	})
	if err != nil {
		writeYurubosError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, state)
}

func (r *router) unreactYurubo(w http.ResponseWriter, req *http.Request, authToken string) {
	state, err := r.yurubosUsecase().UnreactYurubo(req.Context(), yurubos.ReactionInput{
		AuthToken: authToken,
		YuruboID:  req.PathValue("id"),
		UserID:    req.Header.Get("X-Ohey-User-ID"),
	})
	if err != nil {
		writeYurubosError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, state)
}

func writeYurubosError(w http.ResponseWriter, err error) {
	if kind, ok := yurubos.ErrorKindOf(err); ok {
		switch kind {
		case yurubos.ErrorKindInvalidInput:
			writeError(w, http.StatusBadRequest, err.Error())
		case yurubos.ErrorKindForbidden:
			writeError(w, http.StatusForbidden, err.Error())
		case yurubos.ErrorKindNotFound:
			writeError(w, http.StatusNotFound, err.Error())
		case yurubos.ErrorKindUpstream:
			writeError(w, http.StatusBadGateway, err.Error())
		default:
			writeError(w, http.StatusBadRequest, err.Error())
		}
		return
	}
	writeError(w, http.StatusBadGateway, "upstream database error")
}

type yuruboEventPublisher struct {
	router *router
}

func (p yuruboEventPublisher) Publish(ctx context.Context, authToken string, event yurubos.DomainEvent) {
	if p.router == nil || event.Kind != yurubos.EventYuruboCreated {
		return
	}
	payload := event.Row
	if payload == nil {
		payload = map[string]any{}
	}
	if len(event.GroupIDs) > 0 {
		payload["group_ids"] = event.GroupIDs
	}
	yuruboID, _ := payload["id"].(string)
	ownerUserID, _ := payload["owner_user_id"].(string)
	p.router.enqueueAndProcessNotificationOutboxEvent(ctx, authToken, notificationOutboxEvent{
		EventKind:     contracts.DomainEventYuruboCreated,
		AggregateType: "yurubo",
		AggregateID:   yuruboID,
		ActorUserID:   ownerUserID,
		Payload:       payload,
	})
}
