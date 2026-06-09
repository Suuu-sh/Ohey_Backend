package httpapi

import (
	"net/http"

	"github.com/yota/ohey/backend/internal/features/wishitems"
)

func (r *router) wishItemsUsecase() *wishitems.Usecase {
	var repository wishitems.Repository = wishitems.NewSupabaseRepository(r.deps.Supabase)
	if r.deps.Config.DataStore == "postgres" || r.deps.Config.DataStore == "neon" {
		if r.deps.Postgres == nil {
			repository = wishitems.NewPostgresRepository(nil)
		} else {
			repository = wishitems.NewPostgresRepository(r.deps.Postgres.Pool())
		}
	}
	return wishitems.NewUsecase(wishitems.Dependencies{
		Repository: repository,
	})
}

func (r *router) listWishItems(w http.ResponseWriter, req *http.Request, authToken string) {
	rows, err := r.wishItemsUsecase().ListWishItems(req.Context(), wishitems.ListInput{
		AuthToken: authToken,
		UserID:    req.Header.Get("X-Ohey-User-ID"),
		Limit:     req.URL.Query().Get("limit"),
	})
	if err != nil {
		writeWishItemsError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (r *router) listProfileWishItems(w http.ResponseWriter, req *http.Request, authToken string) {
	rows, err := r.wishItemsUsecase().ListProfileWishItems(req.Context(), wishitems.ProfileListInput{
		AuthToken:    authToken,
		ViewerUserID: req.Header.Get("X-Ohey-User-ID"),
		ProfileID:    req.PathValue("id"),
		Limit:        req.URL.Query().Get("limit"),
	})
	if err != nil {
		writeWishItemsError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (r *router) createWishItem(w http.ResponseWriter, req *http.Request, authToken string) {
	var body wishitems.CreateBody
	if !decodeJSONBody(w, req, &body) {
		return
	}
	row, err := r.wishItemsUsecase().CreateWishItem(req.Context(), wishitems.CreateInput{
		AuthToken:   authToken,
		OwnerUserID: req.Header.Get("X-Ohey-User-ID"),
		Body:        body,
	})
	if err != nil {
		writeWishItemsError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, row)
}

func (r *router) updateWishItem(w http.ResponseWriter, req *http.Request, authToken string) {
	var body wishitems.UpdateBody
	if !decodeJSONBody(w, req, &body) {
		return
	}
	row, err := r.wishItemsUsecase().UpdateWishItem(req.Context(), wishitems.UpdateInput{
		AuthToken:   authToken,
		WishItemID:  req.PathValue("id"),
		OwnerUserID: req.Header.Get("X-Ohey-User-ID"),
		Body:        body,
	})
	if err != nil {
		writeWishItemsError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, row)
}

func (r *router) deleteWishItem(w http.ResponseWriter, req *http.Request, authToken string) {
	row, err := r.wishItemsUsecase().DeleteWishItem(req.Context(), wishitems.DeleteInput{
		AuthToken:   authToken,
		WishItemID:  req.PathValue("id"),
		OwnerUserID: req.Header.Get("X-Ohey-User-ID"),
	})
	if err != nil {
		writeWishItemsError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, row)
}

func writeWishItemsError(w http.ResponseWriter, err error) {
	if kind, ok := wishitems.ErrorKindOf(err); ok {
		switch kind {
		case wishitems.ErrorKindInvalidInput:
			writeError(w, http.StatusBadRequest, err.Error())
		case wishitems.ErrorKindNotFound:
			writeError(w, http.StatusNotFound, err.Error())
		case wishitems.ErrorKindUpstream:
			writeError(w, http.StatusBadGateway, err.Error())
		default:
			writeError(w, http.StatusBadRequest, err.Error())
		}
		return
	}
	writeError(w, http.StatusBadGateway, "upstream database error")
}
