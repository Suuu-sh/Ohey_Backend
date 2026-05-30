package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestActionRateLimiterScopesByUserAndAction(t *testing.T) {
	now := time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC)
	limiter := newActionRateLimiter(func() time.Time { return now })
	policy := rateLimitPolicy{Action: "test_action", Limit: 2, Window: time.Minute}

	if allowed, _ := limiter.Allow(testUserID, policy); !allowed {
		t.Fatal("first request should be allowed")
	}
	if allowed, _ := limiter.Allow(testUserID, policy); !allowed {
		t.Fatal("second request should be allowed")
	}
	if allowed, retryAfter := limiter.Allow(testUserID, policy); allowed || retryAfter <= 0 {
		t.Fatalf("third request allowed=%v retryAfter=%v, want limited with retry", allowed, retryAfter)
	}
	if allowed, _ := limiter.Allow(otherUserID, policy); !allowed {
		t.Fatal("other user should have a separate bucket")
	}
	if allowed, _ := limiter.Allow(testUserID, rateLimitPolicy{Action: "other_action", Limit: 2, Window: time.Minute}); !allowed {
		t.Fatal("other action should have a separate bucket")
	}

	now = now.Add(time.Minute + time.Second)
	if allowed, _ := limiter.Allow(testUserID, policy); !allowed {
		t.Fatal("request after reset should be allowed")
	}
}

func TestEnforceRateLimitWritesRetryAfter(t *testing.T) {
	now := time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC)
	r := &router{rateLimiter: newActionRateLimiter(func() time.Time { return now })}
	policy := rateLimitPolicy{Action: "test_action", Limit: 1, Window: time.Minute}
	req := httptest.NewRequest(http.MethodPost, "/v1/test", nil)
	req.Header.Set("X-Ohey-User-ID", testUserID)

	w1 := httptest.NewRecorder()
	if !r.enforceRateLimit(w1, req, policy) {
		t.Fatal("first request should be allowed")
	}

	w2 := httptest.NewRecorder()
	if r.enforceRateLimit(w2, req, policy) {
		t.Fatal("second request should be limited")
	}
	if w2.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d body = %s", w2.Code, w2.Body.String())
	}
	if got := w2.Header().Get("Retry-After"); got == "" {
		t.Fatal("Retry-After header is missing")
	}
}
