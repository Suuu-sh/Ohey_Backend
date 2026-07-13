package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Suuu-sh/Ohey_Backend/internal/config"
)

func TestPublicLegalDocumentsContainReleaseRequirements(t *testing.T) {
	handler := NewRouter(Dependencies{Config: config.Config{AllowedOrigins: []string{"*"}}})
	tests := []struct {
		path    string
		markers []string
	}{
		{
			path:    "/legal/terms.txt",
			markers: []string{"Ohey 利用規約", "通報・ブロック・ミュート・モデレーション", "お問い合わせ"},
		},
		{
			path:    "/legal/privacy-policy.txt",
			markers: []string{"Ohey プライバシーポリシー", "保存期間・アカウント削除", "第三者提供・委託・国外での取扱い"},
		},
	}
	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, test.path, nil))
			if res.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", res.Code, http.StatusOK)
			}
			for _, marker := range test.markers {
				if !strings.Contains(res.Body.String(), marker) {
					t.Fatalf("legal document is missing %q", marker)
				}
			}
		})
	}
}
