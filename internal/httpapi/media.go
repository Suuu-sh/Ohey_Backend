package httpapi

import (
	"net/http"

	"github.com/yota/nomo/backend/internal/features/media"
)

type MediaUploadURLRequest struct {
	Kind          string `json:"kind"`
	ContentType   string `json:"content_type"`
	FileExtension string `json:"file_extension"`
}

func (r *router) createMediaUploadURL(w http.ResponseWriter, req *http.Request, _ string) {
	var input MediaUploadURLRequest
	if !decodeJSONBody(w, req, &input) {
		return
	}
	result, err := r.mediaUsecase().CreateUploadURL(req.Context(), media.UploadRequest{
		Kind:          input.Kind,
		UserID:        req.Header.Get("X-Nomo-User-ID"),
		ContentType:   input.ContentType,
		FileExtension: input.FileExtension,
	})
	if err != nil {
		writeMediaError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (r *router) mediaUsecase() *media.Usecase {
	return media.NewUsecase(media.Dependencies{
		Storage: media.NewSupabaseStorageRepository(r.deps.Config.SupabaseURL, r.deps.Config.SupabaseServiceRoleKey, nil),
	})
}

func writeMediaError(w http.ResponseWriter, err error) {
	if kind, ok := media.ErrorKindOf(err); ok {
		switch kind {
		case media.ErrorKindInvalidInput:
			writeError(w, http.StatusBadRequest, err.Error())
		case media.ErrorKindUpstream:
			writeError(w, http.StatusBadGateway, "upstream service error")
		default:
			writeError(w, http.StatusBadRequest, err.Error())
		}
		return
	}
	writeSupabaseError(w, err)
}
