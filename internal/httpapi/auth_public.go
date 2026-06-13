package httpapi

import (
	"net"
	"net/http"
	"strings"

	"github.com/Suuu-sh/Ohey_Backend/internal/features/profiles"
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
	clerkUserID, _ := created["id"].(string)
	if strings.TrimSpace(clerkUserID) == "" {
		writeError(w, http.StatusBadGateway, "signup failed")
		return
	}
	profile, err := r.profileUsecase().BootstrapProfile(req.Context(), profiles.BootstrapUsecaseInput{
		ClerkUserID: clerkUserID,
		Request: profiles.BootstrapRequest{
			UserID:       userID,
			DisplayName:  displayName,
			CharacterKey: "",
			AvatarURL:    avatarURL,
		},
	})
	if err != nil {
		_ = r.deps.ClerkAPI.DeleteUser(req.Context(), clerkUserID)
		writeProfileError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"user": created, "profile": profile})
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
	case strings.Contains(body, "password"):
		writeError(w, http.StatusBadRequest, friendlyClerkPasswordError(body))
	case strings.Contains(body, "email"):
		writeError(w, http.StatusBadRequest, "メールアドレスを確認してください。")
	default:
		writeError(w, http.StatusBadGateway, "signup failed")
	}
}

func friendlyClerkPasswordError(lowerBody string) string {
	switch {
	case strings.Contains(lowerBody, "pwned"),
		strings.Contains(lowerBody, "breach"),
		strings.Contains(lowerBody, "compromised"),
		strings.Contains(lowerBody, "found in a breach"):
		return "このパスワードは安全性が低いため使えません。別のパスワードにしてください。"
	case strings.Contains(lowerBody, "too weak"),
		strings.Contains(lowerBody, "strength"),
		strings.Contains(lowerBody, "stronger"):
		return "パスワードが弱すぎます。英字・数字を混ぜて、推測されにくいものにしてください。"
	case strings.Contains(lowerBody, "too short"),
		strings.Contains(lowerBody, "minimum"),
		strings.Contains(lowerBody, "length"):
		return "パスワードは6文字以上で入力してください。"
	default:
		return "パスワードを確認してください。安全性の高い別のパスワードを試してください。"
	}
}
