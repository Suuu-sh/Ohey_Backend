package httpapi

import (
	"net"
	"net/http"
	"strings"
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
	if r.deps.ClerkAPI == nil || !r.deps.ClerkAPI.configured() {
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
	created, err := r.deps.ClerkAPI.CreateUser(req.Context(), email, password, userID, displayName, avatarURL)
	if err != nil {
		writeClerkSignupError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"user": created})
	return
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

func writeClerkSignupError(w http.ResponseWriter, err error) {
	body := strings.ToLower(err.Error())
	switch {
	case strings.Contains(body, "already") || strings.Contains(body, "duplicate") || strings.Contains(body, "taken"):
		writeError(w, http.StatusConflict, "このメールアドレスはすでに登録されています。")
	case strings.Contains(body, "password") || strings.Contains(body, "email"):
		writeError(w, http.StatusBadRequest, "登録情報を確認してください。")
	default:
		writeError(w, http.StatusBadGateway, "signup failed")
	}
}
