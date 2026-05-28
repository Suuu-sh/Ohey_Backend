package httpapi

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/yota/nomo/backend/internal/features/drinklogs"
	"github.com/yota/nomo/backend/internal/features/media"
)

type MediaUploadURLRequest struct {
	Kind          string `json:"kind"`
	ContentType   string `json:"content_type"`
	FileExtension string `json:"file_extension"`
}

type MediaDisplayURLRequest struct {
	Path string `json:"path"`
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

func (r *router) createMediaDisplayURL(w http.ResponseWriter, req *http.Request, _ string) {
	var input MediaDisplayURLRequest
	if !decodeJSONBody(w, req, &input) {
		return
	}
	result, err := r.mediaUsecase().CreateDisplayURL(req.Context(), media.DisplayURLRequest{
		UserID: req.Header.Get("X-Nomo-User-ID"),
		Path:   input.Path,
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

type drinkLogPhotoCleaner struct {
	storage *media.SupabaseStorageRepository
}

func (c drinkLogPhotoCleaner) DeleteDrinkLogPhoto(ctx context.Context, photoPath string) error {
	if c.storage == nil {
		return nil
	}
	return c.storage.DeleteObject(ctx, media.PhotoBucket, photoPath)
}

func (r *router) drinkLogPhotoCleaner() drinklogs.MediaCleaner {
	if r.deps.Config.SupabaseServiceRoleKey == "" {
		return nil
	}
	return drinkLogPhotoCleaner{storage: media.NewSupabaseStorageRepository(r.deps.Config.SupabaseURL, r.deps.Config.SupabaseServiceRoleKey, nil)}
}

func (r *router) adminListOrphanDrinkLogPhotos(w http.ResponseWriter, req *http.Request, _ AuthUser) {
	userID, errMessage := cleanUUID(req.URL.Query().Get("user_id"), "user_id")
	if errMessage != "" {
		writeError(w, http.StatusBadRequest, errMessage)
		return
	}
	limit := 100
	if rawLimit := strings.TrimSpace(req.URL.Query().Get("limit")); rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil || parsed <= 0 {
			writeError(w, http.StatusBadRequest, "limit must be a positive integer")
			return
		}
		limit = min(parsed, 1000)
	}
	prefix := "users/" + userID + "/drink_logs"
	storage := media.NewSupabaseStorageRepository(r.deps.Config.SupabaseURL, r.deps.Config.SupabaseServiceRoleKey, nil)
	objects, err := storage.ListObjects(req.Context(), media.PhotoBucket, prefix, limit)
	if err != nil {
		writeSupabaseError(w, err)
		return
	}
	activePaths, err := r.adminActiveDrinkLogPhotoPaths(req.Context(), userID)
	if err != nil {
		writeSupabaseError(w, err)
		return
	}
	orphanPaths := make([]string, 0)
	for _, object := range objects {
		if object.Path == "" || activePaths[object.Path] {
			continue
		}
		orphanPaths = append(orphanPaths, object.Path)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"user_id":       userID,
		"prefix":        prefix,
		"checked_count": len(objects),
		"orphan_paths":  orphanPaths,
	})
}

func (r *router) adminActiveDrinkLogPhotoPaths(ctx context.Context, userID string) (map[string]bool, error) {
	q := url.Values{}
	q.Set("select", "photo_path")
	q.Set("owner_user_id", "eq."+userID)
	q.Set("photo_path", "neq.")
	q.Set("limit", "10000")
	var rows []map[string]any
	if err := r.deps.AdminSupabase.Get(ctx, r.deps.Config.SupabaseServiceRoleKey, "drink_logs", q, &rows); err != nil {
		return nil, err
	}
	paths := make(map[string]bool, len(rows))
	for _, row := range rows {
		if path := strings.TrimSpace(stringValue(row, "photo_path")); path != "" {
			paths[path] = true
		}
	}
	return paths, nil
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
