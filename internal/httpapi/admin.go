package httpapi

import (
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

const adminEmail = "yisshiki39@gmail.com"

var adminUserIDPattern = regexp.MustCompile(`^[A-Za-z0-9_]{3,24}$`)

func (r *router) adminMe(w http.ResponseWriter, req *http.Request, adminUser AuthUser) {
	writeJSON(w, http.StatusOK, map[string]any{
		"id":          adminUser.ID,
		"email":       adminUser.Email,
		"is_admin":    true,
		"environment": r.deps.Config.Environment,
	})
}

func (r *router) adminListUsers(w http.ResponseWriter, req *http.Request, _ AuthUser) {
	q := url.Values{}
	q.Set("select", "id,user_id,display_name,character_key,avatar_url,is_plus,created_at,updated_at")
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
	writeJSON(w, http.StatusOK, rows)
}

func (r *router) adminCreateUser(w http.ResponseWriter, req *http.Request, _ AuthUser) {
	var input AdminCreateUserRequest
	if err := json.NewDecoder(req.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	input.Email = strings.TrimSpace(input.Email)
	input.Password = strings.TrimSpace(input.Password)
	input.UserID = strings.TrimSpace(input.UserID)
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	input.AvatarURL = strings.TrimSpace(input.AvatarURL)
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
	writeJSON(w, http.StatusCreated, firstMap(profiles, profilePayload))
}

func (r *router) adminUpdateUser(w http.ResponseWriter, req *http.Request, _ AuthUser) {
	targetID := strings.TrimSpace(req.PathValue("id"))
	if targetID == "" {
		writeError(w, http.StatusBadRequest, "user id is required")
		return
	}

	var input AdminUpdateUserRequest
	if err := json.NewDecoder(req.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
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
	targetID := strings.TrimSpace(req.PathValue("id"))
	if targetID == "" {
		writeError(w, http.StatusBadRequest, "user id is required")
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

func (r *router) adminListDrinkLogs(w http.ResponseWriter, req *http.Request, _ AuthUser) {
	q := url.Values{}
	q.Set("select", "id,owner_user_id,drank_at,place_name,memo,photo_path,created_at,owner:profiles!drink_logs_owner_user_id_fkey(id,user_id,display_name,avatar_url,is_plus)")
	q.Set("order", "created_at.desc")
	q.Set("limit", "80")
	var rows []map[string]any
	if err := r.deps.AdminSupabase.Get(req.Context(), r.deps.Config.SupabaseServiceRoleKey, "drink_logs", q, &rows); err != nil {
		writeSupabaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (r *router) adminCreateDrinkLog(w http.ResponseWriter, req *http.Request, _ AuthUser) {
	var input AdminCreateDrinkLogRequest
	if err := json.NewDecoder(req.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	input.OwnerUserID = strings.TrimSpace(input.OwnerUserID)
	if input.OwnerUserID == "" {
		writeError(w, http.StatusBadRequest, "owner_user_id is required")
		return
	}
	drankAt := input.DrankAt
	if drankAt.IsZero() {
		drankAt = time.Now()
	}
	payload := map[string]any{
		"owner_user_id": input.OwnerUserID,
		"drank_at":      drankAt.Format(time.RFC3339),
		"place_name":    strings.TrimSpace(input.PlaceName),
		"memo":          strings.TrimSpace(input.Memo),
		"photo_path":    strings.TrimSpace(input.PhotoPath),
	}
	var rows []DrinkLog
	if err := r.deps.AdminSupabase.Post(req.Context(), r.deps.Config.SupabaseServiceRoleKey, "drink_logs", nil, payload, &rows); err != nil {
		writeSupabaseError(w, err)
		return
	}
	if len(rows) == 0 {
		writeError(w, http.StatusBadGateway, "drink log insert returned no rows")
		return
	}
	if err := r.adminInsertDrinkLogFriends(req, rows[0].ID, input.FriendIDs); err != nil {
		writeSupabaseError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, rows[0])
}

func (r *router) adminUpdateDrinkLog(w http.ResponseWriter, req *http.Request, _ AuthUser) {
	logID := strings.TrimSpace(req.PathValue("id"))
	if logID == "" {
		writeError(w, http.StatusBadRequest, "drink log id is required")
		return
	}
	var input AdminUpdateDrinkLogRequest
	if err := json.NewDecoder(req.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	payload := map[string]any{}
	if input.OwnerUserID != nil {
		ownerID := strings.TrimSpace(*input.OwnerUserID)
		if ownerID == "" {
			writeError(w, http.StatusBadRequest, "owner_user_id is required")
			return
		}
		payload["owner_user_id"] = ownerID
	}
	if input.DrankAt != nil {
		payload["drank_at"] = input.DrankAt.Format(time.RFC3339)
	}
	if input.PlaceName != nil {
		payload["place_name"] = strings.TrimSpace(*input.PlaceName)
	}
	if input.Memo != nil {
		payload["memo"] = strings.TrimSpace(*input.Memo)
	}
	if input.PhotoPath != nil {
		payload["photo_path"] = strings.TrimSpace(*input.PhotoPath)
	}
	if len(payload) == 0 {
		writeJSON(w, http.StatusOK, map[string]string{"id": logID})
		return
	}
	q := url.Values{}
	q.Set("id", "eq."+logID)
	var rows []DrinkLog
	if err := r.deps.AdminSupabase.Patch(req.Context(), r.deps.Config.SupabaseServiceRoleKey, "drink_logs", q, payload, &rows); err != nil {
		writeSupabaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (r *router) adminDeleteDrinkLog(w http.ResponseWriter, req *http.Request, _ AuthUser) {
	logID := strings.TrimSpace(req.PathValue("id"))
	if logID == "" {
		writeError(w, http.StatusBadRequest, "drink log id is required")
		return
	}
	q := url.Values{}
	q.Set("id", "eq."+logID)
	var rows []DrinkLog
	if err := r.deps.AdminSupabase.Delete(req.Context(), r.deps.Config.SupabaseServiceRoleKey, "drink_logs", q, &rows); err != nil {
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
		auth := req.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			writeError(w, http.StatusUnauthorized, "missing Bearer token")
			return
		}
		token := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
		if token == "" {
			writeError(w, http.StatusUnauthorized, "missing Bearer token")
			return
		}
		var authUser AuthUser
		if err := r.deps.Supabase.GetAuthUser(req.Context(), token, &authUser); err != nil {
			writeSupabaseError(w, err)
			return
		}
		if authUser.ID == "" {
			writeError(w, http.StatusUnauthorized, "invalid auth user")
			return
		}
		if headerUserID := strings.TrimSpace(req.Header.Get("X-Nomo-User-ID")); headerUserID != "" && headerUserID != authUser.ID {
			writeError(w, http.StatusForbidden, "auth user mismatch")
			return
		}
		if !r.isAdminUser(authUser) {
			writeError(w, http.StatusForbidden, "admin access required")
			return
		}
		next(w, req, authUser)
	}
}

func (r *router) isAdminUser(user AuthUser) bool {
	return strings.EqualFold(strings.TrimSpace(user.Email), adminEmail)
}

func (r *router) adminInsertDrinkLogFriends(req *http.Request, drinkLogID string, friendIDs []string) error {
	links := make([]map[string]string, 0, len(friendIDs))
	for _, id := range friendIDs {
		if trimmed := strings.TrimSpace(id); trimmed != "" {
			links = append(links, map[string]string{"drink_log_id": drinkLogID, "friend_user_id": trimmed})
		}
	}
	if len(links) == 0 {
		return nil
	}
	var ignored []map[string]any
	return r.deps.AdminSupabase.Post(req.Context(), r.deps.Config.SupabaseServiceRoleKey, "drink_log_friends", nil, links, &ignored)
}

func validateAdminProfileInput(userID, displayName string) string {
	if !adminUserIDPattern.MatchString(userID) {
		return "user_id must be 3-24 letters, numbers, or underscores"
	}
	nameLength := utf8.RuneCountInString(displayName)
	if nameLength < 1 || nameLength > 40 {
		return "display_name must be 1-40 characters"
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
	value = strings.TrimSpace(value)
	replacer := strings.NewReplacer("*", "", "(", "", ")", "", ",", "", "\"", "", "'", "")
	return replacer.Replace(value)
}
