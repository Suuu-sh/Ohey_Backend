package httpapi

import (
	"errors"
	"net"
	"net/http"
	"strings"

	"github.com/yota/ohey/backend/internal/supabase"
)

type SignupRequest struct {
	Email       string         `json:"email"`
	Password    string         `json:"password"`
	UserID      string         `json:"user_id"`
	DisplayName string         `json:"display_name"`
	AvatarURL   string         `json:"avatar_url"`
	Avatar      map[string]any `json:"avatar,omitempty"`
}

func (r *router) signupWithPassword(w http.ResponseWriter, req *http.Request) {
	if r.deps.AdminSupabase == nil || strings.TrimSpace(r.deps.Config.SupabaseServiceRoleKey) == "" {
		writeError(w, http.StatusServiceUnavailable, "signup is not configured")
		return
	}
	if !r.enforceSignupRateLimit(w, req) {
		return
	}

	var input SignupRequest
	if !decodeJSONBody(w, req, &input) {
		return
	}
	email := strings.TrimSpace(input.Email)
	password := input.Password
	userID := strings.TrimSpace(input.UserID)
	displayName := strings.TrimSpace(input.DisplayName)
	avatarURL := strings.TrimSpace(input.AvatarURL)
	if email == "" || len(password) < 6 {
		writeError(w, http.StatusBadRequest, "メールアドレスと6文字以上のパスワードを入力してください。")
		return
	}
	if userID == "" || displayName == "" {
		writeError(w, http.StatusBadRequest, "profile fields are required")
		return
	}

	body := map[string]any{
		"email":         email,
		"password":      password,
		"email_confirm": true,
		"user_metadata": map[string]any{
			"user_id":       userID,
			"display_name":  displayName,
			"character_key": "avatar",
			"avatar_url":    avatarURL,
		},
	}
	var created map[string]any
	if err := r.deps.AdminSupabase.AdminCreateUser(req.Context(), body, &created); err != nil {
		writeSupabaseAuthSignupError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"user": created})
}

func (r *router) enforceSignupRateLimit(w http.ResponseWriter, req *http.Request) bool {
	key := clientIP(req)
	allowed, retryAfter := r.rateLimiter.AllowKey(key, rateLimitAuthSignup)
	if allowed {
		return true
	}
	writeRateLimitExceeded(w, rateLimitAuthSignup, retryAfter)
	return false
}

func clientIP(req *http.Request) string {
	if forwarded := strings.TrimSpace(req.Header.Get("X-Forwarded-For")); forwarded != "" {
		if first, _, ok := strings.Cut(forwarded, ","); ok {
			return strings.TrimSpace(first)
		}
		return forwarded
	}
	host, _, err := net.SplitHostPort(req.RemoteAddr)
	if err == nil && host != "" {
		return host
	}
	return req.RemoteAddr
}

func writeSupabaseAuthSignupError(w http.ResponseWriter, err error) {
	var apiError supabase.APIError
	if errors.As(err, &apiError) {
		body := strings.ToLower(apiError.Body)
		switch {
		case apiError.StatusCode == http.StatusUnprocessableEntity && strings.Contains(body, "already"):
			writeError(w, http.StatusConflict, "このメールアドレスはすでに登録されています。")
		case apiError.StatusCode == http.StatusBadRequest:
			writeError(w, http.StatusBadRequest, "登録情報を確認してください。")
		default:
			writeError(w, http.StatusBadGateway, "signup failed")
		}
		return
	}
	writeError(w, http.StatusBadGateway, "signup failed")
}
