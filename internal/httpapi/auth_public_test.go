package httpapi

import "testing"

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
