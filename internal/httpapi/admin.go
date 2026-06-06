package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/yota/ohey/backend/internal/contracts"
	"github.com/yota/ohey/backend/internal/features/dailystatuses"
	"github.com/yota/ohey/backend/internal/features/memories"
	"github.com/yota/ohey/backend/internal/features/profiles"
	"github.com/yota/ohey/backend/internal/features/yurubos"
	"github.com/yota/ohey/backend/internal/supabase"
)

const officialProfileUserID = "ohey_official"
const officialProfileDisplayName = "Ohey公式"
const officialProfileEmail = "ohey-official@official.ohey.app"

func (r *router) adminMe(w http.ResponseWriter, req *http.Request, adminUser AuthUser) {
	writeJSON(w, http.StatusOK, map[string]any{
		"id":          adminUser.ID,
		"email":       adminUser.Email,
		"is_admin":    true,
		"environment": r.deps.Config.Environment,
	})
}

func (r *router) adminListUsers(w http.ResponseWriter, req *http.Request, _ AuthUser) {
	statusDate, errMessage := cleanDateOnlyOrToday(req.URL.Query().Get("date"), "date")
	if errMessage != "" {
		writeError(w, http.StatusBadRequest, errMessage)
		return
	}

	q := url.Values{}
	q.Set(
		"select",
		"id,user_id,display_name,character_key,avatar_url,is_plus,created_at,updated_at",
	)
	q.Set("order", "created_at.desc")
	q.Set("limit", "80")
	if search := sanitizePostgRESTSearch(req.URL.Query().Get("search")); search != "" {
		q.Set("or", "(display_name.ilike.*"+search+"*,user_id.ilike.*"+search+"*)")
	}

	var rows []map[string]any
	if err := r.deps.AdminSupabase.Get(req.Context(), r.deps.Config.SupabaseServiceRoleKey, "profiles", q, &rows); err != nil {
		writeSupabaseError(w, err)
		return
	}
	statusByUserID, err := r.adminStatusesForDate(req.Context(), rows, statusDate)
	if err != nil {
		writeSupabaseError(w, err)
		return
	}
	for _, row := range rows {
		id, _ := row["id"].(string)
		status := statusByUserID[id]
		if strings.TrimSpace(status) == "" {
			status = contracts.DailyStatusUnselected
		}
		row["status"] = status
	}
	writeJSON(w, http.StatusOK, rows)
}

func (r *router) adminCreateUser(w http.ResponseWriter, req *http.Request, _ AuthUser) {
	var input AdminCreateUserRequest
	if !decodeJSONBody(w, req, &input) {
		return
	}
	input.Email = strings.TrimSpace(input.Email)
	input.Password = strings.TrimSpace(input.Password)
	input.UserID = strings.TrimSpace(input.UserID)
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	input.AvatarURL = strings.TrimSpace(input.AvatarURL)
	input.Status = strings.TrimSpace(input.Status)
	statusDate, errMessage := cleanDateOnlyOrToday(input.StatusDate, "status_date")
	if errMessage != "" {
		writeError(w, http.StatusBadRequest, errMessage)
		return
	}
	if errMessage := validateAdminProfileInput(input.UserID, input.DisplayName); errMessage != "" {
		writeError(w, http.StatusBadRequest, errMessage)
		return
	}
	if !strings.Contains(input.Email, "@") {
		writeError(w, http.StatusBadRequest, "email is required")
		return
	}
	if len(input.Password) < 6 {
		writeError(w, http.StatusBadRequest, "password must be at least 6 characters")
		return
	}
	if input.Status == "" {
		input.Status = contracts.DailyStatusUnselected
	}
	cleanStatus, err := dailystatuses.CleanStatus(input.Status)
	if err != nil {
		writeError(w, http.StatusBadRequest, "status is invalid")
		return
	}
	input.Status = string(cleanStatus)

	authPayload := map[string]any{
		"email":         input.Email,
		"password":      input.Password,
		"email_confirm": true,
		"user_metadata": map[string]any{
			"display_name": input.DisplayName,
			"user_id":      input.UserID,
		},
	}
	var authResp map[string]any
	if err := r.deps.AdminSupabase.AdminCreateUser(req.Context(), authPayload, &authResp); err != nil {
		writeSupabaseError(w, err)
		return
	}
	createdUserID := authResponseUserID(authResp)
	if createdUserID == "" {
		writeError(w, http.StatusBadGateway, "auth user creation returned no id")
		return
	}

	profilePayload := map[string]any{
		"id":            createdUserID,
		"user_id":       input.UserID,
		"display_name":  input.DisplayName,
		"character_key": "avatar",
		"avatar_url":    input.AvatarURL,
		"is_plus":       input.IsPlus,
	}
	q := url.Values{}
	q.Set("on_conflict", "id")
	var profiles []map[string]any
	if err := r.deps.AdminSupabase.Upsert(req.Context(), r.deps.Config.SupabaseServiceRoleKey, "profiles", q, profilePayload, &profiles); err != nil {
		_ = r.deps.AdminSupabase.AdminDeleteUser(req.Context(), createdUserID)
		writeSupabaseError(w, err)
		return
	}
	if err := r.upsertAdminDailyStatus(req.Context(), createdUserID, statusDate, input.Status); err != nil {
		_ = r.deps.AdminSupabase.AdminDeleteUser(req.Context(), createdUserID)
		writeSupabaseError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, firstMap(profiles, profilePayload))
}

