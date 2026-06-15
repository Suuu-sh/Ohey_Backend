package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFriendlyClerkPasswordError(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "breached password",
			body: "clerk create user failed: status=422 body=map[errors:[map[message:Password has been found in a breach]]]",
			want: "このパスワードは安全性が低いため使えません。別のパスワードにしてください。",
		},
		{
			name: "weak password",
			body: "password is too weak",
			want: "パスワードが弱すぎます。英字・数字を混ぜて、推測されにくいものにしてください。",
		},
		{
			name: "unknown password error",
			body: "password is invalid",
			want: "パスワードを確認してください。安全性の高い別のパスワードを試してください。",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := friendlyClerkPasswordError(tt.body); got != tt.want {
				t.Fatalf("friendlyClerkPasswordError() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestClientIPUsesRightmostValidForwardedAddress(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/signup", nil)
	req.RemoteAddr = "10.0.0.10:12345"
	req.Header.Set("X-Forwarded-For", "203.0.113.9, bad, 198.51.100.7")

	if got := clientIP(req); got != "198.51.100.7" {
		t.Fatalf("clientIP() = %q, want rightmost valid forwarded address", got)
	}
}

func TestClientIPFallsBackToRemoteAddrWhenForwardedHeaderIsInvalid(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/signup", nil)
	req.RemoteAddr = "192.0.2.20:54321"
	req.Header.Set("X-Forwarded-For", "spoofed")

	if got := clientIP(req); got != "192.0.2.20" {
		t.Fatalf("clientIP() = %q, want remote addr host", got)
	}
}
