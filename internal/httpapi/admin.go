package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode"

	"github.com/yota/ohey/backend/internal/contracts"
	"github.com/yota/ohey/backend/internal/features/dailystatuses"
	"github.com/yota/ohey/backend/internal/features/profiles"
	"github.com/yota/ohey/backend/internal/features/yurubos"
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

	rows, err := r.adminListPostgresUsers(req.Context(), req.URL.Query().Get("search"), statusDate)
	if err != nil {
		writeError(w, http.StatusBadGateway, "database error")
		return
	}
	writeJSON(w, http.StatusOK, rows)
	return
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

	if r.deps.Postgres == nil || r.deps.ClerkAPI == nil || !r.deps.ClerkAPI.configured() {
		writeError(w, http.StatusServiceUnavailable, "admin backend is not configured")
		return
	}
	created, err := r.deps.ClerkAPI.CreateUser(req.Context(), input.Email, input.Password, input.UserID, input.DisplayName, input.AvatarURL)
	if err != nil {
		writeError(w, http.StatusBadGateway, "auth provider error")
		return
	}
	clerkUserID := stringValue(created, "id")
	row, err := scanAdminProfile(r.deps.Postgres.Pool().QueryRow(req.Context(), `insert into profiles (clerk_user_id,user_id,display_name,character_key,avatar_url,is_plus,updated_at) values ($1,$2,$3,'avatar',$4,$5,now()) returning id::text,user_id,display_name,character_key,avatar_url,is_plus,created_at,updated_at`, clerkUserID, input.UserID, input.DisplayName, input.AvatarURL, input.IsPlus))
	if err != nil {
		_ = r.deps.ClerkAPI.DeleteUser(req.Context(), clerkUserID)
		writeError(w, http.StatusBadGateway, "database error")
		return
	}
	if err := r.upsertAdminDailyStatus(req.Context(), stringValue(row, "id"), statusDate, input.Status); err != nil {
		_ = r.deps.ClerkAPI.DeleteUser(req.Context(), clerkUserID)
		writeError(w, http.StatusBadGateway, "database error")
		return
	}
	row["status"] = input.Status
	writeJSON(w, http.StatusCreated, row)
	return
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
			writeUpstreamError(w, err)
			return
		}
	}
	if len(userMeta) > 0 {
		authPayload["user_metadata"] = userMeta
	}

	if input.Email != nil {
		writeError(w, http.StatusBadRequest, "email changes are not supported in Clerk admin yet")
		return
	}
	if r.deps.Postgres == nil {
		writeError(w, http.StatusServiceUnavailable, "admin backend is not configured")
		return
	}
	clerkUserID, err := r.postgresClerkUserIDForProfile(req.Context(), targetID)
	if err != nil {
		writeError(w, http.StatusBadGateway, "database error")
		return
	}
	if len(authPayload) > 0 {
		if r.deps.ClerkAPI == nil || !r.deps.ClerkAPI.configured() {
			writeError(w, http.StatusServiceUnavailable, "admin backend is not configured")
			return
		}
		clerkPayload := map[string]any{}
		if password, ok := authPayload["password"]; ok {
			clerkPayload["password"] = password
		}
		if len(userMeta) > 0 {
			clerkPayload["public_metadata"] = userMeta
		}
		if err := r.deps.ClerkAPI.UpdateUser(req.Context(), clerkUserID, clerkPayload); err != nil {
			writeError(w, http.StatusBadGateway, "auth provider error")
			return
		}
	}
	if len(profilePayload) > 0 {
		row, err := r.patchPostgresAdminProfile(req.Context(), targetID, profilePayload)
		if err != nil {
			writeError(w, http.StatusBadGateway, "database error")
			return
		}
		writeJSON(w, http.StatusOK, []map[string]any{row})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": targetID})
	return
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
	if r.deps.Postgres == nil {
		writeError(w, http.StatusServiceUnavailable, "admin backend is not configured")
		return
	}
	clerkUserID, err := r.postgresClerkUserIDForProfile(req.Context(), targetID)
	if err != nil {
		writeError(w, http.StatusBadGateway, "database error")
		return
	}
	if _, err := r.deps.Postgres.Pool().Exec(req.Context(), `delete from profiles where id=$1`, targetID); err != nil {
		writeError(w, http.StatusBadGateway, "database error")
		return
	}
	if clerkUserID != "" && r.deps.ClerkAPI != nil && r.deps.ClerkAPI.configured() {
		_ = r.deps.ClerkAPI.DeleteUser(req.Context(), clerkUserID)
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": targetID})
	return
}

func (r *router) adminListYurubos(w http.ResponseWriter, req *http.Request, _ AuthUser) {
	status, errMessage := cleanAdminYuruboStatus(req.URL.Query().Get("status"), true)
	if errMessage != "" {
		writeError(w, http.StatusBadRequest, errMessage)
		return
	}
	limit := yurubos.CleanLimit(req.URL.Query().Get("limit"), 80)
	rows, err := r.adminListPostgresYurubos(req.Context(), status, limit)
	if err != nil {
		writeError(w, http.StatusBadGateway, "database error")
		return
	}
	writeJSON(w, http.StatusOK, rows)
	return
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
	row, err := r.adminCreatePostgresYurubo(req.Context(), payload)
	if err != nil {
		writeError(w, http.StatusBadGateway, "database error")
		return
	}
	writeJSON(w, http.StatusCreated, row)
	return
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
	rows, err := r.adminUpdatePostgresYurubo(req.Context(), yuruboID, payload)
	if err != nil {
		writeError(w, http.StatusBadGateway, "database error")
		return
	}
	writeJSON(w, http.StatusOK, rows)
	return
}

func (r *router) adminDeleteYurubo(w http.ResponseWriter, req *http.Request, _ AuthUser) {
	yuruboID, errMessage := cleanUUID(req.PathValue("id"), "yurubo id")
	if errMessage != "" {
		writeError(w, http.StatusBadRequest, errMessage)
		return
	}
	rows, err := r.adminDeletePostgresYurubo(req.Context(), yuruboID)
	if err != nil {
		writeError(w, http.StatusBadGateway, "database error")
		return
	}
	writeJSON(w, http.StatusOK, rows)
	return
}

func (r *router) ensureOfficialProfile(req *http.Request) (string, error) {
	if r.deps.Postgres == nil {
		return "", errors.New("postgres pool is not configured")
	}
	var id string
	err := r.deps.Postgres.Pool().QueryRow(req.Context(), `insert into profiles (user_id,display_name,character_key,avatar_url,is_plus,updated_at) values ($1,$2,'avatar','',true,now()) on conflict (user_id) do update set display_name=excluded.display_name,is_plus=true,updated_at=now() returning id::text`, officialProfileUserID, officialProfileDisplayName).Scan(&id)
	return id, err
}

func randomAdminPassword() (string, error) {
	var bytes [24]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", err
	}
	return "Ohey!" + hex.EncodeToString(bytes[:]), nil
}