func (r *router) adminUpdateUser(w http.ResponseWriter, req *http.Request, _ AuthUser) {
	targetID, errMessage := cleanUUID(req.PathValue("id"), "user id")
	if errMessage != "" {
		writeError(w, http.StatusBadRequest, errMessage)
		return
	}

	var input AdminUpdateUserRequest
	if !decodeJSONBody(w, req, &input) {
		return
	}

	authPayload := map[string]any{}
	userMeta := map[string]any{}
	if input.Email != nil {
		email := strings.TrimSpace(*input.Email)
		if email == "" || !strings.Contains(email, "@") {
			writeError(w, http.StatusBadRequest, "email is invalid")
			return
		}
		authPayload["email"] = email
		authPayload["email_confirm"] = true
	}
	if input.Password != nil {
		password := strings.TrimSpace(*input.Password)
		if password != "" {
			if len(password) < 6 {
				writeError(w, http.StatusBadRequest, "password must be at least 6 characters")
				return
			}
			authPayload["password"] = password
		}
	}

	profilePayload := map[string]any{}
	if input.UserID != nil {
		userID := strings.TrimSpace(*input.UserID)
		if errMessage := validateAdminProfileInput(userID, "ok"); errMessage != "" {
			writeError(w, http.StatusBadRequest, errMessage)
			return
		}
		profilePayload["user_id"] = userID
		userMeta["user_id"] = userID
	}
	if input.DisplayName != nil {
		displayName := strings.TrimSpace(*input.DisplayName)
		if errMessage := validateAdminProfileInput("valid_id", displayName); errMessage != "" {
			writeError(w, http.StatusBadRequest, errMessage)
			return
		}
		profilePayload["display_name"] = displayName
		userMeta["display_name"] = displayName
	}
	if input.AvatarURL != nil {
		profilePayload["avatar_url"] = strings.TrimSpace(*input.AvatarURL)
	}
	if input.IsPlus != nil {
		profilePayload["is_plus"] = *input.IsPlus
	}
	if input.Status != nil {
		status := strings.TrimSpace(*input.Status)
		if status == "" {
			writeError(w, http.StatusBadRequest, "status is required")
			return
		}
		cleanStatus, err := dailystatuses.CleanStatus(status)
		if err != nil {
			writeError(w, http.StatusBadRequest, "status is invalid")
			return
		}
		status = string(cleanStatus)
		statusDateInput := ""
		if input.StatusDate != nil {
			statusDateInput = *input.StatusDate
		}
		statusDate, errMessage := cleanDateOnlyOrToday(statusDateInput, "status_date")
		if errMessage != "" {
			writeError(w, http.StatusBadRequest, errMessage)
			return
		}
		if err := r.upsertAdminDailyStatus(req.Context(), targetID, statusDate, status); err != nil {
			writeSupabaseError(w, err)
			return
		}
	}
	if len(userMeta) > 0 {
		authPayload["user_metadata"] = userMeta
	}

	if len(authPayload) > 0 {
		var ignored map[string]any
		if err := r.deps.AdminSupabase.AdminUpdateUser(req.Context(), targetID, authPayload, &ignored); err != nil {
			writeSupabaseError(w, err)
			return
		}
	}

	if len(profilePayload) > 0 {
		q := url.Values{}
		q.Set("id", "eq."+targetID)
		var rows []map[string]any
		if err := r.deps.AdminSupabase.Patch(req.Context(), r.deps.Config.SupabaseServiceRoleKey, "profiles", q, profilePayload, &rows); err != nil {
			writeSupabaseError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, rows)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"id": targetID})
}

