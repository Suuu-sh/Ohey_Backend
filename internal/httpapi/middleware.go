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
		w.Header().Set("X-XSS-Protection", "0")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=()")
		w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		if strings.HasPrefix(req.URL.Path, "/share/") {
			w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; script-src 'unsafe-inline'; base-uri 'none'; frame-ancestors 'none'")
		} else {
			w.Header().Set("Content-Security-Policy", "default-src 'none'; base-uri 'none'; frame-ancestors 'none'")
		}
		if strings.HasPrefix(req.URL.Path, "/v1/") {
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("Pragma", "no-cache")
		}
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