func (r *router) admin(next func(http.ResponseWriter, *http.Request, AuthUser)) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if r.deps.Postgres == nil {
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
		authUserID := ""
		if r.deps.Config.AuthProvider == "clerk" {
			profile, err := r.profileUsecase().GetProfile(req.Context(), profiles.AuthInput{AuthToken: token, ClerkUserID: strings.TrimSpace(authUser.ID)})
			if err != nil || profile == nil {
				writeProfileError(w, profiles.UserError{Kind: profiles.ErrorKindNotFound, Message: "profile not found"})
				return
			}
			authUserID = profile.ID
			req.Header.Set("X-Ohey-Clerk-User-ID", strings.TrimSpace(authUser.ID))
		} else {
			cleanID, errMessage := cleanUUID(authUser.ID, "auth user id")
			if errMessage != "" {
				writeError(w, http.StatusUnauthorized, "invalid auth user")
				return
			}
			authUserID = cleanID
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
		req.Header.Set("X-Ohey-User-ID", authUserID)
		if !r.isAdminUser(authUser) {
			writeError(w, http.StatusForbidden, "admin access required")
			return
		}
		next(w, req, authUser)
	}
}

type adminProfileRow interface {
	Scan(dest ...any) error
}

func scanAdminProfile(row adminProfileRow) (map[string]any, error) {
	var id, userID, displayName, characterKey string
	var avatarURL *string
	var isPlus bool
	var createdAt, updatedAt any
	if err := row.Scan(&id, &userID, &displayName, &characterKey, &avatarURL, &isPlus, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	out := map[string]any{"id": id, "user_id": userID, "display_name": displayName, "character_key": characterKey, "avatar_url": "", "is_plus": isPlus, "created_at": createdAt, "updated_at": updatedAt}
	if avatarURL != nil {
		out["avatar_url"] = *avatarURL
	}
	return out, nil
}

func (r *router) postgresClerkUserIDForProfile(ctx context.Context, profileID string) (string, error) {
	if r.deps.Postgres == nil {
		return "", errors.New("postgres pool is not configured")
	}
	var clerkUserID *string
	if err := r.deps.Postgres.Pool().QueryRow(ctx, `select clerk_user_id from profiles where id=$1`, profileID).Scan(&clerkUserID); err != nil {
		return "", err
	}
	if clerkUserID == nil {
		return "", nil
	}
	return *clerkUserID, nil
}

func (r *router) patchPostgresAdminProfile(ctx context.Context, profileID string, payload map[string]any) (map[string]any, error) {
	if r.deps.Postgres == nil {
		return nil, errors.New("postgres pool is not configured")
	}
	current, err := scanAdminProfile(r.deps.Postgres.Pool().QueryRow(ctx, `select id::text,user_id,display_name,character_key,avatar_url,is_plus,created_at,updated_at from profiles where id=$1`, profileID))
	if err != nil {
		return nil, err
	}
	userID := stringValue(current, "user_id")
	displayName := stringValue(current, "display_name")
	avatarURL := stringValue(current, "avatar_url")
	characterKey := stringValue(current, "character_key")
	isPlus, _ := current["is_plus"].(bool)
	if v, ok := payload["user_id"].(string); ok {
		userID = v
	}
	if v, ok := payload["display_name"].(string); ok {
		displayName = v
	}
	if v, ok := payload["avatar_url"].(string); ok {
		avatarURL = v
	}
	if v, ok := payload["character_key"].(string); ok {
		characterKey = v
	}
	if v, ok := payload["is_plus"].(bool); ok {
		isPlus = v
	}
	return scanAdminProfile(r.deps.Postgres.Pool().QueryRow(ctx, `update profiles set user_id=$2,display_name=$3,character_key=$4,avatar_url=$5,is_plus=$6,updated_at=now() where id=$1 returning id::text,user_id,display_name,character_key,avatar_url,is_plus,created_at,updated_at`, profileID, userID, displayName, characterKey, avatarURL, isPlus))
}

func (r *router) adminListPostgresUsers(ctx context.Context, search, statusDate string) ([]map[string]any, error) {
	if r.deps.Postgres == nil {
		return []map[string]any{}, nil
	}
	search = strings.TrimSpace(search)
	args := []any{statusDate, contracts.DailyStatusUnselected}
	where := ""
	if search != "" {
		args = append(args, "%"+search+"%")
		where = " where display_name ilike $3 or user_id ilike $3"
	}
	rows, err := r.deps.Postgres.Pool().Query(ctx, `select id::text,user_id,display_name,character_key,avatar_url,is_plus,created_at,updated_at,coalesce((select status from daily_statuses ds where ds.user_id=profiles.id and ds.status_date=$1),$2) from profiles`+where+` order by created_at desc limit 80`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, userID, displayName, characterKey, status string
		var avatarURL *string
		var isPlus bool
		var createdAt, updatedAt any
		if err := rows.Scan(&id, &userID, &displayName, &characterKey, &avatarURL, &isPlus, &createdAt, &updatedAt, &status); err != nil {
			return nil, err
		}
		row := map[string]any{"id": id, "user_id": userID, "display_name": displayName, "character_key": characterKey, "avatar_url": "", "is_plus": isPlus, "created_at": createdAt, "updated_at": updatedAt, "status": status}
		if avatarURL != nil {
			row["avatar_url"] = *avatarURL
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (r *router) adminStatusesForDate(ctx context.Context, rows []map[string]any, statusDate string) (map[string]string, error) {
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		if id, _ := row["id"].(string); id != "" {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 || r.deps.Postgres == nil {
		return map[string]string{}, nil
	}
	q, err := r.deps.Postgres.Pool().Query(ctx, `select user_id::text,status from daily_statuses where user_id=any($1::uuid[]) and status_date=$2`, ids, statusDate)
	if err != nil {
		return nil, err
	}
	defer q.Close()
	m := map[string]string{}
	for q.Next() {
		var userID, status string
		if err := q.Scan(&userID, &status); err != nil {
			return nil, err
		}
		m[userID] = status
	}
	return m, q.Err()
}

func (r *router) upsertAdminDailyStatus(ctx context.Context, targetUserID, statusDate, status string) error {
	if r.deps.Postgres == nil {
		return errors.New("postgres pool is not configured")
	}
	_, err := r.deps.Postgres.Pool().Exec(ctx, `insert into daily_statuses (user_id,status_date,status,updated_at) values ($1,$2,$3,now()) on conflict (user_id,status_date) do update set status=excluded.status, updated_at=now()`, targetUserID, statusDate, status)
	return err
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

func (r *router) addAdminYuruboReactionCounts(ctx context.Context, rows []map[string]any) {
	r.addPostgresAdminYuruboReactionCounts(ctx, rows)
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

func (r *router) adminListPostgresYurubos(ctx context.Context, status string, limit int) ([]map[string]any, error) {
	if r.deps.Postgres == nil {
		return []map[string]any{}, nil
	}
	where := ""
	args := []any{limit}
	if status != contracts.QueryStatusAll {
		where = " where y.status=$2"
		args = append(args, status)
	}
	rows, err := r.deps.Postgres.Pool().Query(ctx, adminYuruboSelectSQL()+where+` order by y.created_at desc limit $1`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		m, err := scanAdminYurubo(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (r *router) adminCreatePostgresYurubo(ctx context.Context, payload map[string]any) (map[string]any, error) {
	if r.deps.Postgres == nil {
		return nil, errors.New("postgres pool is not configured")
	}
	return scanAdminYurubo(r.deps.Postgres.Pool().QueryRow(ctx, adminYuruboMutationSQL(`insert into yurubos (owner_user_id,title,body,category,place_text,time_label,status,visibility,starts_at,updated_at) values ($1,$2,$3,$4,$5,$6,$7,$8,$9,now()) returning *`), payload["owner_user_id"], payload["title"], payload["body"], payload["category"], payload["place_text"], payload["time_label"], payload["status"], payload["visibility"], payload["starts_at"]))
}

func (r *router) adminUpdatePostgresYurubo(ctx context.Context, id string, payload map[string]any) ([]map[string]any, error) {
	if r.deps.Postgres == nil {
		return nil, errors.New("postgres pool is not configured")
	}
	current, err := scanAdminYurubo(r.deps.Postgres.Pool().QueryRow(ctx, adminYuruboSelectSQL()+` where y.id=$1`, id))
	if err != nil {
		return nil, err
	}
	owner := valueOrExistingString(payload, "owner_user_id", current)
	title := valueOrExistingString(payload, "title", current)
	body := valueOrExistingString(payload, "body", current)
	category := valueOrExistingString(payload, "category", current)
	place := valueOrExistingString(payload, "place_text", current)
	timeLabel := valueOrExistingString(payload, "time_label", current)
	status := valueOrExistingString(payload, "status", current)
	visibility := valueOrExistingString(payload, "visibility", current)
	var starts any
	if v, ok := payload["starts_at"]; ok {
		starts = v
	} else if v, ok := current["starts_at"]; ok {
		starts = v
	}
	row, err := scanAdminYurubo(r.deps.Postgres.Pool().QueryRow(ctx, adminYuruboMutationSQL(`update yurubos set owner_user_id=$2,title=$3,body=$4,category=$5,place_text=$6,time_label=$7,status=$8,visibility=$9,starts_at=$10,updated_at=now() where id=$1 returning *`), id, owner, title, body, category, place, timeLabel, status, visibility, starts))
	if err != nil {
		return nil, err
	}
	return []map[string]any{row}, nil
}

func (r *router) adminDeletePostgresYurubo(ctx context.Context, id string) ([]map[string]any, error) {
	if r.deps.Postgres == nil {
		return nil, errors.New("postgres pool is not configured")
	}
	row, err := scanAdminYurubo(r.deps.Postgres.Pool().QueryRow(ctx, adminYuruboMutationSQL(`delete from yurubos where id=$1 returning *`), id))
	if err != nil {
		return nil, err
	}
	return []map[string]any{row}, nil
}

func adminYuruboSelectSQL() string {
	return `select y.id::text,y.wish_item_id::text,y.owner_user_id::text,y.title,y.body,y.category,y.place_text,y.place_lat,y.place_lng,y.time_label,y.starts_at,y.ends_at,y.status,y.visibility,y.expires_at,y.created_at,y.updated_at,o.id::text,o.user_id,o.display_name,o.avatar_url,o.is_plus,(select count(*) from yurubo_reactions yr where yr.yurubo_id=y.id and yr.reaction_type='available') from yurubos y join profiles o on o.id=y.owner_user_id`
}

func adminYuruboMutationSQL(mutation string) string {
	return `with y as (` + mutation + `) select y.id::text,y.wish_item_id::text,y.owner_user_id::text,y.title,y.body,y.category,y.place_text,y.place_lat,y.place_lng,y.time_label,y.starts_at,y.ends_at,y.status,y.visibility,y.expires_at,y.created_at,y.updated_at,o.id::text,o.user_id,o.display_name,o.avatar_url,o.is_plus,(select count(*) from yurubo_reactions yr where yr.yurubo_id=y.id and yr.reaction_type='available') from y join profiles o on o.id=y.owner_user_id`
}

type rowScanner interface{ Scan(dest ...any) error }

func scanAdminYurubo(row rowScanner) (map[string]any, error) {
	var id, owner, title, body, category, place, timeLabel, status, visibility string
	var wishID, avatarURL *string
	var lat, lng *float64
	var starts, ends, expires *time.Time
	var created, updated time.Time
	var ownerID, ownerUserID, ownerName string
	var ownerPlus bool
	var reactionCount int
	if err := row.Scan(&id, &wishID, &owner, &title, &body, &category, &place, &lat, &lng, &timeLabel, &starts, &ends, &status, &visibility, &expires, &created, &updated, &ownerID, &ownerUserID, &ownerName, &avatarURL, &ownerPlus, &reactionCount); err != nil {
		return nil, err
	}
	m := map[string]any{"id": id, "owner_user_id": owner, "title": title, "body": body, "category": category, "place_text": place, "time_label": timeLabel, "status": status, "visibility": visibility, "created_at": created.UTC().Format(time.RFC3339Nano), "updated_at": updated.UTC().Format(time.RFC3339Nano), "reaction_count": reactionCount, "owner": map[string]any{"id": ownerID, "user_id": ownerUserID, "display_name": ownerName, "is_plus": ownerPlus}}
	if wishID != nil {
		m["wish_item_id"] = *wishID
	}
	if lat != nil {
		m["place_lat"] = *lat
	}
	if lng != nil {
		m["place_lng"] = *lng
	}
	if starts != nil {
		m["starts_at"] = starts.UTC().Format(time.RFC3339Nano)
	}
	if ends != nil {
		m["ends_at"] = ends.UTC().Format(time.RFC3339Nano)
	}
	if expires != nil {
		m["expires_at"] = expires.UTC().Format(time.RFC3339Nano)
	}
	if avatarURL != nil {
		m["owner"].(map[string]any)["avatar_url"] = *avatarURL
	}
	return m, nil
}

func valueOrExistingString(payload map[string]any, key string, current map[string]any) string {
	if v, ok := payload[key].(string); ok {
		return v
	}
	return stringValue(current, key)
}

func (r *router) addPostgresAdminYuruboReactionCounts(ctx context.Context, rows []map[string]any) {
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		if id, _ := row["id"].(string); id != "" {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 || r.deps.Postgres == nil {
		return
	}
	q, err := r.deps.Postgres.Pool().Query(ctx, `select yurubo_id::text,count(*) from yurubo_reactions where yurubo_id=any($1::uuid[]) and reaction_type=$2 group by yurubo_id`, ids, contracts.ReactionTypeAvailable)
	if err != nil {
		return
	}
	defer q.Close()
	counts := map[string]int{}
	for q.Next() {
		var id string
		var count int
		if q.Scan(&id, &count) == nil {
			counts[id] = count
		}
	}
	for _, row := range rows {
		if id, _ := row["id"].(string); id != "" {
			row["reaction_count"] = counts[id]
		}
	}
}