func (r *router) adminDeleteUser(w http.ResponseWriter, req *http.Request, adminUser AuthUser) {
	targetID, errMessage := cleanUUID(req.PathValue("id"), "user id")
	if errMessage != "" {
		writeError(w, http.StatusBadRequest, errMessage)
		return
	}
	if targetID == adminUser.ID {
		writeError(w, http.StatusBadRequest, "cannot delete the signed-in admin user")
		return
	}
	if err := r.deps.AdminSupabase.AdminDeleteUser(req.Context(), targetID); err != nil {
		writeSupabaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": targetID})
}

func (r *router) adminListYurubos(w http.ResponseWriter, req *http.Request, _ AuthUser) {
	status, errMessage := cleanAdminYuruboStatus(req.URL.Query().Get("status"), true)
	if errMessage != "" {
		writeError(w, http.StatusBadRequest, errMessage)
		return
	}
	limit := yurubos.CleanLimit(req.URL.Query().Get("limit"), 80)
	q := url.Values{}
	q.Set("select", "id,wish_item_id,owner_user_id,title,body,category,place_text,place_lat,place_lng,time_label,starts_at,ends_at,status,visibility,expires_at,created_at,updated_at,owner:profiles!yurubos_owner_user_id_fkey(id,user_id,display_name,avatar_url,is_plus)")
	q.Set("order", "created_at.desc")
	q.Set("limit", strconv.Itoa(limit))
	if status != contracts.QueryStatusAll {
		q.Set("status", supabase.PostgRESTEq(status))
	}
	var rows []map[string]any
	if err := r.deps.AdminSupabase.Get(req.Context(), r.deps.Config.SupabaseServiceRoleKey, "yurubos", q, &rows); err != nil {
		writeSupabaseError(w, err)
		return
	}
	r.addAdminYuruboReactionCounts(req.Context(), rows)
	writeJSON(w, http.StatusOK, rows)
}

func (r *router) adminCreateYurubo(w http.ResponseWriter, req *http.Request, _ AuthUser) {
	var input AdminCreateYuruboRequest
	if !decodeJSONBody(w, req, &input) {
		return
	}
	ownerID, errMessage := cleanUUID(input.OwnerUserID, "owner_user_id")
	if errMessage != "" {
		writeError(w, http.StatusBadRequest, errMessage)
		return
	}
	title, err := yurubos.CleanTitle(input.Title)
	if err != nil {
		writeYurubosError(w, err)
		return
	}
	status, errMessage := cleanAdminYuruboStatus(input.Status, false)
	if errMessage != "" {
		writeError(w, http.StatusBadRequest, errMessage)
		return
	}
	visibility, errMessage := cleanAdminYuruboVisibility(input.Visibility)
	if errMessage != "" {
		writeError(w, http.StatusBadRequest, errMessage)
		return
	}
	payload := map[string]any{
		"owner_user_id": ownerID,
		"title":         title,
		"body":          strings.TrimSpace(input.Body),
		"category":      yurubos.CleanCategory(input.Category),
		"place_text":    strings.TrimSpace(input.PlaceText),
		"time_label":    strings.TrimSpace(input.TimeLabel),
		"status":        status,
		"visibility":    visibility,
	}
	if normalized, ok, err := yurubos.NormalizeStartsAt(input.StartsAt); err != nil {
		writeYurubosError(w, err)
		return
	} else if ok {
		payload["starts_at"] = normalized
	}
	var rows []map[string]any
	if err := r.deps.AdminSupabase.Post(req.Context(), r.deps.Config.SupabaseServiceRoleKey, "yurubos", nil, payload, &rows); err != nil {
		writeSupabaseError(w, err)
		return
	}
	if len(rows) == 0 {
		writeError(w, http.StatusBadGateway, "yurubo insert returned no rows")
		return
	}
	writeJSON(w, http.StatusCreated, rows[0])
}

func (r *router) adminUpdateYurubo(w http.ResponseWriter, req *http.Request, _ AuthUser) {
	yuruboID, errMessage := cleanUUID(req.PathValue("id"), "yurubo id")
	if errMessage != "" {
		writeError(w, http.StatusBadRequest, errMessage)
		return
	}
	var input AdminUpdateYuruboRequest
	if !decodeJSONBody(w, req, &input) {
		return
	}
	payload := map[string]any{}
	if input.OwnerUserID != nil {
		ownerID, errMessage := cleanUUID(*input.OwnerUserID, "owner_user_id")
		if errMessage != "" {
			writeError(w, http.StatusBadRequest, errMessage)
			return
		}
		payload["owner_user_id"] = ownerID
	}
	if input.Title != nil {
		title, err := yurubos.CleanTitle(*input.Title)
		if err != nil {
			writeYurubosError(w, err)
			return
		}
		payload["title"] = title
	}
	if input.Body != nil {
		payload["body"] = strings.TrimSpace(*input.Body)
	}
	if input.Category != nil {
		payload["category"] = yurubos.CleanCategory(*input.Category)
	}
	if input.PlaceText != nil {
		payload["place_text"] = strings.TrimSpace(*input.PlaceText)
	}
	if input.TimeLabel != nil {
		payload["time_label"] = strings.TrimSpace(*input.TimeLabel)
	}
	if input.StartsAt != nil {
		if normalized, ok, err := yurubos.NormalizeStartsAt(*input.StartsAt); err != nil {
			writeYurubosError(w, err)
			return
		} else if ok {
			payload["starts_at"] = normalized
		} else {
			payload["starts_at"] = nil
		}
	}
	if input.Status != nil {
		status, errMessage := cleanAdminYuruboStatus(*input.Status, false)
		if errMessage != "" {
			writeError(w, http.StatusBadRequest, errMessage)
			return
		}
		payload["status"] = status
	}
	if input.Visibility != nil {
		visibility, errMessage := cleanAdminYuruboVisibility(*input.Visibility)
		if errMessage != "" {
			writeError(w, http.StatusBadRequest, errMessage)
			return
		}
		payload["visibility"] = visibility
	}
	if len(payload) == 0 {
		writeJSON(w, http.StatusOK, map[string]string{"id": yuruboID})
		return
	}
	q := url.Values{}
	q.Set("id", "eq."+yuruboID)
	var rows []map[string]any
	if err := r.deps.AdminSupabase.Patch(req.Context(), r.deps.Config.SupabaseServiceRoleKey, "yurubos", q, payload, &rows); err != nil {
		writeSupabaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (r *router) adminDeleteYurubo(w http.ResponseWriter, req *http.Request, _ AuthUser) {
	yuruboID, errMessage := cleanUUID(req.PathValue("id"), "yurubo id")
	if errMessage != "" {
		writeError(w, http.StatusBadRequest, errMessage)
		return
	}
	q := url.Values{}
	q.Set("id", "eq."+yuruboID)
	var rows []map[string]any
	if err := r.deps.AdminSupabase.Delete(req.Context(), r.deps.Config.SupabaseServiceRoleKey, "yurubos", q, &rows); err != nil {
		writeSupabaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (r *router) adminListMemories(w http.ResponseWriter, req *http.Request, _ AuthUser) {
	q := url.Values{}
	q.Set("select", "id,owner_user_id,happened_at,place_name,place_lat,place_lng,memo,link_url,is_official,created_at,owner:profiles!memories_owner_user_id_fkey(id,user_id,display_name,avatar_url,is_plus)")
	q.Set("order", "created_at.desc")
	q.Set("limit", "80")
	var rows []map[string]any
	if err := r.deps.AdminSupabase.Get(req.Context(), r.deps.Config.SupabaseServiceRoleKey, "memories", q, &rows); err != nil {
		writeSupabaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (r *router) adminListMemoryReports(w http.ResponseWriter, req *http.Request, _ AuthUser) {
	status := strings.TrimSpace(req.URL.Query().Get("status"))
	rows, err := r.adminMemoryReports(req, true, status)
	if err != nil {
		if status != "" && status != contracts.QueryStatusAll {
			writeSupabaseError(w, err)
			return
		}
		rows, err = r.adminMemoryReports(req, false, status)
	}
	if err != nil {
		writeSupabaseError(w, err)
		return
	}
	for _, row := range rows {
		if _, ok := row["status"].(string); !ok {
			row["status"] = contracts.StatusPending
		}
		if _, ok := row["hidden"].(bool); !ok {
			row["hidden"] = true
		}
	}
	writeJSON(w, http.StatusOK, rows)
}

func (r *router) adminMemoryReports(req *http.Request, includeModerationColumns bool, status string) ([]map[string]any, error) {
	selectColumns := "id,memory_id,reporter_user_id,reason,created_at,memory:memories!memory_reports_memory_id_fkey(id,owner_user_id,happened_at,memo,is_official,owner:profiles!memories_owner_user_id_fkey(id,user_id,display_name,avatar_url)),reporter:profiles!memory_reports_reporter_user_id_fkey(id,user_id,display_name,avatar_url)"
	if includeModerationColumns {
		selectColumns = "id,memory_id,reporter_user_id,reason,status,hidden_at,reviewed_at,reviewed_by_user_id,moderation_note,created_at,memory:memories!memory_reports_memory_id_fkey(id,owner_user_id,happened_at,memo,is_official,owner:profiles!memories_owner_user_id_fkey(id,user_id,display_name,avatar_url)),reporter:profiles!memory_reports_reporter_user_id_fkey(id,user_id,display_name,avatar_url)"
	}
	q := url.Values{}
	q.Set("select", selectColumns)
	q.Set("order", "created_at.desc")
	q.Set("limit", "100")
	if includeModerationColumns && status != "" && status != contracts.QueryStatusAll {
		q.Set("status", supabase.PostgRESTEq(status))
	}
	var rows []map[string]any
	if err := r.deps.AdminSupabase.Get(req.Context(), r.deps.Config.SupabaseServiceRoleKey, "memory_reports", q, &rows); err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *router) adminUpdateMemoryReport(w http.ResponseWriter, req *http.Request, adminUser AuthUser) {
	reportID, errMessage := cleanUUID(req.PathValue("id"), "report id")
	if errMessage != "" {
		writeError(w, http.StatusBadRequest, errMessage)
		return
	}
	var input AdminUpdateMemoryReportRequest
	if !decodeJSONBody(w, req, &input) {
		return
	}
	status, err := memories.CleanModerationStatus(input.Status)
	if err != nil {
		writeMemoryError(w, err)
		return
	}
	payload := map[string]any{
		"status":              string(status),
		"reviewed_at":         time.Now().UTC().Format(time.RFC3339),
		"reviewed_by_user_id": adminUser.ID,
		"moderation_note":     shortText(input.ModerationNote, 500),
	}
	q := url.Values{}
	q.Set("id", "eq."+reportID)
	var rows []map[string]any
	if err := r.deps.AdminSupabase.Patch(req.Context(), r.deps.Config.SupabaseServiceRoleKey, "memory_reports", q, payload, &rows); err != nil {
		writeSupabaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (r *router) adminCreateMemory(w http.ResponseWriter, req *http.Request, _ AuthUser) {
	var input AdminCreateMemoryRequest
	if !decodeJSONBody(w, req, &input) {
		return
	}
	input.OwnerUserID = strings.TrimSpace(input.OwnerUserID)
	if input.IsOfficial {
		officialProfileID, err := r.ensureOfficialProfile(req)
		if err != nil {
			writeSupabaseError(w, err)
			return
		}
		input.OwnerUserID = officialProfileID
	}
	if input.OwnerUserID == "" {
		writeError(w, http.StatusBadRequest, "owner_user_id is required")
		return
	}
	if !input.IsOfficial {
		ownerID, errMessage := cleanUUID(input.OwnerUserID, "owner_user_id")
		if errMessage != "" {
			writeError(w, http.StatusBadRequest, errMessage)
			return
		}
		input.OwnerUserID = ownerID
	}
	friendIDs, errMessage := cleanUUIDs(input.FriendIDs, "friend id")
	if errMessage != "" {
		writeError(w, http.StatusBadRequest, errMessage)
		return
	}
	happenedAt := input.HappenedAt
	if happenedAt.IsZero() {
		happenedAt = time.Now()
	}
	payload := map[string]any{
		"owner_user_id": input.OwnerUserID,
		"happened_at":   happenedAt.Format(time.RFC3339),
		"place_name":    strings.TrimSpace(input.PlaceName),
		"memo":          strings.TrimSpace(input.Memo),
		"link_url":      strings.TrimSpace(input.LinkURL),
		"is_official":   input.IsOfficial,
	}
	var rows []Memory
	if err := r.deps.AdminSupabase.Post(req.Context(), r.deps.Config.SupabaseServiceRoleKey, "memories", nil, payload, &rows); err != nil {
		writeSupabaseError(w, err)
		return
	}
	if len(rows) == 0 {
		writeError(w, http.StatusBadGateway, "memory insert returned no rows")
		return
	}
	if err := r.adminInsertMemoryFriends(req, rows[0].ID, friendIDs); err != nil {
		writeSupabaseError(w, err)
		return
	}
	r.createMemoryTaggedNotifications(req, r.deps.Config.SupabaseServiceRoleKey, rows[0].ID, input.OwnerUserID, friendIDs)
	writeJSON(w, http.StatusCreated, rows[0])
}

func (r *router) adminUpdateMemory(w http.ResponseWriter, req *http.Request, _ AuthUser) {
	memoryID, errMessage := cleanUUID(req.PathValue("id"), "memory id")
	if errMessage != "" {
		writeError(w, http.StatusBadRequest, errMessage)
		return
	}
	var input AdminUpdateMemoryRequest
	if !decodeJSONBody(w, req, &input) {
		return
	}
	payload := map[string]any{}
	if input.OwnerUserID != nil {
		ownerID, errMessage := cleanUUID(*input.OwnerUserID, "owner_user_id")
		if errMessage != "" {
			writeError(w, http.StatusBadRequest, errMessage)
			return
		}
		payload["owner_user_id"] = ownerID
	}
	if input.HappenedAt != nil {
		payload["happened_at"] = input.HappenedAt.Format(time.RFC3339)
	}
	if input.PlaceName != nil {
		payload["place_name"] = strings.TrimSpace(*input.PlaceName)
	}
	if input.Memo != nil {
		payload["memo"] = strings.TrimSpace(*input.Memo)
	}
	if input.LinkURL != nil {
		payload["link_url"] = strings.TrimSpace(*input.LinkURL)
	}
	if input.IsOfficial != nil {
		payload["is_official"] = *input.IsOfficial
		if *input.IsOfficial && input.OwnerUserID == nil {
			officialProfileID, err := r.ensureOfficialProfile(req)
			if err != nil {
				writeSupabaseError(w, err)
				return
			}
			payload["owner_user_id"] = officialProfileID
		}
	}
	if len(payload) == 0 {
		writeJSON(w, http.StatusOK, map[string]string{"id": memoryID})
		return
	}
	q := url.Values{}
	q.Set("id", "eq."+memoryID)
	var rows []Memory
	if err := r.deps.AdminSupabase.Patch(req.Context(), r.deps.Config.SupabaseServiceRoleKey, "memories", q, payload, &rows); err != nil {
		writeSupabaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (r *router) ensureOfficialProfile(req *http.Request) (string, error) {
	q := url.Values{}
	q.Set("select", "id,user_id,display_name,character_key,avatar_url,is_plus")
	q.Set("user_id", "eq."+officialProfileUserID)
	q.Set("limit", "1")
	var profiles []Profile
	if err := r.deps.AdminSupabase.Get(req.Context(), r.deps.Config.SupabaseServiceRoleKey, "profiles", q, &profiles); err != nil {
		return "", err
	}
	if len(profiles) > 0 && profiles[0].ID != "" {
		return profiles[0].ID, nil
	}

	password, err := randomAdminPassword()
	if err != nil {
		return "", err
	}
	authPayload := map[string]any{
		"email":         officialProfileEmail,
		"password":      password,
		"email_confirm": true,
		"user_metadata": map[string]any{
			"display_name": officialProfileDisplayName,
			"user_id":      officialProfileUserID,
		},
	}
	var authResp map[string]any
	if err := r.deps.AdminSupabase.AdminCreateUser(req.Context(), authPayload, &authResp); err != nil {
		return "", err
	}
	createdUserID := authResponseUserID(authResp)
	if createdUserID == "" {
		return "", errors.New("official auth user creation returned no id")
	}

	profilePayload := map[string]any{
		"id":            createdUserID,
		"user_id":       officialProfileUserID,
		"display_name":  officialProfileDisplayName,
		"character_key": "avatar",
		"avatar_url":    "",
		"is_plus":       true,
	}
	upsertQ := url.Values{}
	upsertQ.Set("on_conflict", "id")
	var createdProfiles []Profile
	if err := r.deps.AdminSupabase.Upsert(req.Context(), r.deps.Config.SupabaseServiceRoleKey, "profiles", upsertQ, profilePayload, &createdProfiles); err != nil {
		_ = r.deps.AdminSupabase.AdminDeleteUser(req.Context(), createdUserID)
		return "", err
	}
	return createdUserID, nil
}

func randomAdminPassword() (string, error) {
	var bytes [24]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", err
	}
	return "Ohey!" + hex.EncodeToString(bytes[:]), nil
}

func (r *router) adminDeleteMemory(w http.ResponseWriter, req *http.Request, _ AuthUser) {
	memoryID, errMessage := cleanUUID(req.PathValue("id"), "memory id")
	if errMessage != "" {
		writeError(w, http.StatusBadRequest, errMessage)
		return
	}
	q := url.Values{}
	q.Set("id", "eq."+memoryID)
	var rows []Memory
	if err := r.deps.AdminSupabase.Delete(req.Context(), r.deps.Config.SupabaseServiceRoleKey, "memories", q, &rows); err != nil {
		writeSupabaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (r *router) admin(next func(http.ResponseWriter, *http.Request, AuthUser)) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if r.deps.AdminSupabase == nil || r.deps.Config.SupabaseServiceRoleKey == "" {
			writeError(w, http.StatusServiceUnavailable, "admin backend is not configured")
			return
		}
		token, ok := bearerTokenFromRequest(req)
		if !ok {
			writeError(w, http.StatusUnauthorized, "missing Bearer token")
			return
		}
		authUser, err := r.verifyAuthToken(req.Context(), token)
		if err != nil {
			writeAuthVerificationError(w, err)
			return
		}
		authUserID, errMessage := cleanUUID(authUser.ID, "auth user id")
		if errMessage != "" {
			writeError(w, http.StatusUnauthorized, "invalid auth user")
			return
		}
		if headerUserID := strings.TrimSpace(req.Header.Get("X-Ohey-User-ID")); headerUserID != "" {
			cleanHeaderUserID, errMessage := cleanUUID(headerUserID, "X-Ohey-User-ID")
			if errMessage != "" {
				writeError(w, http.StatusBadRequest, errMessage)
				return
			}
			if cleanHeaderUserID != authUserID {
				writeError(w, http.StatusForbidden, "auth user mismatch")
				return
			}
		}
		authUser.ID = authUserID
		if !r.isAdminUser(authUser) {
			writeError(w, http.StatusForbidden, "admin access required")
			return
		}
		next(w, req, authUser)
	}
}

func (r *router) adminStatusesForDate(ctx context.Context, rows []map[string]any, statusDate string) (map[string]string, error) {
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		id, _ := row["id"].(string)
		if id != "" {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return map[string]string{}, nil
	}
	sort.Strings(ids)
	statusQuery := url.Values{}
	statusQuery.Set("select", "user_id,status")
	statusQuery.Set("user_id", "in.("+strings.Join(ids, ",")+")")
	statusQuery.Set("status_date", "eq."+statusDate)
	var statuses []map[string]any
	if err := r.deps.AdminSupabase.Get(ctx, r.deps.Config.SupabaseServiceRoleKey, "daily_statuses", statusQuery, &statuses); err != nil {
		return nil, err
	}
	m := map[string]string{}
	for _, status := range statuses {
		userID, _ := status["user_id"].(string)
		statusKey, _ := status["status"].(string)
		if userID != "" && strings.TrimSpace(statusKey) != "" {
			m[userID] = statusKey
		}
	}
	return m, nil
}

func (r *router) upsertAdminDailyStatus(ctx context.Context, targetUserID, statusDate, status string) error {
	q := url.Values{}
	q.Set("on_conflict", "user_id,status_date")
	payload := map[string]any{
		"user_id":     targetUserID,
		"status_date": statusDate,
		"status":      status,
	}
	var rows []map[string]any
	return r.deps.AdminSupabase.Upsert(ctx, r.deps.Config.SupabaseServiceRoleKey, "daily_statuses", q, payload, &rows)
}

func (r *router) isAdminUser(user AuthUser) bool {
	userEmail := strings.TrimSpace(user.Email)
	if userEmail == "" {
		return false
	}
	for _, adminEmail := range r.deps.Config.AdminEmails {
		if strings.EqualFold(userEmail, adminEmail) {
			return true
		}
	}
	return false
}

func (r *router) adminInsertMemoryFriends(req *http.Request, memoryID string, friendIDs []string) error {
	links := memoryFriendLinks(memoryID, friendIDs)
	if len(links) == 0 {
		return nil
	}
	var ignored []map[string]any
	return r.deps.AdminSupabase.Post(req.Context(), r.deps.Config.SupabaseServiceRoleKey, "memory_tagged_users", nil, links, &ignored)
}

func (r *router) addAdminYuruboReactionCounts(ctx context.Context, rows []map[string]any) {
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		if id, _ := row["id"].(string); id != "" {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return
	}
	q := url.Values{}
	q.Set("select", "yurubo_id,reaction_type")
	q.Set("yurubo_id", "in.("+strings.Join(ids, ",")+")")
	var reactions []map[string]any
	if err := r.deps.AdminSupabase.Get(ctx, r.deps.Config.SupabaseServiceRoleKey, "yurubo_reactions", q, &reactions); err != nil {
		return
	}
	counts := map[string]int{}
	for _, reaction := range reactions {
		id, _ := reaction["yurubo_id"].(string)
		if id == "" {
			continue
		}
		if reactionType, _ := reaction["reaction_type"].(string); reactionType == contracts.ReactionTypeAvailable {
			counts[id]++
		}
	}
	for _, row := range rows {
		if id, _ := row["id"].(string); id != "" {
			row["reaction_count"] = counts[id]
		}
	}
}

func cleanAdminYuruboStatus(value string, allowAll bool) (string, string) {
	status := strings.TrimSpace(value)
	if status == "" {
		if allowAll {
			return contracts.StatusOpen, ""
		}
		return contracts.StatusOpen, ""
	}
	if allowAll && status == contracts.QueryStatusAll {
		return status, ""
	}
	switch status {
	case contracts.StatusOpen, "closed", "expired", contracts.StatusCancelled, "scheduled":
		return status, ""
	default:
		return "", "status is invalid"
	}
}

func cleanAdminYuruboVisibility(value string) (string, string) {
	visibility := strings.TrimSpace(value)
	if visibility == "" {
		return contracts.VisibilityFriends, ""
	}
	switch visibility {
	case contracts.VisibilityFriends, contracts.VisibilityPrivate:
		return visibility, ""
	default:
		return "", "visibility must be friends or private"
	}
}

func validateAdminProfileInput(userID, displayName string) string {
	if _, err := profiles.CleanUserID(userID); err != nil {
		return err.Error()
	}
	if _, err := profiles.CleanDisplayName(displayName); err != nil {
		return err.Error()
	}
	return ""
}

func authResponseUserID(response map[string]any) string {
	if id, ok := response["id"].(string); ok {
		return id
	}
	if user, ok := response["user"].(map[string]any); ok {
		if id, ok := user["id"].(string); ok {
			return id
		}
	}
	return ""
}

func firstMap(rows []map[string]any, fallback map[string]any) map[string]any {
	if len(rows) > 0 {
		return rows[0]
	}
	return fallback
}

func sanitizePostgRESTSearch(value string) string {
	value = shortText(value, maxSearchRunes)
	var builder strings.Builder
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.IsSpace(r) || r == '_' || r == '-' {
			builder.WriteRune(r)
		}
	}
	return strings.Join(strings.Fields(builder.String()), " ")
}
