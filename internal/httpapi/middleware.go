package httpapi

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

func (r *router) withSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=()")
		next.ServeHTTP(w, req)
	})
}

func (r *router) originVerify(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		secret := strings.TrimSpace(r.deps.Config.OriginVerifySecret)
		if secret == "" || isOriginVerifyExempt(req.URL.Path) {
			next.ServeHTTP(w, req)
			return
		}
		actual := strings.TrimSpace(req.Header.Get("X-Origin-Verify"))
		if subtle.ConstantTimeCompare([]byte(actual), []byte(secret)) != 1 {
			writeError(w, http.StatusForbidden, "origin verification failed")
			return
		}
		next.ServeHTTP(w, req)
	})
}

func isOriginVerifyExempt(path string) bool {
	return path == "/health" || path == "/healthz"
}
